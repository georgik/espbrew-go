package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	defaultMaxRetries  = 3
	defaultRetryDelay  = 1 * time.Second
	defaultHTTPTimeout = 10 * time.Second
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	wsDialer   *websocket.Dialer
	maxRetries int
	retryDelay time.Duration
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		wsDialer:   &websocket.Dialer{HandshakeTimeout: defaultHTTPTimeout},
		maxRetries: defaultMaxRetries,
		retryDelay: defaultRetryDelay,
	}
}

func (c *Client) SetRetryPolicy(maxRetries int, retryDelay time.Duration) {
	c.maxRetries = maxRetries
	c.retryDelay = retryDelay
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) SetTimeout(timeout time.Duration) {
	c.httpClient.Timeout = timeout
	c.wsDialer.HandshakeTimeout = timeout
}

type RetryableError struct {
	Err error
}

func (r *RetryableError) Error() string {
	return r.Err.Error()
}

func (r *RetryableError) Unwrap() error {
	return r.Err
}

func isRetryable(statusCode int) bool {
	return statusCode >= 500 || statusCode == http.StatusTooManyRequests
}

func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	var lastErr error
	var resp *http.Response

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			log.Debug().Int("attempt", attempt).Int("max_retries", c.maxRetries).
				Str("url", req.URL.String()).Msg("Retrying request")
			time.Sleep(c.retryDelay * time.Duration(attempt))
		}

		var err error
		resp, err = c.httpClient.Do(req)
		if err != nil {
			lastErr = &RetryableError{Err: err}
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		if !isRetryable(resp.StatusCode) {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return resp, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("status %d", resp.StatusCode)
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

type DeviceInfo struct {
	Path       string      `json:"path"`
	VID        string      `json:"vid,omitempty"`
	PID        string      `json:"pid,omitempty"`
	State      DeviceState `json:"status"`
	NodeID     string      `json:"node_id,omitempty"`
	ChipType   string      `json:"chip_type,omitempty"`
	BoardModel string      `json:"board_model,omitempty"`
	Tags       []string    `json:"tags,omitempty"`
	ReservedBy string      `json:"reserved_by,omitempty"`
}

func (c *Client) ListDevices() ([]DeviceInfo, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/devices", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var devices []DeviceInfo
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return devices, nil
}

func (c *Client) GetStatus() (*ClusterStatus, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/status", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var status ClusterStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &status, nil
}

type ClusterStatus struct {
	NodesCount   int    `json:"nodes_count"`
	DevicesCount int    `json:"devices_count"`
	JobsCount    int    `json:"jobs_count"`
	Role         string `json:"role,omitempty"`
	QueueSize    int    `json:"queue_size,omitempty"`
}

type FlashUploadResponse struct {
	FileID string `json:"file_id"`
	Size   int64  `json:"size"`
}

type FlashSubmitRequest struct {
	DevicePath  string                 `json:"device_path"`
	FileID      string                 `json:"file_id"`
	FirmwareURL string                 `json:"firmware_url,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
	ClientID    string                 `json:"client_id,omitempty"`
	Offset      int                    `json:"offset,omitempty"`
	Erase       bool                   `json:"erase,omitempty"`
}

type FlashSubmitResponse struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	DevicePath string `json:"device_path"`
}

// EraseSubmitRequest represents a request to erase flash memory
type EraseSubmitRequest struct {
	DevicePath string `json:"device_path"`
	Address    uint32 `json:"address,omitempty"`
	Size       uint32 `json:"size,omitempty"`
	EraseAll   bool   `json:"erase_all"`
	ClientID   string `json:"client_id,omitempty"`
}

// EraseSubmitResponse represents the response from an erase job submission
type EraseSubmitResponse struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	DevicePath string `json:"device_path"`
}

// ReadFlashRequest represents a request to read flash memory
type ReadFlashRequest struct {
	DevicePath string `json:"device_path"`
	Address    uint32 `json:"address"`
	Size       uint32 `json:"size"`
	ClientID   string `json:"client_id,omitempty"`
}

// ReadFlashResponse represents the response from a read flash request
type ReadFlashResponse struct {
	JobID       string `json:"job_id"`
	Status      string `json:"status"` // pending, running, completed, failed
	Size        int64  `json:"size"`
	DownloadURL string `json:"download_url,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (c *Client) UploadFirmware(filePath string) (*FlashUploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("firmware", filePath)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/v1/flash/upload", &body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var uploadResp FlashUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &uploadResp, nil
}

func (c *Client) SubmitFlash(req FlashSubmitRequest) (*FlashSubmitResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/flash", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var flashResp FlashSubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&flashResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &flashResp, nil
}

func (c *Client) SubmitErase(req EraseSubmitRequest) (*EraseSubmitResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/flash/erase", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var eraseResp EraseSubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&eraseResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &eraseResp, nil
}

// ReadFlash submits a flash read job to the cluster
func (c *Client) ReadFlash(req ReadFlashRequest) (*ReadFlashResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/flash/read", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var readResp ReadFlashResponse
	if err := json.NewDecoder(resp.Body).Decode(&readResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &readResp, nil
}

// GetReadFlashStatus retrieves the status of a read flash job
func (c *Client) GetReadFlashStatus(jobID string) (*ReadFlashResponse, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/flash/read/"+jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var statusResp ReadFlashResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &statusResp, nil
}

// DownloadReadFlash downloads the flash read data
func (c *Client) DownloadReadFlash(jobID string) ([]byte, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/v1/flash/download/"+jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) CancelJob(jobID string) error {
	req, err := http.NewRequest("DELETE", c.baseURL+"/api/v1/jobs/"+jobID, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

type ProgressMessage struct {
	Type     string `json:"type"`
	JobID    string `json:"job_id,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Status   string `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
}

type ProgressClient struct {
	conn      *websocket.Conn
	url       string
	jobID     string
	mu        sync.Mutex
	closeChan chan struct{}
}

func (c *Client) ConnectProgress(jobID string) (*ProgressClient, error) {
	return c.ConnectProgressWithContext(context.Background(), jobID)
}

func (c *Client) ConnectProgressWithContext(ctx context.Context, jobID string) (*ProgressClient, error) {
	wsURL := c.baseURL
	if len(wsURL) >= 4 && wsURL[:4] == "http" {
		wsURL = "ws" + wsURL[4:]
	}
	wsURL += fmt.Sprintf("/api/v1/flash/%s/progress", jobID)

	var conn *websocket.Conn
	var err error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			log.Debug().Int("attempt", attempt).Str("url", wsURL).Msg("Retrying WebSocket connection")
			time.Sleep(c.retryDelay * time.Duration(attempt))
		}

		conn, _, err = c.wsDialer.DialContext(ctx, wsURL, nil)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("dial websocket: %w", err)
	}

	return &ProgressClient{
		conn:      conn,
		url:       wsURL,
		jobID:     jobID,
		closeChan: make(chan struct{}),
	}, nil
}

func (p *ProgressClient) Stream(callback func(ProgressMessage)) error {
	defer close(p.closeChan)
	defer p.conn.Close()

	for {
		p.mu.Lock()
		msg := ProgressMessage{}
		err := p.conn.ReadJSON(&msg)
		p.mu.Unlock()

		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return fmt.Errorf("read message: %w", err)
		}

		if callback != nil {
			callback(msg)
		}

		if msg.Type == "complete" {
			if msg.Status == "failed" {
				return fmt.Errorf("job failed: %s", msg.Error)
			}
			return nil
		}
	}
}

func (p *ProgressClient) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// SnapRequest represents a request to execute a device snapshot via cluster API.
type SnapRequest struct {
	DeviceID   string `json:"device_id"`   // Device identifier (device_id, MAC, or path)
	Duration   int    `json:"duration"`    // Monitor duration in seconds
	CameraID   string `json:"camera_id"`   // Camera device identifier (optional)
	Firmware   string `json:"firmware"`    // Firmware path on server (or file_id reference)
	ForceFlash bool   `json:"force_flash"` // Force flash even if hash matches
	SkipFlash  bool   `json:"skip_flash"`  // Skip flashing entirely
}

// SnapResponse represents the response from a cluster snap operation.
type SnapResponse struct {
	SnapID    string        `json:"snap_id"`  // Unique snapshot identifier
	Status    string        `json:"status"`   // Operation status (success, partial, failed)
	Error     string        `json:"error"`    // Error message if failed
	ImageData []byte        `json:"-"`        // Raw image data (not in JSON)
	Metadata  *SnapMetadata `json:"metadata"` // Snapshot metadata
}

// SnapMetadata contains descriptive information about a cluster snapshot.
type SnapMetadata struct {
	Timestamp      time.Time `json:"timestamp"`       // When the snapshot was created
	Duration       int64     `json:"duration_ms"`     // How long the snapshot took (milliseconds)
	DevicePath     string    `json:"device_path"`     // Serial port path
	DeviceNode     string    `json:"device_node"`     // Cluster node that owns the device
	FlashEnabled   bool      `json:"flash_enabled"`   // Whether flashing was performed
	MonitorEnabled bool      `json:"monitor_enabled"` // Whether serial monitoring was enabled
	CaptureEnabled bool      `json:"capture_enabled"` // Whether camera capture was enabled
	LogEntryCount  int       `json:"log_entry_count"` // Number of log lines captured
	ImageSize      int       `json:"image_size"`      // Size of captured image in bytes
}

// ExecuteSnap submits a snap request to the cluster and returns the response.
func (c *Client) ExecuteSnap(req SnapRequest) (*SnapResponse, error) {
	// Build request URL with device ID as query parameter (avoids URL encoding issues with device paths)
	url := fmt.Sprintf("%s/api/v1/devices/snap?device_id=%s", c.baseURL, url.QueryEscape(req.DeviceID))

	// Marshal request body (keep for retries)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Execute with retry - recreate request body each time
	var lastErr error
	var resp *http.Response
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			log.Debug().Int("attempt", attempt).Int("max_retries", c.maxRetries).
				Str("url", url).Msg("Retrying snap request")
			time.Sleep(c.retryDelay * time.Duration(attempt))
		}

		// Create fresh request with new body for each attempt
		httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err = c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = &RetryableError{Err: err}
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// Success - check status code
			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
			}

			// Decode response
			var snapResp struct {
				SnapID string                 `json:"snap_id"`
				Status string                 `json:"status"`
				Error  string                 `json:"error,omitempty"`
				Result map[string]interface{} `json:"result,omitempty"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&snapResp); err != nil {
				resp.Body.Close()
				return nil, fmt.Errorf("decode response: %w", err)
			}
			resp.Body.Close()

			// Build response
			response := &SnapResponse{
				SnapID: snapResp.SnapID,
				Status: snapResp.Status,
				Error:  snapResp.Error,
			}

			// Extract metadata from result if available
			if snapResp.Result != nil {
				response.Metadata = extractSnapMetadata(snapResp.Result)
			}

			return response, nil
		}

		if !isRetryable(resp.StatusCode) {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("status %d", resp.StatusCode)
	}

	return nil, fmt.Errorf("request failed: %w", lastErr)
}

// extractSnapMetadata extracts snap metadata from the result map.
func extractSnapMetadata(result map[string]interface{}) *SnapMetadata {
	metadata := &SnapMetadata{}

	if m, ok := result["metadata"].(map[string]interface{}); ok {
		// Extract timestamp
		if ts, ok := m["timestamp"].(string); ok {
			metadata.Timestamp, _ = time.Parse(time.RFC3339, ts)
		}

		// Extract duration
		if dur, ok := m["duration_ms"].(float64); ok {
			metadata.Duration = int64(dur)
		}

		// Extract device path
		if dp, ok := m["device_path"].(string); ok {
			metadata.DevicePath = dp
		}

		// Extract device node
		if dn, ok := m["device_node"].(string); ok {
			metadata.DeviceNode = dn
		}

		// Extract flash enabled
		if fe, ok := m["flash_enabled"].(bool); ok {
			metadata.FlashEnabled = fe
		}

		// Extract monitor enabled
		if me, ok := m["monitor_enabled"].(bool); ok {
			metadata.MonitorEnabled = me
		}

		// Extract capture enabled
		if ce, ok := m["capture_enabled"].(bool); ok {
			metadata.CaptureEnabled = ce
		}

		// Extract log entry count
		if lec, ok := m["log_entry_count"].(float64); ok {
			metadata.LogEntryCount = int(lec)
		}

		// Extract image size
		if is, ok := m["image_size"].(float64); ok {
			metadata.ImageSize = int(is)
		}
	}

	return metadata
}

// FlashHashCheckRequest represents a flash hash check request.
type FlashHashCheckRequest struct {
	Firmware string `json:"firmware"` // Firmware path to check
	Chip     string `json:"chip"`     // Chip type (optional, default: esp32s3)
}

// FlashHashCheckResponse represents the response from a flash hash check.
type FlashHashCheckResponse struct {
	DeviceID      string `json:"device_id"`
	Match         bool   `json:"match"`
	DeviceHash    string `json:"device_hash,omitempty"`
	FirmwareHash  string `json:"firmware_hash,omitempty"`
	FlashRequired bool   `json:"flash_required"`
	Status        string `json:"status"` // "checked", "error"
	Error         string `json:"error,omitempty"`
}

// CheckFlashHash checks if the device flash matches the firmware hash.
func (c *Client) CheckFlashHash(deviceID string, req FlashHashCheckRequest) (*FlashHashCheckResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/devices/%s/flash-hash", c.baseURL, url.PathEscape(deviceID))
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetry(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var hashResp FlashHashCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&hashResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &hashResp, nil
}
