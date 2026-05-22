package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/georgik/esp-ci-cluster/internal/device"
	"github.com/georgik/esp-ci-cluster/internal/flash"
	"github.com/georgik/esp-ci-cluster/pkg/protocol"
	"github.com/rs/zerolog/log"
)

// PeerNode participates in the cluster, discovers local devices, and executes flash jobs.
type PeerNode struct {
	id        string
	leaderURL string
	config    *PeerConfig
	state     *ClusterState
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mdns      *mDNSService
	watcher   *device.Watcher
	flasher   *flash.Flasher
}

type PeerConfig struct {
	HeartbeatInterval time.Duration
	HTTPPort          int
	DisablemDNS       bool // For testing
	DisableWatcher    bool // For testing
}

func NewPeerNode(id, leaderURL string, cfg *PeerConfig) *PeerNode {
	return &PeerNode{
		id:        id,
		leaderURL: leaderURL,
		config:    cfg,
		state:     NewClusterState(),
		flasher:   flash.NewFlasher(nil),
	}
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

	payload := &protocol.HeartbeatPayload{
		NodeID:      p.id,
		DeviceCount: len(devices),
		ActiveJobs:  0,
		Timestamp:   time.Now().Unix(),
		Devices:     devices,
	}

	log.Debug().
		Str("node_id", p.id).
		Int("devices", payload.DeviceCount).
		Msg("Heartbeat sent")

	// In real implementation, send payload to leader via WebSocket/HTTP
	_ = payload
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
