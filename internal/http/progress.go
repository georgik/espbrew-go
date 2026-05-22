package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/georgik/esp-ci-cluster/internal/cluster"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var progressUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type ProgressMessage struct {
	Type     string `json:"type"`
	JobID    string `json:"job_id,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Status   string `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
}

type ProgressSubscriber struct {
	conn   *websocket.Conn
	jobID  string
	send   chan ProgressMessage
	cancel chan struct{}
	mu     sync.Mutex
}

type ProgressHub struct {
	subscribers map[string][]*ProgressSubscriber // jobID -> subscribers
	mu          sync.RWMutex
	broadcast   chan ProgressMessage
}

func NewProgressHub() *ProgressHub {
	hub := &ProgressHub{
		subscribers: make(map[string][]*ProgressSubscriber),
		broadcast:   make(chan ProgressMessage, 100),
	}
	go hub.run()
	return hub
}

func (h *ProgressHub) run() {
	for msg := range h.broadcast {
		h.mu.RLock()
		subs := h.subscribers[msg.JobID]
		h.mu.RUnlock()

		for _, sub := range subs {
			select {
			case sub.send <- msg:
			case <-sub.cancel:
			}
		}
	}
}

func (h *ProgressHub) Subscribe(jobID string, conn *websocket.Conn) *ProgressSubscriber {
	sub := &ProgressSubscriber{
		conn:   conn,
		jobID:  jobID,
		send:   make(chan ProgressMessage, 10),
		cancel: make(chan struct{}),
	}

	h.mu.Lock()
	h.subscribers[jobID] = append(h.subscribers[jobID], sub)
	h.mu.Unlock()

	return sub
}

func (h *ProgressHub) Unsubscribe(sub *ProgressSubscriber) {
	close(sub.cancel)

	h.mu.Lock()
	subs := h.subscribers[sub.jobID]
	for i, s := range subs {
		if s == sub {
			h.subscribers[sub.jobID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(h.subscribers[sub.jobID]) == 0 {
		delete(h.subscribers, sub.jobID)
	}
	h.mu.Unlock()
}

func (h *ProgressHub) BroadcastProgress(jobID string, progress int, status string) {
	h.broadcast <- ProgressMessage{
		Type:     "progress",
		JobID:    jobID,
		Progress: progress,
		Status:   status,
	}
}

func (h *ProgressHub) BroadcastComplete(jobID string, status string, errMsg string) {
	h.broadcast <- ProgressMessage{
		Type:   "complete",
		JobID:  jobID,
		Status: status,
		Error:  errMsg,
	}
}

type ProgressHandler struct {
	master *cluster.MasterNode
	hub    *ProgressHub
}

func NewProgressHandler(master *cluster.MasterNode, hub *ProgressHub) *ProgressHandler {
	return &ProgressHandler{
		master: master,
		hub:    hub,
	}
}

func (h *ProgressHandler) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/flash/{job_id}/progress", h.handleProgressWebSocket).Methods("GET")
}

func (h *ProgressHandler) handleProgressWebSocket(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["job_id"]

	if h.master == nil {
		http.Error(w, "Not available on worker nodes", http.StatusNotImplemented)
		return
	}

	job := h.master.GetJobQueue().Get(jobID)
	if job == nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	conn, err := progressUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Str("job_id", jobID).Msg("WebSocket upgrade failed")
		return
	}

	log.Info().Str("job_id", jobID).Str("remote_addr", r.RemoteAddr).Msg("Progress WebSocket connected")

	sub := h.hub.Subscribe(jobID, conn)
	defer h.hub.Unsubscribe(sub)

	// Send initial state
	jobData := job.ToMap()
	conn.WriteJSON(ProgressMessage{
		Type:     "init",
		JobID:    jobID,
		Progress: jobData["progress"].(int),
		Status:   string(job.Status),
	})

	// Send pings and handle outgoing messages
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-sub.cancel:
				return
			case <-ticker.C:
				sub.mu.Lock()
				conn.WriteMessage(websocket.PingMessage, nil)
				sub.mu.Unlock()
			}
		}
	}()

	for msg := range sub.send {
		sub.mu.Lock()
		if err := conn.WriteJSON(msg); err != nil {
			log.Debug().Err(err).Str("job_id", jobID).Msg("WebSocket write error")
			sub.mu.Unlock()
			return
		}
		sub.mu.Unlock()
	}
}

func (h *ProgressHandler) StartProgressStreamer(jobID string) {
	job := h.master.GetJobQueue().Get(jobID)
	if job == nil {
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-time.After(500 * time.Millisecond):
			job.RLock()
			progress := job.Progress
			status := string(job.Status)
			job.RUnlock()

			h.hub.BroadcastProgress(jobID, progress, status)

			if status == "completed" || status == "failed" {
				job.RLock()
				errMsg := job.Error
				job.RUnlock()
				h.hub.BroadcastComplete(jobID, status, errMsg)
				return
			}
		}
	}
}
