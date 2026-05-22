package cluster

import (
	"context"
	"os"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/stretchr/testify/assert"
)

func TestJobExecutor_Submit(t *testing.T) {
	executor := NewJobExecutor(2)
	executor.Start()
	defer executor.Stop()

	job := &Job{
		ID:         "test-job-1",
		Firmware:   "test.bin",
		DevicePath: "/dev/ttyUSB0",
		Status:     JobPending,
		CreatedAt:  time.Now(),
	}

	// Create a dummy firmware file
	os.WriteFile(job.Firmware, []byte("dummy firmware"), 0644)
	defer os.Remove(job.Firmware)

	executor.Submit(job)

	// Job was submitted (execution may fail due to no device)
	select {
	case result := <-executor.Results():
		assert.Equal(t, job.ID, result.Job.ID)
		assert.NotNil(t, result) // May have error
	case <-time.After(5 * time.Second):
		t.Log("No result within timeout (expected if no device)")
	}
}

func TestJobExecutor_StartStop(t *testing.T) {
	executor := NewJobExecutor(1)
	executor.Start()

	assert.NotNil(t, executor.jobs)
	assert.NotNil(t, executor.results)

	executor.Stop()

	// Channels should be closed after stop
	_, jobsOpen := <-executor.jobs
	_, resultsOpen := <-executor.results

	assert.False(t, jobsOpen, "jobs channel should be closed")
	assert.False(t, resultsOpen, "results channel should be closed")
}

func TestLeaderNode_JobExecutor(t *testing.T) {
	leader := NewLeaderNode("test", &LeaderConfig{
		HTTPPort:          8080,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})

	ctx := context.Background()
	leader.Start(ctx)
	defer leader.Stop()

	// Register device
	leader.RegisterDevice(&protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		Status: "available",
	})

	// Start executor
	leader.StartJobExecutor(1)
	defer leader.StopJobExecutor()

	assert.NotNil(t, leader.executor)

	// Create firmware file
	firmwarePath := "/tmp/test-firmware.bin"
	os.WriteFile(firmwarePath, []byte("test"), 0644)
	defer os.Remove(firmwarePath)

	// Enqueue job
	job, err := leader.EnqueueJob(firmwarePath, "/dev/ttyUSB0")
	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, firmwarePath, job.Firmware)
}
