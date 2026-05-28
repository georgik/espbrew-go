package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
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
}

type FlashSubmitResponse struct {
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

		log.Debug().Str("type", msg.Type).Int("progress", msg.Progress).Msg("Progress update")

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
