package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/device"
	"codeberg.org/georgik/espbrew-go/internal/flash"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// PeerNode participates in the cluster, discovers local devices, and executes flash jobs.
type PeerNode struct {
	id         string
	leaderURL  string
	config     *PeerConfig
	state      *ClusterState
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mdns       *mDNSService
	watcher    *device.Watcher
	flasher    *flash.Flasher
	cameras    *camera.Discoverer
	registered bool // Tracks successful registration with leader
	mode       protocol.OperationMode
	modeTimer  *time.Timer
	modeCancel context.CancelFunc

	// Job execution tracking
	activeJobs  map[string]context.CancelFunc // jobID -> cancel function
	jobMutex    sync.RWMutex
	progressURL string // URL to report progress to leader
}

// Job tracking errors
var (
	ErrJobNotFound      = fmt.Errorf("job not found")
	ErrJobAlreadyExists = fmt.Errorf("job already exists")
	ErrJobNotOwned      = fmt.Errorf("device not owned by this peer")
)

type PeerConfig struct {
	HeartbeatInterval time.Duration
	HTTPPort          int
	DisablemDNS       bool          // For testing
	DisableWatcher    bool          // For testing
	DiscoveryDuration time.Duration // How long to stay in discovery mode (default 5s)
	InitialMode       string        // Starting mode (default "discovery")
}

func NewPeerNode(id, leaderURL string, cfg *PeerConfig) *PeerNode {
	// Ensure leaderURL has a scheme
	if leaderURL != "" && !startsWithScheme(leaderURL) {
		leaderURL = "http://" + leaderURL
	}

	initialMode := protocol.ModeDiscovery
	if cfg.InitialMode == "operational" {
		initialMode = protocol.ModeOperational
	}

	return &PeerNode{
		id:          id,
		leaderURL:   leaderURL,
		config:      cfg,
		state:       NewClusterState(),
		flasher:     flash.NewFlasher(nil),
		cameras:     camera.NewDiscoverer(),
		mode:        initialMode,
		activeJobs:  make(map[string]context.CancelFunc),
		progressURL: leaderURL, // Use leader URL for progress reporting
	}
}

func startsWithScheme(url string) bool {
	return len(url) >= 7 && (url[:7] == "http://" || (len(url) >= 8 && url[:8] == "https://"))
}

func (p *PeerNode) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	log.Info().Str("node_id", p.id).Str("leader", p.leaderURL).Msg("Starting peer node")

	// Start mDNS (skip in test mode)
	if !p.config.DisablemDNS {
		p.mdns = NewmDNSService(p.id, "peer", p.config.HTTPPort)
		if err := p.mdns.Start(); err != nil {
			log.Warn().Err(err).Msg("mDNS failed to start")
		}
	}

	// Start device watcher (skip in test mode)
	if !p.config.DisableWatcher {
		p.watcher = device.NewWatcher()
		p.wg.Add(1)
		go p.watchDevices()
	}

	// Discover cameras on startup
	p.discoverCameras()

	p.wg.Add(1)
	go p.heartbeatLoop()

	// Start discovery timer if in discovery mode
	if p.mode == protocol.ModeDiscovery {
		duration := p.config.DiscoveryDuration
		if duration == 0 {
			duration = 5 * time.Second // Default 5s
		}
		p.startDiscoveryTimer(duration)
	}

	log.Info().Str("mode", string(p.mode)).Msg("Peer node operational mode set")

	return nil
}

func (p *PeerNode) Stop() error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.watcher != nil {
		p.watcher.Close()
	}
	if p.mdns != nil {
		p.mdns.Stop()
	}
	p.wg.Wait()
	return nil
}

func (p *PeerNode) State() *ClusterState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

func (p *PeerNode) ID() string {
	return p.id
}

func (p *PeerNode) watchDevices() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case event, ok := <-p.watcher.Events():
			if !ok {
				return
			}
			p.handleDeviceEvent(event)
		}
	}
}

func (p *PeerNode) handleDeviceEvent(event device.DeviceEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch event.Type {
	case device.DeviceAdded:
		dev := &protocol.DeviceInfo{
			Path:   event.Path,
			VID:    event.VID,
			PID:    event.PID,
			NodeID: p.id,
			Status: "available",
		}
		p.state.Devices[event.Path] = dev
		log.Info().Str("path", event.Path).Msg("Device added on peer")

	case device.DeviceRemoved:
		delete(p.state.Devices, event.Path)
		log.Info().Str("path", event.Path).Msg("Device removed from peer")
	}
}

func (p *PeerNode) heartbeatLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.sendHeartbeat()
		}
	}
}

func (p *PeerNode) sendHeartbeat() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	devices := make([]*protocol.DeviceInfo, 0, len(p.state.Devices))
	for _, dev := range p.state.Devices {
		devices = append(devices, dev)
	}

	cameras := make([]*protocol.CameraInfo, 0, len(p.state.Cameras))
	for _, cam := range p.state.Cameras {
		cameras = append(cameras, cam)
	}

	payload := &protocol.HeartbeatPayload{
		NodeID:      p.id,
		HTTPPort:    p.config.HTTPPort,
		DeviceCount: len(devices),
		CameraCount: len(cameras),
		ActiveJobs:  0,
		Timestamp:   time.Now().Unix(),
		Devices:     devices,
		Cameras:     cameras,
	}

	log.Info().
		Str("node_id", p.id).
		Str("leader", p.leaderURL).
		Int("devices", payload.DeviceCount).
		Int("cameras", payload.CameraCount).
		Msg("Sending heartbeat to leader")

	// Send heartbeat to leader via HTTP
	if err := p.sendHeartbeatHTTP(payload); err != nil {
		log.Warn().Err(err).Msg("Heartbeat failed")
	}
}

func (p *PeerNode) sendHeartbeatHTTP(payload *protocol.HeartbeatPayload) error {
	// start := time.Now()
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}

	p.mu.RLock()
	isRegistered := p.registered
	p.mu.RUnlock()

	// Register if not already registered
	if !isRegistered {
		registerURL := fmt.Sprintf("%s/api/v1/nodes/register", p.leaderURL)
		req, err := http.NewRequest("POST", registerURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create register request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("register request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			p.mu.Lock()
			p.registered = true
			p.mu.Unlock()
			log.Info().Str("node_id", p.id).Msg("Registered with leader")
			// RecordHeartbeatSuccess(p.id, "send")
			// RecordHeartbeatLatency(p.id, "send", time.Since(start).Seconds())
		} else {
			log.Debug().Str("node_id", p.id).Int("status", resp.StatusCode).
				Msg("Registration attempt failed, will retry")
			return fmt.Errorf("registration failed: %d", resp.StatusCode)
		}
	}

	// Send heartbeat update
	heartbeatURL := fmt.Sprintf("%s/api/v1/nodes/%s/heartbeat", p.leaderURL, p.id)
	req, err := http.NewRequest("POST", heartbeatURL, bytes.NewReader(body))
	if err != nil {
		p.mu.Lock()
		p.registered = false
		p.mu.Unlock()
		return fmt.Errorf("create heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		p.mu.Lock()
		p.registered = false
		p.mu.Unlock()
		return fmt.Errorf("send heartbeat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		p.mu.Lock()
		p.registered = false
		p.mu.Unlock()
		return fmt.Errorf("heartbeat failed: %d", resp.StatusCode)
	}

	// RecordHeartbeatSuccess(p.id, "send")
	// RecordHeartbeatLatency(p.id, "send", time.Since(start).Seconds())
	return nil
}

func (p *PeerNode) ExecuteJob(ctx context.Context, job *Job) error {
	log.Info().Str("job_id", job.ID).Str("device", job.DevicePath).
		Msg("Executing job on peer")

	req := &flash.FlashRequest{
		Port:     job.DevicePath,
		Firmware: []byte("placeholder"), // Would load from job.Firmware
		Progress: make(chan int, 10),
	}

	go func() {
		for progress := range req.Progress {
			p.mu.Lock()
			job.Progress = progress
			p.mu.Unlock()
			log.Debug().Str("job_id", job.ID).Int("progress", progress).
				Msg("Job progress")
		}
	}()

	result := p.flasher.Flash(ctx, req)
	close(req.Progress)

	p.mu.Lock()
	defer p.mu.Unlock()

	if result.Error != nil {
		job.Status = JobFailed
		job.Error = result.Error.Error()
		return fmt.Errorf("flash failed: %w", result.Error)
	}

	job.Status = JobComplete
	job.Progress = 100
	now := time.Now()
	job.CompletedAt = &now

	return nil
}

// AssignJob assigns a job from the leader to this peer.
// It validates device ownership and spawns an execution goroutine.
func (p *PeerNode) AssignJob(ctx context.Context, job *Job) error {
	p.jobMutex.Lock()
	defer p.jobMutex.Unlock()

	// Check if job already exists
	if _, exists := p.activeJobs[job.ID]; exists {
		return ErrJobAlreadyExists
	}

	// Validate device ownership
	p.mu.RLock()
	dev, exists := p.state.Devices[job.DevicePath]
	p.mu.RUnlock()

	if !exists {
		return fmt.Errorf("device not found: %s", job.DevicePath)
	}

	if dev.NodeID != p.id {
		return ErrJobNotOwned
	}

	if dev.Status != "available" {
		return fmt.Errorf("device not available: %s", dev.Status)
	}

	// Create cancellable context for this job
	jobCtx, jobCancel := context.WithCancel(p.ctx)

	// Track the job
	p.activeJobs[job.ID] = jobCancel

	// Mark device as busy
	p.mu.Lock()
	dev.Status = "busy"
	p.state.Devices[job.DevicePath] = dev
	p.mu.Unlock()

	// Spawn execution goroutine
	go p.executeJobAsync(jobCtx, job)

	log.Info().Str("job_id", job.ID).Str("device", job.DevicePath).
		Msg("Job assigned to peer, starting execution")
	return nil
}

// CancelJob cancels an active job on this peer.
func (p *PeerNode) CancelJob(jobID string) error {
	p.jobMutex.Lock()
	defer p.jobMutex.Unlock()

	cancel, exists := p.activeJobs[jobID]
	if !exists {
		return ErrJobNotFound
	}

	// Cancel the job context
	cancel()

	// Remove from tracking
	delete(p.activeJobs, jobID)

	log.Info().Str("job_id", jobID).Msg("Job cancelled on peer")
	return nil
}

// executeJobAsync runs a job asynchronously and reports progress to the leader.
func (p *PeerNode) executeJobAsync(ctx context.Context, job *Job) {
	defer func() {
		// Clean up job tracking
		p.jobMutex.Lock()
		delete(p.activeJobs, job.ID)
		p.jobMutex.Unlock()

		// Release device
		p.mu.Lock()
		if dev, exists := p.state.Devices[job.DevicePath]; exists {
			dev.Status = "available"
			p.state.Devices[job.DevicePath] = dev
		}
		p.mu.Unlock()
	}()

	log.Info().Str("job_id", job.ID).Str("device", job.DevicePath).
		Msg("Starting job execution on peer")

	// Execute the job
	err := p.ExecuteJob(ctx, job)

	// Report final status to leader
	p.reportJobCompletion(job, err)
}

// reportJobCompletion sends the final job status to the leader.
func (p *PeerNode) reportJobCompletion(job *Job, err error) {
	if p.progressURL == "" {
		log.Warn().Str("job_id", job.ID).Msg("No leader URL configured, skipping progress report")
		return
	}

	url := fmt.Sprintf("%s/api/v1/nodes/%s/jobs/%s/progress", p.progressURL, p.id, job.ID)

	payload := map[string]interface{}{
		"job_id":   job.ID,
		"status":   string(job.Status),
		"progress": job.Progress,
		"node_id":  p.id,
	}

	if err != nil {
		payload["error"] = err.Error()
	}

	// Send with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Failed to create progress request")
		return
	}

	req.Header.Set("Content-Type", "application/json")

	body, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Failed to marshal progress payload")
		return
	}

	req.Body = nil // Reset body
	// Re-create with proper body
	req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Failed to create progress request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error().Err(err).Str("job_id", job.ID).Msg("Failed to send progress to leader")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		log.Warn().Str("job_id", job.ID).Int("status", resp.StatusCode).
			Msg("Leader rejected progress update")
		return
	}

	log.Info().Str("job_id", job.ID).Str("status", string(job.Status)).
		Msg("Job completion reported to leader")
}

// discoverCameras scans for available cameras
func (p *PeerNode) discoverCameras() {
	cameras, err := p.cameras.Discover()
	if err != nil {
		log.Warn().Err(err).Msg("Camera discovery failed")
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, cam := range cameras {
		protoCam := &protocol.CameraInfo{
			ID:      cam.ID,
			Name:    cam.Name,
			Path:    cam.Path,
			Backend: string(cam.Backend),
			NodeID:  p.id,
			Status:  "available",
		}
		p.state.Cameras[cam.ID] = protoCam
	}

	log.Info().Int("count", len(cameras)).Msg("Cameras discovered")
}

// GetMode returns the current operational mode
func (p *PeerNode) GetMode() protocol.OperationMode {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.mode
}

// SetMode sets the operational mode and updates discovery state
func (p *PeerNode) SetMode(mode protocol.OperationMode) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.mode == mode {
		return nil // Already in this mode
	}

	log.Info().Str("mode", string(mode)).Str("node_id", p.id).Msg("Switching operational mode")

	oldMode := p.mode
	p.mode = mode

	// Stop existing mode timer if running
	if p.modeTimer != nil {
		p.modeTimer.Stop()
		p.modeTimer = nil
	}
	if p.modeCancel != nil {
		p.modeCancel()
		p.modeCancel = nil
	}

	switch mode {
	case protocol.ModeDiscovery:
		// Enable discovery
		if p.watcher != nil {
			p.watcher.Resume()
		}

		// Set timer to auto-switch to operational mode
		duration := p.config.DiscoveryDuration
		if duration == 0 {
			duration = 5 * time.Second // Default 5s
		}
		p.startDiscoveryTimer(duration)

	case protocol.ModeOperational:
		// Disable discovery
		if p.watcher != nil {
			p.watcher.Pause()
		}

	default:
		log.Warn().Str("mode", string(mode)).Msg("Unknown mode")
		p.mode = oldMode // Revert
		return fmt.Errorf("unknown mode: %s", mode)
	}

	return nil
}

// startDiscoveryTimer starts the timer to auto-switch from discovery to operational mode
func (p *PeerNode) startDiscoveryTimer(duration time.Duration) {
	ctx, cancel := context.WithCancel(p.ctx)
	p.modeCancel = cancel

	p.modeTimer = time.AfterFunc(duration, func() {
		select {
		case <-ctx.Done():
			return
		default:
			log.Info().Dur("duration", duration).Msg("Discovery duration expired, switching to operational mode")
			p.SetMode(protocol.ModeOperational)
		}
	})

	log.Info().Dur("duration", duration).Msg("Discovery timer started")
}
