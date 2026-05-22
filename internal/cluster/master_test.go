package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/georgik/esp-ci-cluster/pkg/protocol"
	"github.com/stretchr/testify/assert"
)

func TestMasterNodeRegistersDevice(t *testing.T) {
	master := NewMasterNode("test-master", &MasterConfig{
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
		HTTPPort:          8080,
		DisablemDNS:       true, // Disable mDNS in tests
	})

	ctx := context.Background()
	assert.NoError(t, master.Start(ctx))
	defer master.Stop()

	dev := &protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		NodeID: "worker-1",
		Status: "available",
	}

	master.state.Devices[dev.Path] = dev

	state := master.State()
	assert.Equal(t, 1, len(state.Devices))
	assert.Equal(t, dev, state.Devices[dev.Path])
}

func TestMasterNodeTimeoutCleanup(t *testing.T) {
	master := NewMasterNode("test-master", &MasterConfig{
		HeartbeatInterval: 100 * time.Millisecond,
		NodeTimeout:       200 * time.Millisecond,
		HTTPPort:          8080,
		DisablemDNS:       true,
	})

	ctx := context.Background()
	assert.NoError(t, master.Start(ctx))
	defer master.Stop()

	node := &protocol.NodeInfo{
		ID:       "stale-node",
		Address:  "127.0.0.1",
		Role:     "worker",
		LastSeen: time.Now().Add(-1 * time.Minute),
	}
	master.RegisterNode(node)

	assert.Equal(t, 1, len(master.State().Nodes))

	time.Sleep(300 * time.Millisecond)

	assert.Equal(t, 0, len(master.State().Nodes), "stale node should be cleaned up")
}

func TestWorkerNodeHeartbeat(t *testing.T) {
	worker := NewWorkerNode("test-worker", "ws://master:8080", &WorkerConfig{
		HeartbeatInterval: 100 * time.Millisecond,
		HTTPPort:          8081,
		DisablemDNS:       true,
		DisableWatcher:    true,
	})

	ctx := context.Background()
	assert.NoError(t, worker.Start(ctx))
	defer worker.Stop()

	assert.Equal(t, "test-worker", worker.id)
	assert.Equal(t, "ws://master:8080", worker.masterURL)
}

func TestJobQueue(t *testing.T) {
	q := NewJobQueue()

	job1 := q.Enqueue("firmware1.bin", "/dev/ttyUSB0")
	job2 := q.Enqueue("firmware2.bin", "/dev/ttyUSB1")

	assert.Equal(t, 2, q.PendingCount())

	// Dequeue should return jobs in order
	dequeued := q.Dequeue("node-1")
	assert.Equal(t, job1.ID, dequeued.ID)
	assert.Equal(t, 1, q.PendingCount())

	dequeued = q.Dequeue("node-2")
	assert.Equal(t, job2.ID, dequeued.ID)
	assert.Equal(t, 0, q.PendingCount())

	// Dequeuing empty queue returns nil
	dequeued = q.Dequeue("node-3")
	assert.Nil(t, dequeued)
}

func TestJobProgress(t *testing.T) {
	q := NewJobQueue()

	job := q.Enqueue("firmware.bin", "/dev/ttyUSB0")

	q.UpdateProgress(job.ID, 50)
	assert.Equal(t, 50, job.Progress)

	q.UpdateProgress(job.ID, 100)
	assert.Equal(t, 100, job.Progress)
}

func TestJobComplete(t *testing.T) {
	q := NewJobQueue()

	job := q.Enqueue("firmware.bin", "/dev/ttyUSB0")

	q.Complete(job.ID, nil)
	assert.Equal(t, JobComplete, job.Status)
	assert.NotNil(t, job.CompletedAt)

	job2 := q.Enqueue("firmware2.bin", "/dev/ttyUSB1")
	testErr := assert.AnError
	q.Complete(job2.ID, testErr)
	assert.Equal(t, JobFailed, job2.Status)
	assert.NotEmpty(t, job2.Error)
}
