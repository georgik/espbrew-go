package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Client is the Wokwi API client
type Client struct {
	transport *Transport
	connected bool
	mu        sync.Mutex
	diagram   string
	firmware  string
	elf       string
}

// NewClient creates a new Wokwi API client
func NewClient(token string) *Client {
	return NewClientWithServer(token, "")
}

// NewClientWithServer creates a new client with a custom server URL
func NewClientWithServer(token, server string) *Client {
	return &Client{
		transport: NewTransport(token, server),
	}
}

// Connect connects to the Wokwi API
func (c *Client) Connect(ctx context.Context) (*HelloMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hello, err := c.transport.Connect(ctx)
	if err != nil {
		return nil, err
	}

	c.connected = true
	log.Info().Str("version", hello.AppVersion).Msg("Connected to Wokwi API")
	return hello, nil
}

// Close closes the connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false
	return c.transport.Close()
}

// SetDiagram sets the diagram JSON content
func (c *Client) SetDiagram(diagram string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diagram = diagram
}

// UploadDiagram uploads the diagram JSON to the server
func (c *Client) UploadDiagram(ctx context.Context) error {
	c.mu.Lock()
	diagram := c.diagram
	c.mu.Unlock()

	if diagram == "" {
		return fmt.Errorf("diagram not set")
	}

	return c.UploadFile(ctx, "diagram.json", []byte(diagram))
}

// SetFirmware sets the firmware file path
func (c *Client) SetFirmware(firmware string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.firmware = firmware
}

// SetELF sets the ELF file path
func (c *Client) SetELF(elf string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.elf = elf
}

// UploadFile uploads a file to the server
func (c *Client) UploadFile(ctx context.Context, name string, data []byte) error {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return fmt.Errorf("not connected")
	}
	c.mu.Unlock()

	params := UploadParams{
		Name:   name,
		Binary: base64.StdEncoding.EncodeToString(data),
	}

	_, err := c.transport.Request(ctx, "file:upload", map[string]interface{}{
		"name":   params.Name,
		"binary": params.Binary,
	})

	if err != nil {
		return fmt.Errorf("failed to upload %s: %w", name, err)
	}

	log.Debug().Str("file", name).Msg("File uploaded")
	return nil
}

// UploadFirmware uploads the firmware binary
func (c *Client) UploadFirmware(ctx context.Context) error {
	c.mu.Lock()
	firmwarePath := c.firmware
	c.mu.Unlock()

	if firmwarePath == "" {
		return fmt.Errorf("firmware not set")
	}

	data, err := os.ReadFile(firmwarePath)
	if err != nil {
		return fmt.Errorf("failed to read firmware: %w", err)
	}

	name := filepath.Base(firmwarePath)
	return c.UploadFile(ctx, name, data)
}

// UploadELF uploads the ELF file
func (c *Client) UploadELF(ctx context.Context) error {
	c.mu.Lock()
	elfPath := c.elf
	c.mu.Unlock()

	if elfPath == "" {
		return nil // ELF is optional
	}

	data, err := os.ReadFile(elfPath)
	if err != nil {
		return fmt.Errorf("failed to read ELF: %w", err)
	}

	name := filepath.Base(elfPath)
	return c.UploadFile(ctx, name, data)
}

// StartSimulation starts the simulation
func (c *Client) StartSimulation(ctx context.Context) error {
	c.mu.Lock()
	firmware := filepath.Base(c.firmware)
	elf := filepath.Base(c.elf)
	c.mu.Unlock()

	if firmware == "" {
		return fmt.Errorf("firmware not set")
	}

	params := SimStartParams{
		Firmware: firmware,
		Pause:    false,
	}

	if elf != "" {
		params.Elf = elf
	}

	_, err := c.transport.Request(ctx, "sim:start", map[string]interface{}{
		"firmware": params.Firmware,
		"elf":      params.Elf,
		"pause":    params.Pause,
	})

	if err != nil {
		return fmt.Errorf("failed to start simulation: %w", err)
	}

	log.Info().Msg("Simulation started")
	return nil
}

// PauseSimulation pauses the running simulation
func (c *Client) PauseSimulation(ctx context.Context) error {
	_, err := c.transport.Request(ctx, "sim:pause", nil)
	return err
}

// ResumeSimulation resumes the simulation
func (c *Client) ResumeSimulation(ctx context.Context) error {
	_, err := c.transport.Request(ctx, "sim:resume", nil)
	return err
}

// RestartSimulation restarts the simulation
func (c *Client) RestartSimulation(ctx context.Context) error {
	_, err := c.transport.Request(ctx, "sim:restart", map[string]interface{}{
		"pause": false,
	})
	return err
}

// SubscribeSerial subscribes to serial output events
func (c *Client) SubscribeSerial() chan EventMessage {
	return c.transport.Subscribe("sim:serial")
}

// UnsubscribeSerial unsubscribes from serial output events
func (c *Client) UnsubscribeSerial(ch chan EventMessage) {
	c.transport.Unsubscribe("sim:serial", ch)
}

// WriteSerial writes data to the serial port
func (c *Client) WriteSerial(ctx context.Context, data []byte) error {
	// Convert bytes to array of numbers for JSON
	numbers := make([]int, len(data))
	for i, b := range data {
		numbers[i] = int(b)
	}

	_, err := c.transport.Request(ctx, "sim:serial:write", map[string]interface{}{
		"data": numbers,
	})
	return err
}

// WaitForOutput waits for a specific pattern in serial output
func (c *Client) WaitForOutput(ctx context.Context, pattern string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	serialCh := c.SubscribeSerial()
	defer c.UnsubscribeSerial(serialCh)

	buffer := make([]byte, 0, 4096)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-serialCh:
			if data, ok := event.Result["data"].(string); ok {
				buffer = append(buffer, []byte(data)...)

				// Check for pattern
				if len(buffer) >= len(pattern) {
					if contains(buffer, []byte(pattern)) {
						return nil
					}
				}

				// Keep buffer manageable
				if len(buffer) > 65536 {
					buffer = buffer[len(buffer)-4096:]
				}
			}
		}
	}
}

// StreamOutput streams serial output to a writer
func (c *Client) StreamOutput(ctx context.Context, out io.Writer) error {
	serialCh := c.SubscribeSerial()
	defer c.UnsubscribeSerial(serialCh)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-serialCh:
			if !ok {
				return nil
			}
			if data, ok := event.Result["data"].(string); ok {
				if _, err := out.Write([]byte(data)); err != nil {
					return err
				}
			}
		}
	}
}

// contains checks if haystack contains needle
func contains(haystack, needle []byte) bool {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
