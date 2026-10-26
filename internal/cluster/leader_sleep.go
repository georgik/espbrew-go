package cluster

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/georgik/espbrew-go/internal/config"
	"github.com/georgik/espbrew-go/internal/devicesleep"
	"github.com/georgik/espbrew-go/internal/persistence"
	"github.com/georgik/espbrew-go/internal/powercontrol"
	"github.com/georgik/espbrew-go/internal/powercycler"
	"github.com/georgik/espbrew-go/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// sleepController returns the power controller used to switch hub ports. Tests
// inject a mock via LeaderNode.powerCtrl; production uses the real platform
// controller, which is a no-op (ErrNotSupported) on platforms without
// power-controllable hubs.
func (l *LeaderNode) sleepController() devicesleep.Controller {
	if l.powerCtrl != nil {
		return l.powerCtrl
	}
	// Power-cycler mode. "remote" (or "external") drives hub port power by
	// calling the dedicated, privileged hub-power pod over the cluster network,
	// so the leader itself stays non-privileged. "direct" (the default) uses
	// the in-process controller (local sysfs, needs a privileged pod). The mode
	// is read from LeaderConfig first, then the ESPBREW_POWER_MODE env var, so
	// it can be set via the Deployment manifest or the environment.
	mode := strings.ToLower(strings.TrimSpace(l.config.PowerMode))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(os.Getenv("ESPBREW_POWER_MODE")))
	}
	if mode == "remote" || mode == "external" {
		url := strings.TrimSpace(l.config.PowerRemoteURL)
		if url == "" {
			url = strings.TrimSpace(os.Getenv("ESPBREW_POWER_REMOTE_URL"))
		}
		log.Info().Str("mode", mode).Str("remote_url", url).
			Msg("Power-cycler mode: remote (hub-power pod)")
		return powercycler.NewRemote(url)
	}
	// Default: opt-in uhubctl controller, else the platform controller. The
	// platform-specific selection lives in powercontrol.NewPreferredController,
	// so this file stays cross-platform.
	return powercontrol.NewPreferredController()
}

// countPowerManagedDevices returns how many espbrew.toml devices declare a USB
// hub location (and are therefore power-managed).
func (l *LeaderNode) countPowerManagedDevices() int {
	count := 0
	for i := range l.config.Devices {
		if l.config.Devices[i].PowerManaged() {
			count++
		}
	}
	return count
}

// registerPowerManagedDevices registers every espbrew.toml device that declares
// a USB hub location with the sleep manager. It only records the location and
// port; the initial awake/sleeping state is reconciled after the watcher has
// scanned the live USB tree (see reconcilePowerManagedDevices).
func (l *LeaderNode) registerPowerManagedDevices() {
	if l.sleeper == nil {
		return
	}
	for i := range l.config.Devices {
		cfg := l.config.Devices[i]
		loc, port, ok := cfg.PowerLocation()
		if !ok {
			continue
		}
		l.sleeper.Register(cfg.Path, loc, port)
		log.Debug().Str("path", cfg.Path).Str("alias", cfg.Alias).
			Str("location", loc).Int("port", port).
			Msg("Power-managed device registered for auto sleep")
	}
}

// reconcilePowerManagedDevices marks each power-managed board sleeping when it
// is not present in the live USB tree, and creates a record for it so it can
// still be addressed by alias. Boards that are present stay awake. It is called
// once after the watcher has had a chance to scan.
func (l *LeaderNode) reconcilePowerManagedDevices() {
	if l.sleeper == nil {
		return
	}
	for i := range l.config.Devices {
		cfg := l.config.Devices[i]
		_, _, ok := cfg.PowerLocation()
		if !ok {
			continue
		}

		l.mu.RLock()
		_, present := l.state.Devices[cfg.Path]
		l.mu.RUnlock()

		if present {
			// Board is live and powered: make sure the manager agrees.
			l.sleeper.SetSleeping(cfg.Path, false)
			continue
		}
		l.ensureSleepingRecord(cfg)
	}
}

// ensureSleepingRecord creates (if needed) a record for a power-managed board
// that is currently absent from the live USB tree, and marks it sleeping.
func (l *LeaderNode) ensureSleepingRecord(cfg config.DeviceConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if dev, exists := l.state.Devices[cfg.Path]; exists {
		// It appeared between the check and the lock - keep it awake.
		dev.Sleeping = false
		if dev.Status == "sleeping" {
			dev.Status = "available"
		}
		return
	}

	record := &persistence.DeviceRecord{
		DeviceID:    cfg.ID,
		ChipType:    cfg.ChipType,
		Aliases:     []string{cfg.Alias},
		Description: cfg.Description,
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		LastPath:    cfg.Path,
		NodeID:      l.id,
	}
	if err := l.store.SaveDevice(record); err != nil {
		log.Warn().Str("path", cfg.Path).Err(err).Msg("Failed to persist sleeping device record")
	}

	dev := &protocol.DeviceInfo{
		Path:      cfg.Path,
		RealPath:  cfg.Path,
		NodeID:    l.id,
		DeviceID:  cfg.ID,
		ChipType:  cfg.ChipType,
		Name:      cfg.Alias,
		Status:    "sleeping",
		Sleeping:  true,
		Backend:   protocol.BackendPhysical,
		FirstSeen: time.Now(),
	}
	l.state.Devices[cfg.Path] = dev
	l.sleeper.SetSleeping(cfg.Path, true)
	// Register the lock too so a later wake+reserve (flash/monitor) can claim it
	// even though it is not yet present on the live USB bus.
	l.devices.Register(cfg.Path)

	log.Info().Str("path", cfg.Path).Str("alias", cfg.Alias).
		Msg("Device not present in live USB tree - marked sleeping (wakes before use)")
}

// wakeDevice powers a sleeping power-managed board on and waits for it to
// enumerate before an operation starts. It is a no-op for boards that are not
// power-managed or already awake. If power control fails it logs a warning and
// still returns, so the operation can proceed (the board may already be up).
func (l *LeaderNode) wakeDevice(path string) {
	if l.sleeper == nil || !l.sleeper.IsSleeping(path) {
		return
	}
	if err := l.sleeper.Wake(path); err != nil {
		log.Warn().Str("path", path).Err(err).
			Msg("Failed to power on device before operation (continuing)")
		return
	}
	// Mark the board in use so the idle timer starts counting from now and the
	// board is reported awake while it enumerates.
	l.sleeper.Touch(path)

	// Mark the board in use so the idle timer starts counting from now and the
	// board is reported awake while it enumerates.
	l.setStateAwake(path)

	delay := l.sleeper.WakeDelay()
	log.Info().Str("path", path).Dur("wake_delay", delay).
		Msg("Device woken, waiting for enumeration before operation")
	if delay > 0 {
		time.Sleep(delay)
	}
}

// setStateAwake mirrors the manager's awake state onto the live device record
// so the API and flash/monitor selection report the board as available. It is
// safe to call on a board that is already awake.
func (l *LeaderNode) setStateAwake(path string) {
	l.mu.Lock()
	if dev, exists := l.state.Devices[path]; exists {
		dev.Sleeping = false
		if dev.Status == "sleeping" {
			dev.Status = "available"
		}
		l.state.Devices[path] = dev
	}
	l.mu.Unlock()
}

// wakeDeviceForFlash prepares a power-managed board for a flash. It performs a
// full hub power cycle (cold boot) when the node can control the hub, then
// wakes the board and waits for enumeration. The power cycle is the "reliable
// cold boot" that lets the subsequent DTR/RTS bootloader reset (the flasher's
// ResetBridge path) land on a freshly rebooted chip instead of one still
// running the app.
//
// It is a no-op for boards that are not power-managed. A power-cycle failure is
// non-fatal (logged, then falls back to wakeDevice): a board on a node without
// a controllable hub still flashes via the DTR/RTS reset alone.
func (l *LeaderNode) wakeDeviceForFlash(path string) {
	if l.sleeper != nil && isPowerManagedPath(l, path) {
		if err := l.sleeper.PowerCycle(path); err != nil {
			log.Warn().Str("path", path).Err(err).
				Msg("Power cycle before flash failed (falling back to DTR/RTS reset alone)")
		} else {
			log.Info().Str("path", path).Msg("Power-cycled board before flash (cold boot)")
		}
	}
	l.wakeDevice(path)
	// PowerCycle already flipped the manager's sleeping flag and powered the
	// port back on, so wakeDevice() returns early without mirroring that state
	// onto the live record. Do it here so the board is reported awake (and
	// available for selection) regardless of which path woke it.
	l.setStateAwake(path)
}

// WakeDeviceForSnap power-cycles a power-managed board so the snapshot monitor
// starts from a fresh boot. The esp_lcd_screenshot framebuffer dump fires once
// per boot at generation 10 (~5.5s after the reset); coordinating that reset
// with the start of the monitor window makes the dump land well inside the
// window, so it is captured reliably instead of firing before the monitor
// begins listening (which leaves only the periodic re-dump ~38s later, usually
// past the window).
//
// It is a no-op for boards that are not power-managed. A power-cycle failure is
// non-fatal (logged, then continues): the monitor still runs against a live
// board. The follow-through wakeDevice() gives the chip a moment to
// re-enumerate so the monitor opens the fresh node rather than a stale one.
func (l *LeaderNode) WakeDeviceForSnap(path string) {
	powerCycled := false
	if l.sleeper != nil && isPowerManagedPath(l, path) {
		if err := l.sleeper.PowerCycle(path); err != nil {
			log.Warn().Str("path", path).Err(err).
				Msg("Power cycle before snap failed (continuing)")
		} else {
			log.Info().Str("path", path).Msg("Power-cycled board before snap (fresh boot)")
			powerCycled = true
		}
	}
	// Hold the board busy for the whole wake-up + monitor window so the idle
	// sweeper cannot power the hub port off mid-snapshot. PowerCycle already
	// cleared the sleeping flag, so wakeDevice() below early-returns without
	// calling Touch(); the busy flag is what keeps the sweeper away. The
	// caller releases it once the monitor finishes (see ReleaseDeviceBusy).
	if l.sleeper != nil {
		l.sleeper.SetBusy(path, true)
	}
	l.wakeDevice(path)
	// Only a power cycle re-enumerates the USB-UART bridge, dropping its
	// /dev/serial/by-id symlink for a moment. Wait for it to reappear so the
	// monitor opens the freshly created node instead of a stale/absent one.
	// Skip the wait when nothing was power-cycled (virtual or non-power-managed
	// devices): there is nothing to re-enumerate, and polling a path that never
	// resolves would just burn the full 5s timeout.
	if powerCycled {
		l.waitForUSBNode(path, 5*time.Second)
	}
	l.setStateAwake(path)
}

// waitForUSBNode polls os.Readlink on the given /dev/serial/by-id path until it
// resolves (the board has re-enumerated after a power cycle) or the timeout
// elapses. A power-cycled USB-UART bridge drops its by-id symlink for a short
// window while the kernel re-enumerates it, so callers that start an operation
// immediately after a cycle must wait out that window first.
func (l *LeaderNode) waitForUSBNode(path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Readlink(path); err == nil {
			return
		}
		if !time.Now().Before(deadline) {
			log.Warn().Str("path", path).Msg("USB node did not re-enumerate within timeout")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// releaseDeviceAfterOp clears the busy flag and resets the idle timer after an
// operation finishes, so the board is powered down after IdleTimeout of use.
func (l *LeaderNode) releaseDeviceAfterOp(path string) {
	if l.sleeper == nil {
		return
	}
	l.sleeper.SetBusy(path, false)
	l.sleeper.Touch(path)
}

// ReleaseDeviceBusy is the exported counterpart used by the snap path: the
// monitor holds the board busy from just after the power cycle until the
// monitor finishes, then releases it here so the idle sweeper resumes.
func (l *LeaderNode) ReleaseDeviceBusy(path string) {
	l.releaseDeviceAfterOp(path)
}

// isPowerManagedPath reports whether the given port is a power-managed board
// (declares a USB hub location in espbrew.toml).
func isPowerManagedPath(l *LeaderNode, path string) bool {
	if l.sleeper == nil {
		return false
	}
	cfg := l.config.DeviceByPath(path)
	return cfg != nil && cfg.PowerManaged()
}

// runDeviceSleepLoop periodically asks the sleep manager to power down idle
// boards and mirrors the result onto the device records. It exits when the
// leader context is cancelled.
func (l *LeaderNode) runDeviceSleepLoop() {
	defer l.wg.Done()

	period := time.Second
	if l.sleeper != nil {
		period = l.sleeper.Config().SweepPeriod
	}
	t := time.NewTicker(period)
	defer t.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-t.C:
			l.reconcileDeviceSleep()
		}
	}
}

// reconcileDeviceSleep powers down idle boards and marks them sleeping in the
// live state so the API and flash/monitor selection see the truth.
func (l *LeaderNode) reconcileDeviceSleep() {
	if l.sleeper == nil {
		return
	}
	down := l.sleeper.Sweep(time.Now())
	for _, path := range down {
		l.mu.Lock()
		if dev, ok := l.state.Devices[path]; ok {
			dev.Sleeping = true
			dev.Status = "sleeping"
			l.state.Devices[path] = dev
		}
		l.mu.Unlock()
		log.Info().Str("path", path).Msg("Device put to sleep after idle timeout")
	}
}

// PowerDevice manually switches a power-managed board's hub port on or off via
// the sleep manager, so an operator (or the UI) can force a cold boot / power
// save on demand, independent of the idle timer.
//
//   - on=true  wakes the board: powers the hub port on and waits WakeDelay for
//     enumeration (a no-op if the board is already awake).
//   - on=false powers the board off and marks it sleeping.
//
// It returns an error if power management is unavailable on this node, the
// board is not power-managed, the board is busy (mid-operation), or the hub
// cannot be reached. A board that is currently absent from the live USB tree is
// still addressable by alias: waking it powers the port back on, while powering
// an already-absent board off is rejected as a no-op.
func (l *LeaderNode) PowerDevice(path string, on bool) error {
	if l.sleeper == nil {
		return fmt.Errorf("power management is unavailable on this node")
	}
	if !isPowerManagedPath(l, path) {
		return fmt.Errorf("device %s is not power-managed (add usb_location/usb_port to its espbrew.toml entry)", path)
	}

	l.mu.RLock()
	_, exists := l.state.Devices[path]
	l.mu.RUnlock()
	if !exists && on {
		// Absent + asked to power on: still addressable by alias, wake it.
	} else if !exists && !on {
		return fmt.Errorf("device %s is not present and is already powered off", path)
	}

	// Never cut power to a board that is mid-operation.
	if l.sleeper.IsBusy(path) {
		return fmt.Errorf("device %s is busy - power state unchanged", path)
	}

	if on {
		if err := l.sleeper.Wake(path); err != nil {
			return wrapPowerErr(err, path, true)
		}
		l.mu.Lock()
		if dev, ok := l.state.Devices[path]; ok {
			dev.Sleeping = false
			if dev.Status == "sleeping" {
				dev.Status = "available"
			}
			l.state.Devices[path] = dev
		}
		l.mu.Unlock()
		log.Info().Str("path", path).Msg("Device powered on via request")
		return nil
	}

	if err := l.sleeper.Sleep(path); err != nil {
		return wrapPowerErr(err, path, false)
	}
	l.mu.Lock()
	if dev, ok := l.state.Devices[path]; ok {
		dev.Sleeping = true
		if dev.Status == "available" {
			dev.Status = "sleeping"
		}
		l.state.Devices[path] = dev
	}
	l.mu.Unlock()
	log.Info().Str("path", path).Msg("Device powered off via request")
	return nil
}

// wrapPowerErr turns a hub-control failure into a clear, actionable message.
// A missing hub almost always means the board is not attached to a hub espbrew
// can control (e.g. it sits on a parent hub that listHubs skips, or the USB
// ioctl nodes are unavailable), so we say so explicitly.
func wrapPowerErr(err error, path string, on bool) error {
	if errors.Is(err, powercontrol.ErrHubNotFound) {
		return fmt.Errorf("cannot power %s device %s: hub not found. The board's USB hub is not one espbrew can control (listHubs only drives leaf hubs, or the /dev/bus/usb nodes are unavailable)", boolWord(on), path)
	}
	return fmt.Errorf("failed to power %s: %w", boolWord(on), err)
}

func boolWord(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
