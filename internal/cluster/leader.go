package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/device"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// Virtual device definitions
var virtualDevices = []struct {
	path      string
	chip      string
	flashSize string
	label     string
}{
	{path: "wokwi-esp32s3", chip: "esp32s3", flashSize: "16MB", label: "Wokwi ESP32-S3"},
	{path: "wokwi-esp32", chip: "esp32", flashSize: "4MB", label: "Wokwi ESP32"},
	{path: "wokwi-esp32c3", chip: "esp32c3", flashSize: "4MB", label: "Wokwi ESP32-C3"},
	{path: "wokwi-esp32c6", chip: "esp32c6", flashSize: "8MB", label: "Wokwi ESP32-C6"},
}

// LeaderNode coordinates the cluster, discovers local devices, and aggregates state from peers.
type LeaderNode struct {
	id       string
	config   *LeaderConfig
	state    *ClusterState
	queue    *JobQueue
	executor *JobExecutor
	devices  *DeviceRegistry
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mdns     *mDNSService
	watcher  *device.Watcher
}

type LeaderConfig struct {
	HeartbeatInterval time.Duration
	NodeTimeout       time.Duration
	HTTPPort          int
	DisablemDNS       bool // For testing
	DisableWatcher    bool // For testing
}

func NewLeaderNode(id string, cfg *LeaderConfig) *LeaderNode {
	return &LeaderNode{
		id:      id,
		config:  cfg,
		state:   NewClusterState(),
		queue:   NewJobQueue(),
		devices: NewDeviceRegistry(),
	}
}

func (l *LeaderNode) Start(ctx context.Context) error {
	l.ctx, l.cancel = context.WithCancel(ctx)

	log.Info().Str("node_id", l.id).Msg("Starting leader node")

	// Start mDNS (skip in test mode)
	if !l.config.DisablemDNS {
		l.mdns = NewmDNSService(l.id, "leader", l.config.HTTPPort)
		if err := l.mdns.Start(); err != nil {
			log.Warn().Err(err).Msg("mDNS failed to start")
		}
	}

	// Start device watcher (skip in test mode)
	if !l.config.DisableWatcher {
		l.watcher = device.NewWatcher()
		l.wg.Add(1)
		go l.watchDevices()
	}

	l.wg.Add(1)
	go l.runCleanupLoop()

	l.wg.Add(1)
	go l.runJobDispatcher()

	l.wg.Add(1)
	go l.runMaintenanceLoop()

	// Register virtual devices
	l.registerVirtualDevices()

	return nil
}

func (l *LeaderNode) Stop() error {
	if l.cancel != nil {
		l.cancel()
	}
	if l.watcher != nil {
		l.watcher.Close()
	}
	if l.mdns != nil {
		l.mdns.Stop()
	}
	l.wg.Wait()
	return nil
}

func (l *LeaderNode) State() *ClusterState {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.state
}

func (l *LeaderNode) RegisterNode(node *protocol.NodeInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()

	node.LastSeen = time.Now()
	l.state.Nodes[node.ID] = node
	log.Info().Str("node_id", node.ID).Msg("Node registered")
}

func (l *LeaderNode) UnregisterNode(nodeID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.state.Nodes, nodeID)

	for path, dev := range l.state.Devices {
		if dev.NodeID == nodeID {
			delete(l.state.Devices, path)
		}
	}

	log.Info().Str("node_id", nodeID).Msg("Node unregistered")
}

func (l *LeaderNode) UpdateHeartbeat(nodeID string, payload *protocol.HeartbeatPayload) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if node, exists := l.state.Nodes[nodeID]; exists {
		node.LastSeen = time.Now()
		log.Debug().Str("node_id", nodeID).Time("last_seen", node.LastSeen).
			Msg("Heartbeat received, LastSeen updated")
	} else {
		log.Warn().Str("node_id", nodeID).
			Msg("Heartbeat received from unknown node - not registered")
	}

	// Aggregate devices from peer
	if payload.Devices != nil {
		for _, dev := range payload.Devices {
			// Ensure node_id is set correctly
			dev.NodeID = nodeID
			l.state.Devices[dev.Path] = dev

			// Register device in local registry (idempotent)
			l.devices.Register(dev.Path)
		}

		// Remove devices from this node that are no longer in heartbeat
		currentPaths := make(map[string]bool)
		for _, dev := range payload.Devices {
			currentPaths[dev.Path] = true
		}

		for path, dev := range l.state.Devices {
			if dev.NodeID == nodeID && !currentPaths[path] {
				delete(l.state.Devices, path)
				log.Info().Str("path", path).Str("node_id", nodeID).
					Msg("Device removed from peer")
			}
		}

		log.Debug().
			Str("node_id", nodeID).
			Int("devices", len(payload.Devices)).
			Msg("Peer devices aggregated")
	}
}

func (l *LeaderNode) RegisterDevice(device *protocol.DeviceInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()

	device.NodeID = l.id
	l.state.Devices[device.Path] = device
	l.devices.Register(device.Path)
	log.Info().Str("path", device.Path).Msg("Device registered on leader")
}

func (l *LeaderNode) EnqueueJob(firmwarePath, devicePath string) (*Job, error) {
	return l.EnqueueJobWithOffset(firmwarePath, devicePath, 0)
}

func (l *LeaderNode) EnqueueJobWithOffset(firmwarePath, devicePath string, offset int) (*Job, error) {
	l.mu.RLock()
	_, exists := l.state.Devices[devicePath]
	l.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}

	job := l.queue.Enqueue(firmwarePath, devicePath, offset)

	// Reserve device for this job
	if !l.devices.Reserve(devicePath, job.ID) {
		l.queue.Complete(job.ID, fmt.Errorf("device reservation failed"))
		return nil, fmt.Errorf("device not available: %s", devicePath)
	}

	l.mu.Lock()
	dev := l.state.Devices[devicePath]
	dev.Status = "busy"
	l.state.Devices[devicePath] = dev
	l.mu.Unlock()

	return job, nil
}

func (l *LeaderNode) GetJobQueue() *JobQueue {
	return l.queue
}

func (l *LeaderNode) GetDevices() *DeviceRegistry {
	return l.devices
}

func (l *LeaderNode) GetPeers() []*PeerInfo {
	if l.mdns == nil {
		return nil
	}
	return l.mdns.Peers()
}

func (l *LeaderNode) watchDevices() {
	defer l.wg.Done()

	for {
		select {
		case <-l.ctx.Done():
			return
		case event, ok := <-l.watcher.Events():
			if !ok {
				return
			}
			l.handleDeviceEvent(event)
		}
	}
}

func (l *LeaderNode) handleDeviceEvent(event device.DeviceEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()

	switch event.Type {
	case device.DeviceAdded:
		dev := &protocol.DeviceInfo{
			Path:   event.Path,
			VID:    event.VID,
			PID:    event.PID,
			NodeID: l.id,
			Status: "available",
		}
		l.state.Devices[event.Path] = dev
		l.devices.Register(event.Path)
		log.Info().Str("path", event.Path).Msg("Device added on leader")

	case device.DeviceRemoved:
		delete(l.state.Devices, event.Path)
		log.Info().Str("path", event.Path).Msg("Device removed from leader")
	}
}

func (l *LeaderNode) registerVirtualDevices() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, vdev := range virtualDevices {
		dev := &protocol.DeviceInfo{
			Path:   vdev.path,
			NodeID: l.id,
			Status: "available",
		}
		l.state.Devices[vdev.path] = dev
		l.devices.Register(vdev.path)
		log.Info().Str("path", vdev.path).Str("chip", vdev.chip).Str("label", vdev.label).
			Msg("Virtual device registered")
	}
}

func (l *LeaderNode) runCleanupLoop() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			l.cleanupStaleNodes()
		}
	}
}

func (l *LeaderNode) cleanupStaleNodes() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for id, node := range l.state.Nodes {
		if now.Sub(node.LastSeen) > l.config.NodeTimeout {
			log.Warn().Str("node_id", id).Msg("Node timed out")
			delete(l.state.Nodes, id)

			for path, dev := range l.state.Devices {
				if dev.NodeID == id {
					delete(l.state.Devices, path)
				}
			}
		}
	}
}

func (l *LeaderNode) runJobDispatcher() {
	defer l.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			l.dispatchJobs()
		}
	}
}

func (l *LeaderNode) dispatchJobs() {
	if l.queue.PendingCount() == 0 {
		return
	}

	// Find available devices
	l.mu.RLock()
	available := make([]string, 0)
	for path, dev := range l.state.Devices {
		if dev.Status == "available" {
			available = append(available, path)
		}
	}
	l.mu.RUnlock()

	if len(available) == 0 {
		return
	}

	// Assign job to device
	job := l.queue.Dequeue(l.id)
	if job == nil {
		return
	}

	l.mu.Lock()
	dev := l.state.Devices[job.DevicePath]
	dev.Status = "busy"
	l.state.Devices[job.DevicePath] = dev
	l.mu.Unlock()

	log.Info().Str("job_id", job.ID).Str("device", job.DevicePath).
		Msg("Job assigned to device")

	// Submit to executor
	if l.executor != nil {
		l.executor.Submit(job)
	} else {
		// No executor, mark as running (for testing)
		l.queue.UpdateProgress(job.ID, 50)
	}
}

func (l *LeaderNode) runMaintenanceLoop() {
	defer l.wg.Done()

	// Run maintenance every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Also run once on startup after a short delay
	time.Sleep(10 * time.Second)
	l.performMaintenance()

	for {
		select {
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			l.performMaintenance()
		}
	}
}

func (l *LeaderNode) performMaintenance() {
	// Clean up stale device reservations (older than 30 minutes)
	staleReservations := l.devices.CleanupStaleReservations(30 * time.Minute)

	// Clean up old jobs (older than 24 hours)
	oldJobs := l.queue.CleanupOld(DefaultJobTTL)

	// Release devices for cancelled/timeout jobs
	l.cleanupOrphanedDevices()

	if staleReservations > 0 || oldJobs > 0 {
		log.Info().
			Int("stale_reservations", staleReservations).
			Int("old_jobs", oldJobs).
			Msg("Maintenance completed")
	}
}

func (l *LeaderNode) cleanupOrphanedDevices() {
	jobs := l.queue.List()

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
	l.mu.Lock()
	for path, dev := range l.state.Devices {
		if dev.Status == "busy" {
			if _, isActive := activeDevices[path]; !isActive {
				log.Warn().Str("path", path).Msg("Releasing orphaned busy device")
				dev.Status = "available"
				l.state.Devices[path] = dev
				l.devices.Release(path, "")
			}
		}
	}
	l.mu.Unlock()
}
