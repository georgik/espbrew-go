package cluster

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

func (m *MasterNode) StartJobExecutor(workers int) {
	m.StartJobExecutorWithProgress(workers, nil)
}

func (m *MasterNode) StartJobExecutorWithProgress(workers int, progressCB func(string, int, string)) {
	if m.executor != nil {
		return
	}

	m.executor = NewJobExecutorWithProgress(workers, progressCB)
	m.executor.Start()

	m.wg.Add(1)
	go m.handleJobResults()
}

func (m *MasterNode) StopJobExecutor() {
	if m.executor != nil {
		m.executor.Stop()
	}
}

func (m *MasterNode) handleJobResults() {
	defer m.wg.Done()

	for result := range m.executor.Results() {
		job := result.Job

		// Release device lock
		m.devices.Release(job.DevicePath, job.ID)

		// Update device status
		m.mu.Lock()
		if dev, exists := m.state.Devices[job.DevicePath]; exists {
			dev.Status = "available"
			m.state.Devices[job.DevicePath] = dev
		}
		m.mu.Unlock()

		// Complete job in queue
		m.queue.Complete(job.ID, result.Error)

		if result.Error != nil {
			log.Error().Err(result.Error).Str("job_id", job.ID).Msg("Job failed")
		} else {
			log.Info().Str("job_id", job.ID).Msg("Job succeeded")
		}
	}
}

func (m *MasterNode) CancelJob(jobID string) error {
	job := m.queue.Get(jobID)
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
	if err := m.queue.Cancel(jobID); err != nil {
		return err
	}

	// Release device if it was reserved
	m.devices.Release(devicePath, jobID)

	// Update device status
	m.mu.Lock()
	if dev, exists := m.state.Devices[devicePath]; exists {
		dev.Status = "available"
		m.state.Devices[devicePath] = dev
	}
	m.mu.Unlock()

	log.Info().Str("job_id", jobID).Msg("Job cancelled")

	return nil
}
