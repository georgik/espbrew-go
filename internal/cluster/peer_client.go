package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// PeerJobClient handles HTTP communication from leader to peer nodes
// for job assignment and control.
type PeerJobClient struct {
	baseURL    string
	nodeID     string
	httpClient *http.Client
	timeout    time.Duration
}

// NewPeerJobClient creates a new client for communicating with a peer node.
// The baseURL should be the peer's HTTP address (e.g., "http://localhost:8081").
func NewPeerJobClient(baseURL, nodeID string) *PeerJobClient {
	// Ensure baseURL has a scheme
	if len(baseURL) >= 7 && (baseURL[:7] != "http://" && baseURL[:7] != "https:") {
		baseURL = "http://" + baseURL
	}

	return &PeerJobClient{
		baseURL: baseURL,
		nodeID:  nodeID,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		timeout: 10 * time.Minute, // Default job timeout
	}
}

// SetTimeout sets the HTTP client timeout for requests.
func (c *PeerJobClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	c.httpClient.Timeout = timeout
}

// JobAssignRequest is sent from leader to peer to assign a job.
type JobAssignRequest struct {
	JobID        string `json:"job_id"`
	JobType      string `json:"job_type"` // "flash" or "erase"
	DevicePath   string `json:"device_path"`
	Firmware     string `json:"firmware"`      // Firmware path (for flash jobs)
	Offset       int    `json:"offset"`        // Flash offset (for flash jobs)
	Erase        bool   `json:"erase"`         // Enable erase before flash
	EraseAll     bool   `json:"erase_all"`     // For erase jobs
	EraseAddress uint32 `json:"erase_address"` // For erase jobs
	EraseSize    uint32 `json:"erase_size"`    // For erase jobs
}

// JobAssignResponse is the peer's response to a job assignment.
type JobAssignResponse struct {
	Status  string `json:"status"` // "accepted", "rejected"
	Message string `json:"message,omitempty"`
	JobID   string `json:"job_id"`
}

// AssignJob sends a job assignment to the peer node.
// Returns 202 Accepted if the peer accepted the job.
func (c *PeerJobClient) AssignJob(ctx context.Context, job *Job) (*JobAssignResponse, error) {
	req := &JobAssignRequest{
		JobID:        job.ID,
		JobType:      string(job.Type),
		DevicePath:   job.DevicePath,
		Offset:       job.Offset,
		Erase:        job.Erase,
		EraseAll:     job.EraseAll,
		EraseAddress: job.EraseAddress,
		EraseSize:    job.EraseSize,
	}

	if job.Type == JobTypeFlash {
		req.Firmware = job.Firmware
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal job assign request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/jobs/assign", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create assign request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	log.Info().
		Str("peer_id", c.nodeID).
		Str("job_id", job.ID).
		Str("device", job.DevicePath).
		Msg("Assigning job to peer")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("assign job request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read assign response: %w", err)
	}

	if resp.StatusCode == http.StatusAccepted {
		var response JobAssignResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			return &JobAssignResponse{Status: "accepted", JobID: job.ID}, nil
		}
		return &response, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("device not found on peer: %s", job.DevicePath)
	}

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("peer unavailable: %s", string(respBody))
	}

	return nil, fmt.Errorf("assign job failed: status %d: %s", resp.StatusCode, string(respBody))
}

// CancelJob sends a cancellation request to the peer node.
func (c *PeerJobClient) CancelJob(ctx context.Context, jobID string) error {
	url := fmt.Sprintf("%s/api/v1/jobs/%s/cancel", c.baseURL, jobID)
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("create cancel request: %w", err)
	}

	log.Info().
		Str("peer_id", c.nodeID).
		Str("job_id", jobID).
		Msg("Cancelling job on peer")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("cancel job request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		return nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil // Job already completed/doesn't exist
	}

	return fmt.Errorf("cancel job failed: status %d", resp.StatusCode)
}

// Ping checks if the peer node is reachable.
func (c *PeerJobClient) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ping request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed: status %d", resp.StatusCode)
	}

	return nil
}
