package cluster

import (
	"errors"
	"fmt"
	"time"

	"github.com/georgik/espbrew-go/internal/config"
	"github.com/georgik/espbrew-go/internal/devicesleep"
	"github.com/georgik/espbrew-go/internal/persistence"
	"github.com/georgik/espbrew-go/internal/powercontrol"
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
	// Opt-in uhubctl controller: drives the hub by its location string, so it
	// can power boards that sit on a PARENT hub (which the default sysfs
	// controller's listHubs skips). Enabled with ESPBREW_POWER_UHUBCTL.
	if powercontrol.Enabled() {
		return powercontrol.NewUhubController()
	}
	return powercontrol.NewController()
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

	l.mu.Lock()
	if dev, exists := l.state.Devices[path]; exists {
		dev.Sleeping = false
		if dev.Status == "sleeping" {
			dev.Status = "available"
		}
		l.state.Devices[path] = dev
	}
	l.mu.Unlock()

	delay := l.sleeper.WakeDelay()
	log.Info().Str("path", path).Dur("wake_delay", delay).
		Msg("Device woken, waiting for enumeration before operation")
	if delay > 0 {
		time.Sleep(delay)
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
