package cluster

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/google/uuid"
)

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobAssigned  JobStatus = "assigned"
	JobRunning   JobStatus = "running"
	JobComplete JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

type Job struct {
	ID         string
	Firmware   string
	DevicePath string
	DeviceNode string
	Status     JobStatus
	Progress   int
	CreatedAt  time.Time
	StartedAt  *time.Time
	CompletedAt *time.Time
	Error      string
	mu         sync.RWMutex
}

type JobQueue struct {
	jobs  map[string]*Job
	pending []string
	mu    sync.RWMutex
}

func NewJobQueue() *JobQueue {
	return &JobQueue{
		jobs:   make(map[string]*Job),
		pending: make([]string, 0),
	}
}

func (q *JobQueue) Enqueue(firmwarePath, devicePath string) *Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	job := &Job{
		ID:         uuid.New().String(),
		Firmware:   firmwarePath,
		DevicePath: devicePath,
		Status:     JobPending,
		CreatedAt:  time.Now(),
	}

	q.jobs[job.ID] = job
	q.pending = append(q.pending, job.ID)

	log.Info().Str("job_id", job.ID).Str("device", devicePath).
		Msg("Job enqueued")

	return job
}

func (q *JobQueue) Dequeue(deviceNode string) *Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) == 0 {
		return nil
	}

	jobID := q.pending[0]
	q.pending = q.pending[1:]

	job := q.jobs[jobID]
	job.mu.Lock()
	job.Status = JobAssigned
	job.DeviceNode = deviceNode
	now := time.Now()
	job.StartedAt = &now
	job.mu.Unlock()

	log.Info().Str("job_id", job.ID).Str("node", deviceNode).
		Msg("Job dequeued")

	return job
}

func (q *JobQueue) Get(jobID string) *Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.jobs[jobID]
}

func (q *JobQueue) List() []*Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	jobs := make([]*Job, 0, len(q.jobs))
	for _, j := range q.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}

func (q *JobQueue) UpdateProgress(jobID string, progress int) {
	q.mu.RLock()
	job := q.jobs[jobID]
	q.mu.RUnlock()

	if job == nil {
		return
	}

	job.mu.Lock()
	job.Progress = progress
	job.mu.Unlock()

	log.Debug().Str("job_id", jobID).Int("progress", progress).
		Msg("Job progress updated")
}

func (q *JobQueue) Complete(jobID string, err error) {
	q.mu.RLock()
	job := q.jobs[jobID]
	q.mu.RUnlock()

	if job == nil {
		return
	}

	job.mu.Lock()
	defer job.mu.Unlock()

	now := time.Now()
	job.CompletedAt = &now

	if err != nil {
		job.Status = JobFailed
		job.Error = err.Error()
		log.Error().Str("job_id", jobID).Err(err).Msg("Job failed")
	} else {
		job.Status = JobComplete
		log.Info().Str("job_id", jobID).Msg("Job completed")
	}
}

func (q *JobQueue) PendingCount() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.pending)
}

func (j *Job) ToMap() map[string]interface{} {
	j.mu.RLock()
	defer j.mu.RUnlock()

	m := map[string]interface{}{
		"id":          j.ID,
		"firmware":    j.Firmware,
		"device_path": j.DevicePath,
		"device_node": j.DeviceNode,
		"status":      j.Status,
		"progress":    j.Progress,
		"created_at":  j.CreatedAt,
	}

	if j.StartedAt != nil {
		m["started_at"] = *j.StartedAt
	}
	if j.CompletedAt != nil {
		m["completed_at"] = *j.CompletedAt
	}
	if j.Error != "" {
		m["error"] = j.Error
	}

	return m
}
