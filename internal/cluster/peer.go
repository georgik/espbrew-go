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
}

type PeerConfig struct {
	HeartbeatInterval time.Duration
	HTTPPort          int
	DisablemDNS       bool // For testing
	DisableWatcher    bool // For testing
}

func NewPeerNode(id, leaderURL string, cfg *PeerConfig) *PeerNode {
	// Ensure leaderURL has a scheme
	if leaderURL != "" && !startsWithScheme(leaderURL) {
		leaderURL = "http://" + leaderURL
	}

	return &PeerNode{
		id:        id,
		leaderURL: leaderURL,
		config:    cfg,
		state:     NewClusterState(),
		flasher:   flash.NewFlasher(nil),
		cameras:   camera.NewDiscoverer(),
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
			Backend: string(cam.Backend),
			NodeID:  p.id,
			Status:  "available",
		}
		p.state.Cameras[cam.ID] = protoCam
	}

	log.Info().Int("count", len(cameras)).Msg("Cameras discovered")
}
