package cluster

import (
	"github.com/rs/zerolog/log"
)

func (m *MasterNode) StartJobExecutor(workers int) {
	if m.executor != nil {
		return
	}

	m.executor = NewJobExecutor(workers)
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
