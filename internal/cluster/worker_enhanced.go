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

type WorkerNode struct {
	id        string
	masterURL string
	config    *WorkerConfig
	state     *ClusterState
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mdns      *mDNSService
	watcher   *device.Watcher
	flasher   *flash.Flasher
}

type WorkerConfig struct {
	HeartbeatInterval time.Duration
	HTTPPort          int
	DisablemDNS       bool // For testing
	DisableWatcher    bool // For testing
}

func NewWorkerNode(id, masterURL string, cfg *WorkerConfig) *WorkerNode {
	return &WorkerNode{
		id:        id,
		masterURL: masterURL,
		config:    cfg,
		state:     NewClusterState(),
		flasher:   flash.NewFlasher(nil),
	}
}

func (w *WorkerNode) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	log.Info().Str("node_id", w.id).Str("master", w.masterURL).Msg("Starting worker node")

	// Start mDNS (skip in test mode)
	if !w.config.DisablemDNS {
		w.mdns = NewmDNSService(w.id, "worker", w.config.HTTPPort)
		if err := w.mdns.Start(); err != nil {
			log.Warn().Err(err).Msg("mDNS failed to start")
		}
	}

	// Start device watcher (skip in test mode)
	if !w.config.DisableWatcher {
		w.watcher = device.NewWatcher()
		w.wg.Add(1)
		go w.watchDevices()
	}

	w.wg.Add(1)
	go w.heartbeatLoop()

	return nil
}

func (w *WorkerNode) Stop() error {
	if w.cancel != nil {
		w.cancel()
	}
	if w.watcher != nil {
		w.watcher.Close()
	}
	if w.mdns != nil {
		w.mdns.Stop()
	}
	w.wg.Wait()
	return nil
}

func (w *WorkerNode) State() *ClusterState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state
}

func (w *WorkerNode) watchDevices() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		case event, ok := <-w.watcher.Events():
			if !ok {
				return
			}
			w.handleDeviceEvent(event)
		}
	}
}

func (w *WorkerNode) handleDeviceEvent(event device.DeviceEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch event.Type {
	case device.DeviceAdded:
		dev := &protocol.DeviceInfo{
			Path:   event.Path,
			VID:    event.VID,
			PID:    event.PID,
			NodeID: w.id,
			Status: "available",
		}
		w.state.Devices[event.Path] = dev
		log.Info().Str("path", event.Path).Msg("Device added")

	case device.DeviceRemoved:
		delete(w.state.Devices, event.Path)
		log.Info().Str("path", event.Path).Msg("Device removed")
	}
}

func (w *WorkerNode) heartbeatLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.sendHeartbeat()
		}
	}
}

func (w *WorkerNode) sendHeartbeat() {
	w.mu.RLock()
	deviceCount := len(w.state.Devices)
	w.mu.RUnlock()

	log.Debug().
		Str("node_id", w.id).
		Int("devices", deviceCount).
		Msg("Heartbeat sent")

	// In real implementation, send to master via WebSocket
}

func (w *WorkerNode) ExecuteJob(ctx context.Context, job *Job) error {
	log.Info().Str("job_id", job.ID).Str("device", job.DevicePath).
		Msg("Executing job")

	req := &flash.FlashRequest{
		Port:     job.DevicePath,
		Firmware: []byte("placeholder"), // Would load from job.Firmware
		Progress: make(chan int, 10),
	}

	go func() {
		for progress := range req.Progress {
			w.mu.Lock()
			job.Progress = progress
			w.mu.Unlock()
			log.Debug().Str("job_id", job.ID).Int("progress", progress).
				Msg("Job progress")
		}
	}()

	result := w.flasher.Flash(ctx, req)
	close(req.Progress)

	w.mu.Lock()
	defer w.mu.Unlock()

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
