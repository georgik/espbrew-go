package device

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type platformWatcher interface {
	close()
}

type Watcher struct {
	events   chan DeviceEvent
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	platform platformWatcher
	mu       sync.RWMutex
	seen     map[string]*DeviceInfo
}

func NewWatcher() *Watcher {
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{
		events: make(chan DeviceEvent, 10),
		ctx:    ctx,
		cancel: cancel,
		seen:   make(map[string]*DeviceInfo),
	}

	scanner := NewScanner()
	w.platform = newPollingWatcher(w, scanner)

	log.Info().Msg("Device watcher started")

	return w
}

func (w *Watcher) Events() <-chan DeviceEvent {
	return w.events
}

func (w *Watcher) Close() error {
	w.cancel()
	if w.platform != nil {
		w.platform.close()
	}
	w.wg.Wait()
	close(w.events)
	return nil
}

func (w *Watcher) sendEvent(event DeviceEvent) {
	select {
	case w.events <- event:
	default:
		log.Warn().Str("path", event.Path).Msg("Event channel full, dropping")
	}
}

type pollingWatcher struct {
	watcher  *Watcher
	scanner  *Scanner
	interval time.Duration
	ticker   *time.Ticker
	stop     chan struct{}
}

func newPollingWatcher(w *Watcher, s *Scanner) platformWatcher {
	pw := &pollingWatcher{
		watcher:  w,
		scanner:  s,
		interval: 2 * time.Second,
		stop:     make(chan struct{}),
	}

	w.wg.Add(1)
	go pw.run()

	return pw
}

func (pw *pollingWatcher) run() {
	defer pw.watcher.wg.Done()

	// Initial scan
	pw.scan()

	pw.ticker = time.NewTicker(pw.interval)
	defer pw.ticker.Stop()

	for {
		select {
		case <-pw.watcher.ctx.Done():
			return
		case <-pw.stop:
			return
		case <-pw.ticker.C:
			pw.scan()
		}
	}
}

func (pw *pollingWatcher) scan() {
	// Scan for serial ports
	ports, err := pw.scanner.Scan()
	if err != nil {
		log.Debug().Err(err).Msg("Device scan failed")
		return
	}

	// Deduplicate macOS tty/cu devices (same physical device)
	// Prefer cu.* devices (call-out, no carrier wait)
	ports = deduplicatePorts(ports)

	current := make(map[string]Port)
	for _, p := range ports {
		current[p.Path] = p
	}

	pw.watcher.mu.Lock()
	defer pw.watcher.mu.Unlock()

	// Detect added devices
	for path := range current {
		if _, exists := pw.watcher.seen[path]; !exists {
			// New device
			if pw.watcher.isLikelyESP(path) {
				dev := DeviceInfo{
					Path:         path,
					VID:          ESP_VID,
					PID:          ESP_PID_S3,
					SerialNumber: "",
				}
				pw.watcher.seen[path] = &dev
				pw.watcher.sendEvent(DeviceEvent{
					Type: DeviceAdded,
					Path: path,
					VID:  ESP_VID,
					PID:  ESP_PID_S3,
				})
				log.Info().Str("path", path).Msg("Device added")
			}
		}
	}

	// Detect removed devices
	for path := range pw.watcher.seen {
		if _, exists := current[path]; !exists {
			delete(pw.watcher.seen, path)
			pw.watcher.sendEvent(DeviceEvent{
				Type: DeviceRemoved,
				Path: path,
			})
			log.Info().Str("path", path).Msg("Device removed")
		}
	}
}

func (pw *pollingWatcher) close() {
	close(pw.stop)
}

func (w *Watcher) isLikelyESP(path string) bool {
	espPatterns := []string{
		"usbmodem", "usbserial", "ttyUSB", "ttyACM", "tty.wchusb",
		"SLAB", "CP21", "FTDI", "CH340",
		"COM", // Windows COM ports
	}

	pathLower := strings.ToLower(path)
	for _, pattern := range espPatterns {
		if strings.Contains(pathLower, strings.ToLower(pattern)) {
			return true
		}
	}

	// Explicit exclusions
	excludePatterns := []string{
		"bluetooth", "buds", "debug-console", "incoming",
	}
	for _, pattern := range excludePatterns {
		if strings.Contains(pathLower, pattern) {
			return false
		}
	}

	return false
}

// deduplicatePorts removes duplicate macOS tty/cu device entries
// On macOS, each USB serial device creates two entries:
//
//	/dev/cu.xxx (call-out, preferred)
//	/dev/tty.xxx (call-in, waits for carrier)
//
// We keep only the cu.* variant to avoid duplicate detection.
func deduplicatePorts(ports []Port) []Port {
	// Group by base device name
	groups := make(map[string][]Port)
	for _, p := range ports {
		base := deviceBaseName(p.Path)
		if base != "" {
			groups[base] = append(groups[base], p)
		}
	}

	result := make([]Port, 0, len(ports))
	seen := make(map[string]bool)

	for _, p := range ports {
		base := deviceBaseName(p.Path)
		if base == "" {
			// Not a macOS style device, keep as-is
			result = append(result, p)
			continue
		}

		if seen[base] {
			// Already processed this device group
			continue
		}
		seen[base] = true

		// Get all ports for this device
		group := groups[base]
		if len(group) == 1 {
			result = append(result, group[0])
			continue
		}

		// Prefer cu.* over tty.*
		var preferred Port
		for _, cand := range group {
			if containsPrefix(cand.Path, "/dev/cu.") {
				preferred = cand
				break
			}
		}
		// Fallback to first if no cu.* found
		if preferred.Path == "" && len(group) > 0 {
			preferred = group[0]
		}
		if preferred.Path != "" {
			result = append(result, preferred)
		}
	}

	return result
}

// deviceBaseName extracts the base device name without tty/cu prefix
// e.g., "/dev/cu.usbmodem1401" -> "usbmodem1401"
//
//	"/dev/tty.usbmodem1401" -> "usbmodem1401"
func deviceBaseName(path string) string {
	// Check for macOS-style /dev/{cu,tty}.{name}
	if strings.HasPrefix(path, "/dev/cu.") {
		return strings.TrimPrefix(path, "/dev/cu.")
	}
	if strings.HasPrefix(path, "/dev/tty.") && !strings.Contains(path, "ttyUSB") && !strings.Contains(path, "ttyACM") {
		// Exclude Linux-style ttyUSB/ttyACM
		return strings.TrimPrefix(path, "/dev/tty.")
	}
	return ""
}

func containsPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}
