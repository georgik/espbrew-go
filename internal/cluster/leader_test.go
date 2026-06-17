package cluster

import (
	"context"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/persistence"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/stretchr/testify/assert"
)

func TestLeaderNodeRegistersDevice(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	leader := NewLeaderNode("test-leader", &LeaderConfig{
		HeartbeatInterval:  time.Second,
		NodeTimeout:        5 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true, // Disable mDNS in tests
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	ctx := context.Background()
	assert.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	dev := &protocol.DeviceInfo{
		Path:   "/dev/ttyUSB0",
		VID:    0x4348,
		PID:    0x0027,
		NodeID: "peer-1",
		Status: "available",
	}

	leader.state.Devices[dev.Path] = dev

	state := leader.State()
	assert.Equal(t, 5, len(state.Devices)) // 4 virtual devices + 1 registered
	assert.Equal(t, dev, state.Devices[dev.Path])
}

func TestLeaderNodeTimeoutCleanup(t *testing.T) {
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	leader := NewLeaderNode("test-leader", &LeaderConfig{
		HeartbeatInterval:  100 * time.Millisecond,
		NodeTimeout:        200 * time.Millisecond,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
	}, store)

	ctx := context.Background()
	assert.NoError(t, leader.Start(ctx))
	defer leader.Stop()

	node := &protocol.NodeInfo{
		ID:       "stale-node",
		Address:  "127.0.0.1",
		Role:     "peer",
		LastSeen: time.Now().Add(-1 * time.Minute),
	}
	leader.RegisterNode(node)

	// Leader node + 1 registered node = 2 total
	assert.Equal(t, 2, len(leader.State().Nodes))

	time.Sleep(300 * time.Millisecond)

	// Only leader node should remain (stale node cleaned up)
	assert.Equal(t, 1, len(leader.State().Nodes), "stale node should be cleaned up")
}

func TestPeerNodeHeartbeat(t *testing.T) {
	peer := NewPeerNode("test-peer", "ws://leader:8080", &PeerConfig{
		HeartbeatInterval: 100 * time.Millisecond,
		HTTPPort:          8081,
		DisablemDNS:       true,
		DisableWatcher:    true,
	})

	ctx := context.Background()
	assert.NoError(t, peer.Start(ctx))
	defer peer.Stop()

	assert.Equal(t, "test-peer", peer.id)
	assert.Equal(t, "http://ws://leader:8080", peer.leaderURL)
}

func TestJobQueue(t *testing.T) {
	q := NewJobQueue()

	job1 := q.Enqueue("firmware1.bin", "/dev/ttyUSB0", 0)
	job2 := q.Enqueue("firmware2.bin", "/dev/ttyUSB1", 0)

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

	job := q.Enqueue("firmware.bin", "/dev/ttyUSB0", 0)

	q.UpdateProgress(job.ID, 50)
	assert.Equal(t, 50, job.Progress)

	q.UpdateProgress(job.ID, 100)
	assert.Equal(t, 100, job.Progress)
}

func TestJobComplete(t *testing.T) {
	q := NewJobQueue()

	job := q.Enqueue("firmware.bin", "/dev/ttyUSB0", 0)

	q.Complete(job.ID, nil)
	assert.Equal(t, JobComplete, job.Status)
	assert.NotNil(t, job.CompletedAt)

	job2 := q.Enqueue("firmware2.bin", "/dev/ttyUSB1", 0)
	testErr := assert.AnError
	q.Complete(job2.ID, testErr)
	assert.Equal(t, JobFailed, job2.Status)
	assert.NotEmpty(t, job2.Error)
}
