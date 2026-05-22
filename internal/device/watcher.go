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
