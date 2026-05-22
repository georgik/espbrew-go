package cluster

import (
	"fmt"

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
	l.executor.Start()

	l.wg.Add(1)
	go l.handleJobResults()
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
