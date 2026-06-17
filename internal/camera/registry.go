package camera

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Registry manages camera discovery with caching and device watching
type Registry struct {
	discoverer *Discoverer
	mu         sync.RWMutex
	cameras    map[string]*CameraInfo
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	interval   time.Duration
	paused     bool
}

var (
	// global registry instance
	globalRegistry *Registry
	once           sync.Once
)

// GetRegistry returns the global camera registry (singleton)
func GetRegistry() *Registry {
	once.Do(func() {
		globalRegistry = NewRegistry()
		globalRegistry.Start()
	})
	return globalRegistry
}

// NewRegistry creates a new camera registry
func NewRegistry() *Registry {
	ctx, cancel := context.WithCancel(context.Background())
	return &Registry{
		discoverer: NewDiscoverer(),
		cameras:    make(map[string]*CameraInfo),
		ctx:        ctx,
		cancel:     cancel,
		interval:   5 * time.Second,
	}
}

// Start begins the camera watcher
func (r *Registry) Start() {
	r.wg.Add(1)
	go r.watch()
	log.Info().Msg("Camera registry started")
}

// Stop stops the camera watcher
func (r *Registry) Stop() {
	r.cancel()
	r.wg.Wait()
	log.Info().Msg("Camera registry stopped")
}

// watch periodically scans for camera changes
func (r *Registry) watch() {
	defer r.wg.Done()

	// Initial scan
	r.scan()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.scan()
		}
	}
}

// scan updates cached camera list
func (r *Registry) scan() {
	// Skip scanning if registry is paused
	if r.IsPaused() {
		return
	}

	cameras, err := r.discoverer.Discover()
	if err != nil {
		log.Debug().Err(err).Msg("Camera scan failed")
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Build new camera map
	newCameras := make(map[string]*CameraInfo)
	for _, cam := range cameras {
		newCameras[cam.ID] = cam
	}

	// Detect changes
	for id, cam := range newCameras {
		if existing, ok := r.cameras[id]; !ok {
			log.Info().
				Str("camera_id", id).
				Str("name", cam.Name).
				Msg("Camera detected")
		} else if existing.Path != cam.Path {
			log.Info().
				Str("camera_id", id).
				Str("old_path", existing.Path).
				Str("new_path", cam.Path).
				Msg("Camera path changed")
		}
	}

	// Detect removals
	for id := range r.cameras {
		if _, ok := newCameras[id]; !ok {
			log.Info().
				Str("camera_id", id).
				Msg("Camera removed")
		}
	}

	r.cameras = newCameras
}

// List returns all cached cameras
func (r *Registry) List() []*CameraInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cameras := make([]*CameraInfo, 0, len(r.cameras))
	for _, cam := range r.cameras {
		cameras = append(cameras, cam)
	}
	return cameras
}

// GetByID finds a camera by ID
func (r *Registry) GetByID(id string) (*CameraInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cam, ok := r.cameras[id]
	return cam, ok
}

// GetByPath finds a camera by device path
func (r *Registry) GetByPath(path string) (*CameraInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cam := range r.cameras {
		if cam.Path == path {
			return cam, true
		}
	}
	return nil, false
}

// GetNameByID returns camera name for given camera ID
func (r *Registry) GetNameByID(id string) string {
	if id == "" || id == "unknown" {
		return "Unknown Camera"
	}
	if cam, ok := r.GetByID(id); ok {
		return cam.Name
	}
	// Fallback to shortened ID
	if len(id) >= 8 {
		return "Camera " + id[:8]
	}
	return "Camera " + id
}

// Refresh forces an immediate re-scan
func (r *Registry) Refresh() {
	r.scan()
}

// Pause pauses the camera registry (stops scanning)
func (r *Registry) Pause() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = true
	log.Info().Msg("Camera registry paused")
}

// Resume resumes the camera registry
func (r *Registry) Resume() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paused = false
	log.Info().Msg("Camera registry resumed")
}

// IsPaused returns whether the registry is paused
func (r *Registry) IsPaused() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.paused
}
