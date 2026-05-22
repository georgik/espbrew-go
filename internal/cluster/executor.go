package cluster

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/georgik/esp-ci-cluster/internal/flash"
	"github.com/rs/zerolog/log"
)

type JobExecutor struct {
	flasher *flash.Flasher
	jobs    chan *Job
	results chan *JobResult
	workers int
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

type JobResult struct {
	Job   *Job
	Error error
}

func NewJobExecutor(workers int) *JobExecutor {
	ctx, cancel := context.WithCancel(context.Background())
	return &JobExecutor{
		flasher: flash.NewFlasher(nil),
		jobs:    make(chan *Job, 100),
		results: make(chan *JobResult, 100),
		workers: workers,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (e *JobExecutor) Start() {
	log.Info().Int("workers", e.workers).Msg("Starting job executor")

	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go e.worker(i)
	}
}

func (e *JobExecutor) Stop() {
	log.Info().Msg("Stopping job executor")
	e.cancel()
	e.wg.Wait()
	close(e.jobs)
	close(e.results)
}

func (e *JobExecutor) Submit(job *Job) {
	select {
	case e.jobs <- job:
		log.Debug().Str("job_id", job.ID).Msg("Job submitted")
	case <-e.ctx.Done():
		log.Warn().Str("job_id", job.ID).Msg("Executor shutdown, job rejected")
	}
}

func (e *JobExecutor) Results() <-chan *JobResult {
	return e.results
}

func (e *JobExecutor) worker(id int) {
	defer e.wg.Done()

	log.Info().Int("worker_id", id).Msg("Worker started")

	for {
		select {
		case <-e.ctx.Done():
			log.Info().Int("worker_id", id).Msg("Worker shutting down")
			return
		case job, ok := <-e.jobs:
			if !ok {
				return
			}
			e.executeJob(id, job)
		}
	}
}

func (e *JobExecutor) executeJob(workerID int, job *Job) {
	log.Info().
		Str("job_id", job.ID).
		Str("device", job.DevicePath).
		Int("worker_id", workerID).
		Msg("Executing job")

	job.mu.Lock()
	job.Status = JobRunning
	job.mu.Unlock()

	// Load firmware file
	firmware, err := os.ReadFile(job.Firmware)
	if err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Failed to read firmware")
		e.results <- &JobResult{Job: job, Error: err}
		return
	}

	// Execute flash
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Minute)
	defer cancel()

	req := &flash.FlashRequest{
		Port:     job.DevicePath,
		Firmware: firmware,
		Progress: make(chan int, 10),
	}

	// Progress goroutine
	go func() {
		for progress := range req.Progress {
			job.mu.Lock()
			job.Progress = progress
			job.mu.Unlock()
			log.Debug().Str("job_id", job.ID).Int("progress", progress).Msg("Flash progress")
		}
	}()

	result := e.flasher.Flash(ctx, req)
	close(req.Progress)

	job.mu.Lock()
	if result.Error != nil {
		job.Status = JobFailed
		job.Error = result.Error.Error()
		log.Error().Err(result.Error).Str("job_id", job.ID).Msg("Job failed")
	} else {
		job.Status = JobComplete
		job.Progress = 100
		now := job.CreatedAt // Use created time as fallback
		job.CompletedAt = &now
		log.Info().Str("job_id", job.ID).Msg("Job completed successfully")
	}
	job.mu.Unlock()

	e.results <- &JobResult{Job: job, Error: result.Error}
}

// FlashWithProgress flashes firmware and reports progress via callback
func FlashWithProgress(ctx context.Context, port string, firmware []byte, progress func(int)) error {
	f := flash.NewFlasher(nil)
	req := &flash.FlashRequest{
		Port:     port,
		Firmware: firmware,
		Progress: make(chan int, 10),
	}

	// Forward progress
	done := make(chan struct{})
	go func() {
		for p := range req.Progress {
			if progress != nil {
				progress(p)
			}
		}
		close(done)
	}()

	result := f.Flash(ctx, req)
	<-done

	return result.Error
}

// MonitorSerial opens serial monitor for a device
type SerialMonitor struct {
	port   io.ReadCloser
	cancel context.CancelFunc
}

func NewSerialMonitor(port string) (*SerialMonitor, error) {
	// TODO: Implement serial monitor using go.bug.st/serial
	return nil, nil
}

func (m *SerialMonitor) Read(p []byte) (n int, err error) {
	if m.port == nil {
		return 0, io.EOF
	}
	return m.port.Read(p)
}

func (m *SerialMonitor) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.port != nil {
		return m.port.Close()
	}
	return nil
}
