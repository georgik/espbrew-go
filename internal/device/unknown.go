package device

import (
	"sync"
	"time"
)

// UnknownPort represents a port that failed probing
type UnknownPort struct {
	Path       string
	VID        uint16
	PID        uint16
	Product    string
	Serial     string
	Location   string
	FirstSeen  time.Time
	LastSeen   time.Time
	LastError  string
	ProbeCount int
	PortType   PortType
}

// UnknownTracker tracks ports that fail identification
type UnknownTracker struct {
	ports map[string]*UnknownPort
	mu    sync.RWMutex
}

// NewUnknownTracker creates a new tracker for unidentified ports
func NewUnknownTracker() *UnknownTracker {
	return &UnknownTracker{
		ports: make(map[string]*UnknownPort),
	}
}

// RecordFailure records a failed probe attempt
func (t *UnknownTracker) RecordFailure(path string, vid, pid uint16, product, serial, location, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	p, exists := t.ports[path]
	if !exists {
		p = &UnknownPort{
			Path:      path,
			VID:       vid,
			PID:       pid,
			Product:   product,
			Serial:    serial,
			Location:  location,
			FirstSeen: time.Now(),
		}
		t.ports[path] = p
	}

	p.LastSeen = time.Now()
	p.LastError = errMsg
	p.ProbeCount++
	p.PortType = DetectPortType(vid, pid, product, path)
}

// RecordSuccess removes a port from unknown tracking after successful identification
func (t *UnknownTracker) RecordSuccess(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.ports, path)
}

// List returns all currently unidentified ports
func (t *UnknownTracker) List() []UnknownPort {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]UnknownPort, 0, len(t.ports))
	for _, p := range t.ports {
		result = append(result, *p)
	}
	return result
}

// Get returns a specific unknown port record
func (t *UnknownTracker) Get(path string) (UnknownPort, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	p, exists := t.ports[path]
	if !exists {
		return UnknownPort{}, false
	}
	return *p, true
}

// Remove removes a port from tracking
func (t *UnknownTracker) Remove(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.ports, path)
}

// Cleanup removes entries not seen recently
func (t *UnknownTracker) Cleanup(olderThan time.Duration) []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	var removed []string

	for path, p := range t.ports {
		if p.LastSeen.Before(cutoff) {
			delete(t.ports, path)
			removed = append(removed, path)
		}
	}

	return removed
}

// Count returns the number of tracked unknown ports
func (t *UnknownTracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.ports)
}
