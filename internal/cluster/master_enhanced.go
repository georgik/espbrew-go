package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/georgik/esp-ci-cluster/pkg/protocol"
)

type MasterNode struct {
	id        string
	config    *MasterConfig
	state     *ClusterState
	queue     *JobQueue
	executor  *JobExecutor
	devices   *DeviceRegistry
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mdns      *mDNSService
}

type MasterConfig struct {
	HeartbeatInterval time.Duration
	NodeTimeout       time.Duration
	HTTPPort          int
	DisablemDNS       bool // For testing
}

func NewMasterNode(id string, cfg *MasterConfig) *MasterNode {
	return &MasterNode{
		id:     id,
		config: cfg,
		state:  NewClusterState(),
		queue:  NewJobQueue(),
	}
}

func (m *MasterNode) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	log.Info().Str("node_id", m.id).Msg("Starting master node")

	// Start mDNS (skip in test mode)
	if !m.config.DisablemDNS {
		m.mdns = NewmDNSService(m.id, "master", m.config.HTTPPort)
		if err := m.mdns.Start(); err != nil {
			log.Warn().Err(err).Msg("mDNS failed to start")
		}
	}

	m.wg.Add(1)
	go m.runCleanupLoop()

	m.wg.Add(1)
	go m.runJobDispatcher()

	return nil
}

func (m *MasterNode) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.mdns != nil {
		m.mdns.Stop()
	}
	m.wg.Wait()
	return nil
}

func (m *MasterNode) State() *ClusterState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *MasterNode) RegisterNode(node *protocol.NodeInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	node.LastSeen = time.Now()
	m.state.Nodes[node.ID] = node
	log.Info().Str("node_id", node.ID).Msg("Node registered")
}

func (m *MasterNode) UnregisterNode(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.state.Nodes, nodeID)

	for path, dev := range m.state.Devices {
		if dev.NodeID == nodeID {
			delete(m.state.Devices, path)
		}
	}

	log.Info().Str("node_id", nodeID).Msg("Node unregistered")
}

func (m *MasterNode) UpdateHeartbeat(nodeID string, payload *protocol.HeartbeatPayload) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, exists := m.state.Nodes[nodeID]; exists {
		node.LastSeen = time.Now()
	}
}

func (m *MasterNode) RegisterDevice(device *protocol.DeviceInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	device.NodeID = m.id
	m.state.Devices[device.Path] = device
	log.Info().Str("path", device.Path).Msg("Device registered on master")
}

func (m *MasterNode) EnqueueJob(firmwarePath, devicePath string) (*Job, error) {
	m.mu.Lock()
	dev, exists := m.state.Devices[devicePath]
	m.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}

	if dev.Status != "available" {
		return nil, fmt.Errorf("device not available: %s (status: %s)", devicePath, dev.Status)
	}

	job := m.queue.Enqueue(firmwarePath, devicePath)

	m.mu.Lock()
	dev.Status = "busy"
	m.state.Devices[devicePath] = dev
	m.mu.Unlock()

	return job, nil
}

func (m *MasterNode) GetJobQueue() *JobQueue {
	return m.queue
}

func (m *MasterNode) GetPeers() []*PeerInfo {
	if m.mdns == nil {
		return nil
	}
	return m.mdns.Peers()
}

func (m *MasterNode) runCleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupStaleNodes()
		}
	}
}

func (m *MasterNode) cleanupStaleNodes() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, node := range m.state.Nodes {
		if now.Sub(node.LastSeen) > m.config.NodeTimeout {
			log.Warn().Str("node_id", id).Msg("Node timed out")
			delete(m.state.Nodes, id)

			for path, dev := range m.state.Devices {
				if dev.NodeID == id {
					delete(m.state.Devices, path)
				}
			}
		}
	}
}

func (m *MasterNode) runJobDispatcher() {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.dispatchJobs()
		}
	}
}

func (m *MasterNode) dispatchJobs() {
	if m.queue.PendingCount() == 0 {
		return
	}

	// Find available devices
	m.mu.RLock()
	available := make([]string, 0)
	for path, dev := range m.state.Devices {
		if dev.Status == "available" {
			available = append(available, path)
		}
	}
	m.mu.RUnlock()

	if len(available) == 0 {
		return
	}

	// Assign job to device
	job := m.queue.Dequeue(m.id)
	if job == nil {
		return
	}

	m.mu.Lock()
	dev := m.state.Devices[job.DevicePath]
	dev.Status = "busy"
	m.state.Devices[job.DevicePath] = dev
	m.mu.Unlock()

	log.Info().Str("job_id", job.ID).Str("device", job.DevicePath).
		Msg("Job assigned to device")

	// Submit to executor
	if m.executor != nil {
		m.executor.Submit(job)
	} else {
		// No executor, mark as running (for testing)
		m.queue.UpdateProgress(job.ID, 50)
	}
}
