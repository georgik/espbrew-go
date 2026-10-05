package cluster

import (
	"fmt"

	"github.com/georgik/espbrew-go/pkg/protocol"
	"github.com/rs/zerolog/log"
)

func (l *LeaderNode) StartJobExecutor(workers int) {
	l.StartJobExecutorWithProgress(workers, nil)
}

func (l *LeaderNode) StartJobExecutorWithProgress(workers int, progressCB func(string, int, string)) {
	if l.executor != nil {
		return
	}

	l.executor = NewJobExecutorWithProgress(workers, progressCB)
	// For Wokwi devices the "flash" is starting (or reusing) the simulation
	// rather than writing over a serial port. The session manager assembles the
	// image (bootloader + partition table + app) and uploads it, then calls
	// sim:start; `monitor` attaches to the resulting session.
	l.executor.wokwiHook = l.runWokwiFlash
	l.executor.Start()

	l.wg.Add(1)
	go l.handleJobResults()
}

// runWokwiFlash implements the executor's Wokwi hook. It returns handled=true
// only for Wokwi devices, in which case it starts (or reuses) the simulation
// instead of flashing over a serial port.
func (l *LeaderNode) runWokwiFlash(job *Job) (bool, error) {
	l.mu.RLock()
	dev, exists := l.state.Devices[job.DevicePath]
	l.mu.RUnlock()
	if !exists || dev.Backend != protocol.BackendWokwi {
		return false, nil
	}

	// A flash can carry its own Wokwi diagram (from the project's
	// diagram.json) that overrides the device default, so a board-specific
	// diagram is used even for the generic wokwi:<chip> devices. The monitor
	// reads the diagram from the device's WokwiConfig when it starts.
	monitorDev := dev
	if job.Diagram != "" {
		cfg, ok := dev.BackendConfig.(*protocol.WokwiConfig)
		if ok {
			cfgCopy := *cfg
			cfgCopy.DiagramJSON = job.Diagram
			monitorDev = &protocol.DeviceInfo{
				Path:          dev.Path,
				DeviceID:      dev.DeviceID,
				ChipType:      dev.ChipType,
				Backend:       dev.Backend,
				BackendConfig: &cfgCopy,
			}
			log.Info().Str("device", dev.Path).Int("diagram_bytes", len(job.Diagram)).Msg("Using project diagram for Wokwi flash")
		} else {
			log.Warn().Str("device", dev.Path).Msg("Job carries a diagram but device has no Wokwi config; ignoring diagram")
		}
	}

	// Start (or reuse) the simulation. Get blocks until sim:start has been
	// sent, so by the time this returns the image is uploaded and running.
	if _, err := l.wokwiSessions.Get(monitorDev, job.Firmware); err != nil {
		return true, fmt.Errorf("start wokwi simulation: %w", err)
	}
	return true, nil
}

func (l *LeaderNode) StopJobExecutor() {
	if l.executor != nil {
		l.executor.Stop()
	}
}

func (l *LeaderNode) handleJobResults() {
	defer l.wg.Done()

	for result := range l.executor.Results() {
		job := result.Job

		// Release device lock
		l.devices.Release(job.DevicePath, job.ID)

		// Update device status
		l.mu.Lock()
		if dev, exists := l.state.Devices[job.DevicePath]; exists {
			dev.Status = "available"
			l.state.Devices[job.DevicePath] = dev
		}
		l.mu.Unlock()

		// Complete job in queue
		l.queue.Complete(job.ID, result.Error)

		// The operation is done: stop holding the board busy and reset its idle
		// timer so it is powered down after the idle timeout of further use.
		l.releaseDeviceAfterOp(job.DevicePath)

		if result.Error != nil {
			log.Error().Err(result.Error).Str("job_id", job.ID).Msg("Job failed")
		} else {
			log.Info().Str("job_id", job.ID).Msg("Job succeeded")
		}
	}
}

func (l *LeaderNode) CancelJob(jobID string) error {
	job := l.queue.Get(jobID)
	if job == nil {
		return fmt.Errorf("job not found: %s", jobID)
	}

	job.RLock()
	status := job.Status
	devicePath := job.DevicePath
	job.RUnlock()

	if status == JobComplete || status == JobFailed || status == JobCancelled || status == JobTimedOut {
		return fmt.Errorf("cannot cancel job in state: %s", status)
	}

	// Cancel the job
	if err := l.queue.Cancel(jobID); err != nil {
		return err
	}

	// Release device if it was reserved
	l.devices.Release(devicePath, jobID)

	// Update device status
	l.mu.Lock()
	if dev, exists := l.state.Devices[devicePath]; exists {
		dev.Status = "available"
		l.state.Devices[devicePath] = dev
	}
	l.mu.Unlock()

	log.Info().Str("job_id", jobID).Msg("Job cancelled")

	return nil
}
