package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// DeviceName extracts the base device name without the /dev/ prefix
func DeviceName(devicePath string) string {
	if devicePath == "" {
		return ""
	}
	base := filepath.Base(devicePath)
	if base == devicePath {
		// Already just a name
		return devicePath
	}
	return base
}

type MonitorMessage struct {
	Type    string `json:"type"`    // "data", "error", "monitor_start", "reset_complete"
	Data    string `json:"data"`    // base64 encoded data for type "data"
	Port    string `json:"port"`    // port for type "monitor_start"
	Baud    int    `json:"baud"`    // baud for type "monitor_start"
	Message string `json:"message"` // error message for type "error"
}

type MonitorClient struct {
	baseURL    string
	devicePath string
	baud       int
	reset      bool
	exitOn     string
	conn       *websocket.Conn
	httpClient *http.Client
	mu         sync.Mutex
	cancel     context.CancelFunc
}

type MonitorConfig struct {
	Baud        int
	Reset       bool
	ExitOn      string
	ExitOnError string
	Duration    time.Duration
}

func NewMonitorClient(baseURL, devicePath string, cfg MonitorConfig) *MonitorClient {
	return &MonitorClient{
		baseURL:    baseURL,
		devicePath: devicePath,
		baud:       cfg.Baud,
		reset:      cfg.Reset,
		exitOn:     cfg.ExitOn,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *MonitorClient) writeMessage(msgType string, data interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	return c.conn.WriteJSON(map[string]interface{}{
		"type": msgType,
		"data": data,
	})
}

func (c *MonitorClient) Reset() error {
	return c.writeMessage("reset", nil)
}

func (c *MonitorClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}

	if c.conn != nil {
		// Send close message
		c.conn.WriteJSON(map[string]interface{}{
			"type": "close",
		})
		return c.conn.Close()
	}
	return nil
}

func (c *MonitorClient) readLoop(ctx context.Context, dataCh chan<- []byte, errorCh chan<- error) {
	defer close(dataCh)
	// Note: errorCh closed explicitly before return to ensure proper order

	for {
		select {
		case <-ctx.Done():
			close(errorCh)
			return
		default:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()

			if conn == nil {
				close(errorCh)
				return
			}

			var msg MonitorMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				select {
				case errorCh <- fmt.Errorf("read error: %w", err):
				case <-ctx.Done():
				}
				close(errorCh)
				return
			}

			log.Debug().Str("type", msg.Type).Msg("Monitor message")

			switch msg.Type {
			case "data":
				if msg.Data != "" {
					data, err := base64.StdEncoding.DecodeString(msg.Data)
					if err != nil {
						log.Warn().Err(err).Str("b64_data", msg.Data).Msg("Failed to decode base64 data")
						continue
					}
					log.Debug().Int("bytes", len(data)).Str("content", string(data)).Msg("Received data from server")

					// Normalize line endings: convert \r\n to \n for clean terminal output
					data = normalizeLineEndings(data)

					select {
					case dataCh <- data:
					case <-ctx.Done():
						return
					}
				}

			case "error":
				select {
				case errorCh <- fmt.Errorf("monitor error: %s", msg.Message):
				case <-ctx.Done():
					close(errorCh)
					return
				}
				close(errorCh)
				return

			case "reset_complete":
				// Reset completed
				log.Debug().Msg("Device reset completed")

			case "monitor_start":
				log.Info().Str("port", msg.Port).Int("baud", msg.Baud).Msg("Monitor started")

			case "exit":
				// Server-side exit (pattern matched)
				select {
				case errorCh <- fmt.Errorf("server exit: %s", msg.Message):
				case <-ctx.Done():
					close(errorCh)
					return
				}
				close(errorCh)
				return
			}
		}
	}
}

func (c *MonitorClient) Stream(ctx context.Context, dataCh chan<- []byte, errorCh chan<- error) error {
	// Build WebSocket URL
	wsURL := c.baseURL
	if wsURL[:4] == "http" {
		wsURL = "ws" + wsURL[4:]
	}

	// Build query parameters
	query := ""
	if c.baud != 115200 {
		query = fmt.Sprintf("?baud=%d", c.baud)
	}
	if c.exitOn != "" {
		if query == "" {
			query = "?"
		} else {
			query += "&"
		}
		query += fmt.Sprintf("exit_on=%s", jsonEscape(c.exitOn))
	}
	if c.reset {
		if query == "" {
			query = "?"
		} else {
			query += "&"
		}
		query += "reset=1"
	}

	wsURL += "/api/v1/monitor/" + DeviceName(c.devicePath) + query

	log.Debug().Str("url", wsURL).Msg("Connecting to monitor WebSocket")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Start read loop
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.readLoop(ctx, dataCh, errorCh)

	return nil
}

func (c *MonitorClient) ReserveDevice(clientID string, ttl int) error {
	req := map[string]interface{}{
		"client_id": clientID,
		"ttl":       ttl,
	}
	body, _ := json.Marshal(req)

	reqURL := c.baseURL + "/api/v1/devices/" + DeviceName(c.devicePath) + "/reserve"
	resp, err := c.httpClient.Post(reqURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("reserve request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reserve failed: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *MonitorClient) ReleaseDevice(clientID string) error {
	req := map[string]interface{}{
		"client_id": clientID,
	}
	body, _ := json.Marshal(req)

	reqURL := c.baseURL + "/api/v1/devices/" + DeviceName(c.devicePath) + "/reserve"
	req2, _ := http.NewRequest("DELETE", reqURL, bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req2)
	if err != nil {
		return fmt.Errorf("release request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("release failed: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// normalizeLineEndings ensures proper line endings for raw terminal mode
// In raw mode, \r is needed to move cursor to column 0
// ESP32 serial output uses CRLF (\r\n), we preserve that
// Lone \n gets converted to \r\n for proper cursor positioning
func normalizeLineEndings(data []byte) []byte {
	result := make([]byte, 0, len(data)*2)
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			// Keep \r\n as-is for proper raw mode behavior
			result = append(result, '\r', '\n')
			i++ // skip the \n we just processed
		} else if data[i] == '\n' {
			// Lone \n becomes \r\n for raw mode
			result = append(result, '\r', '\n')
		} else if data[i] == '\r' {
			// Lone \r (rare) stays as \r
			result = append(result, '\r')
		} else {
			result = append(result, data[i])
		}
	}
	return result
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}
