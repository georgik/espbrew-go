package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/georgik/esp-ci-cluster/pkg/protocol"
	"github.com/rs/zerolog/log"
)

type MasterNode struct {
	id       string
	config   *MasterConfig
	state    *ClusterState
	queue    *JobQueue
	executor *JobExecutor
	devices  *DeviceRegistry
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mdns     *mDNSService
}

type MasterConfig struct {
	HeartbeatInterval time.Duration
	NodeTimeout       time.Duration
	HTTPPort          int
	DisablemDNS       bool // For testing
}

func NewMasterNode(id string, cfg *MasterConfig) *MasterNode {
	return &MasterNode{
		id:      id,
		config:  cfg,
		state:   NewClusterState(),
		queue:   NewJobQueue(),
		devices: NewDeviceRegistry(),
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

	m.wg.Add(1)
	go m.runMaintenanceLoop()

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
	m.devices.Register(device.Path)
	log.Info().Str("path", device.Path).Msg("Device registered on master")
}

func (m *MasterNode) EnqueueJob(firmwarePath, devicePath string) (*Job, error) {
	m.mu.RLock()
	_, exists := m.state.Devices[devicePath]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}

	job := m.queue.Enqueue(firmwarePath, devicePath)

	// Reserve device for this job
	if !m.devices.Reserve(devicePath, job.ID) {
		m.queue.Complete(job.ID, fmt.Errorf("device reservation failed"))
		return nil, fmt.Errorf("device not available: %s", devicePath)
	}

	m.mu.Lock()
	dev := m.state.Devices[devicePath]
	dev.Status = "busy"
	m.state.Devices[devicePath] = dev
	m.mu.Unlock()

	return job, nil
}

func (m *MasterNode) GetJobQueue() *JobQueue {
	return m.queue
}

func (m *MasterNode) GetDevices() *DeviceRegistry {
	return m.devices
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

func (m *MasterNode) runMaintenanceLoop() {
	defer m.wg.Done()

	// Run maintenance every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Also run once on startup after a short delay
	time.Sleep(10 * time.Second)
	m.performMaintenance()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performMaintenance()
		}
	}
}

func (m *MasterNode) performMaintenance() {
	// Clean up stale device reservations (older than 30 minutes)
	staleReservations := m.devices.CleanupStaleReservations(30 * time.Minute)

	// Clean up old jobs (older than 24 hours)
	oldJobs := m.queue.CleanupOld(DefaultJobTTL)

	// Release devices for cancelled/timeout jobs
	m.cleanupOrphanedDevices()

	if staleReservations > 0 || oldJobs > 0 {
		log.Info().
			Int("stale_reservations", staleReservations).
			Int("old_jobs", oldJobs).
			Msg("Maintenance completed")
	}
}

func (m *MasterNode) cleanupOrphanedDevices() {
	jobs := m.queue.List()

	// Track active job devices
	activeDevices := make(map[string]string)
	for _, job := range jobs {
		job.RLock()
		if job.Status == JobPending || job.Status == JobAssigned || job.Status == JobRunning {
			activeDevices[job.DevicePath] = job.ID
		}
		job.RUnlock()
	}

	// Find devices marked busy but no active job
	m.mu.Lock()
	for path, dev := range m.state.Devices {
		if dev.Status == "busy" {
			if _, isActive := activeDevices[path]; !isActive {
				log.Warn().Str("path", path).Msg("Releasing orphaned busy device")
				dev.Status = "available"
				m.state.Devices[path] = dev
				m.devices.Release(path, "")
			}
		}
	}
	m.mu.Unlock()
}
