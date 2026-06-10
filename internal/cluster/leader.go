package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/device"
	"codeberg.org/georgik/espbrew-go/internal/flashhash"
	"codeberg.org/georgik/espbrew-go/internal/inventory"
	"codeberg.org/georgik/espbrew-go/internal/inventory/rom"
	"codeberg.org/georgik/espbrew-go/internal/persistence"
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
	store    *persistence.Store
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mdns     *mDNSService
	watcher  *device.Watcher
}

type LeaderConfig struct {
	HeartbeatInterval  time.Duration
	NodeTimeout        time.Duration
	HTTPPort           int
	DisablemDNS        bool // For testing
	DisableWatcher     bool // For testing
	DisableMaintenance bool // For testing - skips maintenance loop
}

func NewLeaderNode(id string, cfg *LeaderConfig, store *persistence.Store) *LeaderNode {
	return &LeaderNode{
		id:      id,
		config:  cfg,
		state:   NewClusterState(),
		queue:   NewJobQueue(),
		devices: NewDeviceRegistry(),
		store:   store,
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

	if !l.config.DisableMaintenance {
		l.wg.Add(1)
		go l.runMaintenanceLoop()
	}

	// Start camera registry (handles discovery and watching)
	camera.GetRegistry().Start()

	// Wait for initial camera scan to complete
	time.Sleep(500 * time.Millisecond)

	// Discover cameras on startup
	l.discoverCameras()

	// Load persisted devices from store
	l.loadPersistedDevices()

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

func (l *LeaderNode) ID() string {
	return l.id
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
		if payload.HTTPPort > 0 {
			node.Port = payload.HTTPPort
		}
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

			// Process and store flash hash data if present
			if dev.FlashHashes != nil && dev.DeviceID != "" {
				l.processDeviceFlashHashes(dev.DeviceID, dev.FlashHashes)
			}
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

	// Aggregate cameras from peer
	if payload.Cameras != nil {
		for _, cam := range payload.Cameras {
			// Ensure node_id is set correctly
			cam.NodeID = nodeID
			l.state.Cameras[cam.ID] = cam
		}

		// Remove cameras from this node that are no longer in heartbeat
		currentIDs := make(map[string]bool)
		for _, cam := range payload.Cameras {
			currentIDs[cam.ID] = true
		}

		for id, cam := range l.state.Cameras {
			if cam.NodeID == nodeID && !currentIDs[id] {
				delete(l.state.Cameras, id)
				log.Info().Str("camera_id", id).Str("node_id", nodeID).
					Msg("Camera removed from peer")
			}
		}

		log.Debug().
			Str("node_id", nodeID).
			Int("cameras", len(payload.Cameras)).
			Msg("Peer cameras aggregated")
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

// RegisterCamera registers a camera on the leader node
func (l *LeaderNode) RegisterCamera(camera *protocol.CameraInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()

	camera.NodeID = l.id
	l.state.Cameras[camera.ID] = camera
	log.Info().Str("camera_id", camera.ID).Str("name", camera.Name).Str("path", camera.Path).Msg("Camera registered on leader")
}

// GetCameras returns all registered cameras
func (l *LeaderNode) GetCameras() []*protocol.CameraInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	cameras := make([]*protocol.CameraInfo, 0, len(l.state.Cameras))
	for _, cam := range l.state.Cameras {
		cameras = append(cameras, cam)
	}
	return cameras
}

func (l *LeaderNode) EnqueueJob(firmwarePath, devicePath string) (*Job, error) {
	return l.EnqueueJobWithOffset(firmwarePath, devicePath, 0)
}

func (l *LeaderNode) EnqueueJobWithOffset(firmwarePath, devicePath string, offset int) (*Job, error) {
	l.mu.RLock()
	dev, exists := l.state.Devices[devicePath]
	l.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}

	if dev.Disabled {
		return nil, fmt.Errorf("device is disabled: %s", devicePath)
	}

	job := l.queue.Enqueue(firmwarePath, devicePath, offset)

	// Reserve device for this job
	if !l.devices.Reserve(devicePath, job.ID) {
		l.queue.Complete(job.ID, fmt.Errorf("device reservation failed"))
		return nil, fmt.Errorf("device not available: %s", devicePath)
	}

	l.mu.Lock()
	dev = l.state.Devices[devicePath]
	dev.Status = "busy"
	l.state.Devices[devicePath] = dev
	l.mu.Unlock()

	return job, nil
}

func (l *LeaderNode) EnqueueEraseJob(devicePath string, eraseAll bool, address, size uint32) (*Job, error) {
	l.mu.RLock()
	dev, exists := l.state.Devices[devicePath]
	l.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("device not found: %s", devicePath)
	}

	if dev.Disabled {
		return nil, fmt.Errorf("device is disabled: %s", devicePath)
	}

	job := l.queue.EnqueueErase(devicePath, eraseAll, address, size)

	// Reserve device for this job
	if !l.devices.Reserve(devicePath, job.ID) {
		l.queue.Complete(job.ID, fmt.Errorf("device reservation failed"))
		return nil, fmt.Errorf("device not available: %s", devicePath)
	}

	l.mu.Lock()
	dev = l.state.Devices[devicePath]
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
		// Check if device already exists in memory
		existingDev, exists := l.state.Devices[event.Path]

		if exists {
			// Device exists - update VID/PID/Status but preserve identity
			existingDev.VID = event.VID
			existingDev.PID = event.PID
			existingDev.Status = "available"
			l.state.Devices[event.Path] = existingDev
			log.Info().Str("path", event.Path).Str("device_id", existingDev.DeviceID).
				Msg("Device re-connected, preserving identity")
			l.devices.Register(event.Path)
			return
		}

		// Not in memory - check persistence for device with this path
		persisted, err := l.store.GetDeviceByPath(event.Path)
		if err == nil && persisted != nil {
			// Device exists in persistence - restore it
			status := "available"
			if persisted.Disabled {
				status = "disabled"
			}
			dev := &protocol.DeviceInfo{
				Path:            event.Path,
				DeviceID:        persisted.DeviceID,
				ChipType:        persisted.ChipType,
				SerialNumber:    persisted.MACAddress,
				VID:             event.VID,
				PID:             event.PID,
				NodeID:          l.id,
				Status:          status,
				Disabled:        persisted.Disabled,
				DisabledReason:  persisted.DisabledReason,
				DisabledBy:      persisted.DisabledBy,
				DisabledAt:      persisted.DisabledAt,
				Protected:       persisted.Protected,
				ProtectedReason: persisted.ProtectedReason,
				ProtectedBy:     persisted.ProtectedBy,
				ProtectedAt:     persisted.ProtectedAt,
			}
			l.state.Devices[event.Path] = dev
			l.devices.Register(event.Path)
			log.Info().Str("path", event.Path).Str("device_id", persisted.DeviceID).
				Msg("Device restored from persistence")
			return
		}

		// Truly new device - create fresh entry
		dev := &protocol.DeviceInfo{
			Path:   event.Path,
			VID:    event.VID,
			PID:    event.PID,
			NodeID: l.id,
			Status: "available",
		}
		l.state.Devices[event.Path] = dev
		l.devices.Register(event.Path)

		// Quick probe immediately for new devices
		l.wg.Add(1)
		go l.probeDeviceQuickAsync(dev)
		log.Info().Str("path", event.Path).Msg("Device added on leader")

	case device.DeviceRemoved:
		delete(l.state.Devices, event.Path)
		log.Info().Str("path", event.Path).Msg("Device removed from leader")
	}
}

// probeDeviceQuickAsync probes device using boot log (no bootloader entry required)
func (l *LeaderNode) probeDeviceQuickAsync(dev *protocol.DeviceInfo) {
	defer l.wg.Done()

	// Use boot log monitoring (works even if device is running app)
	identity, err := inventory.ProbeFromBootLog(dev.Path)
	if err != nil {
		log.Debug().Str("path", dev.Path).Err(err).Msg("Boot log probe failed (device may not be ESP or not responding)")
		return
	}

	deviceID := rom.DeviceID(identity.MAC)

	l.mu.Lock()
	dev.DeviceID = deviceID
	dev.ChipType = identity.Chip
	dev.SerialNumber = identity.MAC
	l.mu.Unlock()

	// Store in persistence - load existing to preserve FirstSeen
	now := time.Now()
	existing, err := l.store.GetDevice(deviceID)
	var record *persistence.DeviceRecord
	if err == nil && existing != nil {
		// Update existing record
		record = existing
		record.MACAddress = identity.MAC
		record.ChipType = identity.Chip
		record.ChipRev = fmt.Sprintf("%d.%d", identity.ChipMajor, identity.ChipMinor)
		record.FlashSize = identity.FlashSize
		record.PSRAMSize = identity.PSRAMSize
		record.LastSeen = now
		record.LastPath = dev.Path
		record.NodeID = l.id
	} else {
		// New record
		record = &persistence.DeviceRecord{
			DeviceID:   deviceID,
			MACAddress: identity.MAC,
			ChipType:   identity.Chip,
			ChipRev:    fmt.Sprintf("%d.%d", identity.ChipMajor, identity.ChipMinor),
			FlashSize:  identity.FlashSize,
			PSRAMSize:  identity.PSRAMSize,
			FirstSeen:  now,
			LastSeen:   now,
			LastPath:   dev.Path,
			NodeID:     l.id,
		}
	}

	if err := l.store.SaveDevice(record); err != nil {
		log.Warn().Err(err).Msg("Failed to save device to persistence")
		return
	}

	log.Info().
		Str("path", dev.Path).
		Str("device_id", deviceID).
		Str("chip", identity.Chip).
		Msg("Device identified from boot log")
}

// ProbeDevice manually probes a device using boot log monitoring
func (l *LeaderNode) ProbeDevice(path string) (*protocol.DeviceInfo, error) {
	// Use boot log monitoring - resets device and reads boot messages
	identity, err := inventory.ProbeFromBootLog(path)
	if err != nil {
		return nil, fmt.Errorf("boot log probe failed: %w", err)
	}

	deviceID := rom.DeviceID(identity.MAC)

	l.mu.Lock()
	dev, exists := l.state.Devices[path]
	if !exists {
		dev = &protocol.DeviceInfo{
			Path:   path,
			NodeID: l.id,
			Status: "available",
		}
		l.state.Devices[path] = dev
		l.devices.Register(path)
	}
	dev.DeviceID = deviceID
	dev.ChipType = identity.Chip
	dev.SerialNumber = identity.MAC
	l.mu.Unlock()

	// Store in persistence - load existing to preserve FirstSeen
	now := time.Now()
	existing, err := l.store.GetDevice(deviceID)
	var record *persistence.DeviceRecord
	if err == nil && existing != nil {
		// Update existing record
		record = existing
		record.MACAddress = identity.MAC
		record.ChipType = identity.Chip
		record.ChipRev = fmt.Sprintf("%d.%d", identity.ChipMajor, identity.ChipMinor)
		record.FlashSize = identity.FlashSize
		record.PSRAMSize = identity.PSRAMSize
		record.LastSeen = now
		record.LastPath = path
		record.NodeID = l.id
	} else {
		// New record
		record = &persistence.DeviceRecord{
			DeviceID:   deviceID,
			MACAddress: identity.MAC,
			ChipType:   identity.Chip,
			ChipRev:    fmt.Sprintf("%d.%d", identity.ChipMajor, identity.ChipMinor),
			FlashSize:  identity.FlashSize,
			PSRAMSize:  identity.PSRAMSize,
			FirstSeen:  now,
			LastSeen:   now,
			LastPath:   path,
			NodeID:     l.id,
		}
	}

	if err := l.store.SaveDevice(record); err != nil {
		log.Warn().Err(err).Msg("Failed to save device to persistence")
	}

	log.Info().
		Str("path", path).
		Str("device_id", deviceID).
		Str("chip", identity.Chip).
		Msg("Manual probe successful")

	return dev, nil
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

// discoverCameras scans for available cameras and registers them
func (l *LeaderNode) discoverCameras() {
	registry := camera.GetRegistry()
	cameras := registry.List()

	// Filter to keep only index0 (primary interface) per physical camera
	// Most USB cameras expose video-index0 (main) and video-index1 (metadata)
	cameras = filterPrimaryCameras(cameras)

	// Sort cameras alphabetically by name for consistent ordering
	sortCamerasByName(cameras)

	for _, cam := range cameras {
		protoCam := &protocol.CameraInfo{
			ID:      cam.ID,
			Name:    cam.Name,
			Path:    cam.Path,
			Backend: string(cam.Backend),
			NodeID:  l.id,
			Status:  "available",
		}
		l.RegisterCamera(protoCam)
	}

	log.Info().Int("count", len(cameras)).Msg("Cameras discovered")
}

// filterPrimaryCameras keeps only video-index0 (primary interface) per physical camera.
// This filters out video-index1 which is often used for metadata/controls.
func filterPrimaryCameras(cameras []*camera.CameraInfo) []*camera.CameraInfo {
	seen := make(map[string]bool) // base camera name -> already seen
	filtered := make([]*camera.CameraInfo, 0, len(cameras))

	for _, cam := range cameras {
		// Extract base camera name (remove -video-indexN suffix)
		baseName := cam.Name
		if idx := strings.Index(cam.Name, "-video-index"); idx > 0 {
			baseName = cam.Name[:idx]
		}

		// Prefer index0 over index1
		if strings.Contains(cam.Name, "-video-index1") {
			continue
		}

		// Skip if we already have this camera (index0)
		if seen[baseName] {
			continue
		}

		seen[baseName] = true
		filtered = append(filtered, cam)
	}

	return filtered
}

// sortCamerasByName sorts cameras alphabetically by name.
func sortCamerasByName(cameras []*camera.CameraInfo) {
	sort.Slice(cameras, func(i, j int) bool {
		return cameras[i].Name < cameras[j].Name
	})
}

// UpdateDeviceInfo updates device info in cluster state (for manual entry)
func (l *LeaderNode) UpdateDeviceInfo(path, deviceID, chipType, serialNumber string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if dev, exists := l.state.Devices[path]; exists {
		dev.DeviceID = deviceID
		dev.ChipType = chipType
		dev.SerialNumber = serialNumber
		l.state.Devices[path] = dev

		// Persist to store - load existing first to preserve fields
		var record *persistence.DeviceRecord
		existing, err := l.store.GetDevice(deviceID)
		if err == nil && existing != nil {
			// Update existing record, preserving FirstSeen and other fields
			record = existing
			record.DeviceID = deviceID
			record.MACAddress = serialNumber
			record.ChipType = chipType
			record.LastSeen = time.Now()
			record.LastPath = path
			record.NodeID = l.id
		} else {
			// New record
			now := time.Now()
			record = &persistence.DeviceRecord{
				DeviceID:   deviceID,
				MACAddress: serialNumber,
				ChipType:   chipType,
				FirstSeen:  now,
				LastSeen:   now,
				LastPath:   path,
				NodeID:     l.id,
			}
		}

		if err := l.store.SaveDevice(record); err != nil {
			log.Warn().Err(err).Msg("Failed to persist device info update")
		}

		log.Info().
			Str("path", path).
			Str("device_id", deviceID).
			Str("chip", chipType).
			Msg("Device info updated in cluster state")
	}
}

// loadPersistedDevices restores devices from persistence store on startup
func (l *LeaderNode) loadPersistedDevices() {
	devices, err := l.store.ListDevices()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to list devices from store")
		return
	}

	loaded := 0
	for _, record := range devices {
		// For now, only restore devices that were last seen on this node
		// Using LastPath as the device identifier (user's approach for early phase)
		if record.NodeID != l.id {
			continue
		}
		if record.LastPath == "" {
			continue
		}

		l.mu.Lock()
		dev := &protocol.DeviceInfo{
			Path:            record.LastPath,
			DeviceID:        record.DeviceID,
			ChipType:        record.ChipType,
			SerialNumber:    record.MACAddress,
			NodeID:          l.id,
			Status:          "available", // Assume available on restart
			Disabled:        record.Disabled,
			DisabledReason:  record.DisabledReason,
			DisabledBy:      record.DisabledBy,
			DisabledAt:      record.DisabledAt,
			Protected:       record.Protected,
			ProtectedReason: record.ProtectedReason,
			ProtectedBy:     record.ProtectedBy,
			ProtectedAt:     record.ProtectedAt,
		}
		if record.Disabled {
			dev.Status = "disabled"
		}
		l.state.Devices[record.LastPath] = dev
		l.devices.Register(record.LastPath)
		l.mu.Unlock()

		loaded++
	}

	log.Info().Int("count", loaded).Msg("Devices loaded from persistence")
}

// UpdateDeviceStatus updates the status of a device in the cluster state
func (l *LeaderNode) UpdateDeviceStatus(deviceID string, status string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for path, dev := range l.state.Devices {
		if dev.DeviceID == deviceID || dev.Path == deviceID {
			dev.Status = status
			l.state.Devices[path] = dev
			log.Debug().Str("device_id", deviceID).Str("status", status).Msg("Device status updated")
			return
		}
	}
}

// UpdateDeviceDisabled updates the disabled state of a device
func (l *LeaderNode) UpdateDeviceDisabled(deviceID string, disabled bool, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for path, dev := range l.state.Devices {
		if dev.DeviceID == deviceID || dev.Path == deviceID {
			dev.Disabled = disabled
			dev.DisabledReason = reason
			if disabled {
				dev.Status = "disabled"
			} else {
				dev.Status = "available"
			}
			l.state.Devices[path] = dev
			log.Info().Str("device_id", deviceID).Bool("disabled", disabled).Str("reason", reason).Msg("Device disabled state updated")
			return
		}
	}
}

// UpdateDeviceProtected updates the protected state of a device
func (l *LeaderNode) UpdateDeviceProtected(deviceID string, protected bool, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for path, dev := range l.state.Devices {
		if dev.DeviceID == deviceID || dev.Path == deviceID {
			dev.Protected = protected
			dev.ProtectedReason = reason
			l.state.Devices[path] = dev
			log.Info().Str("device_id", deviceID).Bool("protected", protected).Str("reason", reason).Msg("Device protected state updated")
			return
		}
	}
}

// processDeviceFlashHashes stores flash hash data for a device from peer heartbeat
func (l *LeaderNode) processDeviceFlashHashes(deviceID string, hashes *protocol.DeviceFlashHashes) {
	// Create a job hash record for storage
	// Use device ID as job ID for the "latest" device hashes
	jobHashes := &flashhash.JobFlashHashes{
		JobID:     "device-" + deviceID,
		DeviceID:  deviceID,
		Regions:   hashes.Regions,
		CreatedAt: hashes.UpdatedAt,
	}

	if err := l.store.SaveFlashHashes(jobHashes); err != nil {
		log.Warn().Err(err).Str("device_id", deviceID).Msg("Failed to save device flash hashes")
		return
	}

	log.Debug().
		Str("device_id", deviceID).
		Int("regions", len(hashes.Regions)).
		Msg("Device flash hashes stored from heartbeat")
}

// GetDeviceFlashHashes retrieves the latest flash hashes for a device
func (l *LeaderNode) GetDeviceFlashHashes(deviceID string) (*flashhash.JobFlashHashes, error) {
	// Try to get the device-specific hashes first
	jobID := "device-" + deviceID
	hashes, err := l.store.GetFlashHashes(jobID)
	if err == nil {
		return hashes, nil
	}

	// Fall back to listing hashes for device (may have job-specific hashes)
	hashList, err := l.store.ListFlashHashesForDevice(deviceID)
	if err != nil || len(hashList) == 0 {
		return nil, err
	}

	// Return most recent
	return hashList[0], nil
}

// StoreJobFlashHashes stores flash hashes for a specific job
func (l *LeaderNode) StoreJobFlashHashes(jobID, deviceID string, regions []flashhash.FlashRegionInfo) error {
	hashes := &flashhash.JobFlashHashes{
		JobID:     jobID,
		DeviceID:  deviceID,
		Regions:   regions,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	return l.store.SaveFlashHashes(hashes)
}
