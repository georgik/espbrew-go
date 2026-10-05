package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Client is the Wokwi API client.
//
// It speaks the Wokwi Simulation API over WebSocket: the hello handshake,
// file:upload, sim:start, and the serial-monitor:* command/event pair. The
// wire contract is documented in wiki/wokwi-sim-api-analysis.md.
type Client struct {
	transport *Transport
	connected bool
	mu        sync.Mutex
	diagram   string
}

// NewClient creates a new Wokwi API client pointed at the public server.
func NewClient(token string) *Client {
	return NewClientWithServer(token, "")
}

// NewClientWithServer creates a new client. An empty server falls back to the
// public wss://wokwi.com endpoint inside NewTransport.
func NewClientWithServer(token, server string) *Client {
	return &Client{
		transport: NewTransport(token, server),
	}
}

// Connect connects to the Wokwi API and performs the hello handshake.
func (c *Client) Connect(ctx context.Context) (*HelloMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	hello, err := c.transport.Connect(ctx)
	if err != nil {
		return nil, err
	}

	c.connected = true
	log.Info().Str("version", hello.AppVersion).Str("server", hello.ServerURL).Msg("Connected to Wokwi API")
	return hello, nil
}

// Close closes the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false
	return c.transport.Close()
}

// Diagram returns the currently set diagram JSON.
func (c *Client) Diagram() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.diagram
}

// SetDiagram sets the diagram JSON content.
func (c *Client) SetDiagram(diagram string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diagram = diagram
}

// UploadDiagram uploads the diagram.json to the server.
func (c *Client) UploadDiagram(ctx context.Context) error {
	c.mu.Lock()
	diagram := c.diagram
	c.mu.Unlock()

	if diagram == "" {
		return fmt.Errorf("diagram not set")
	}

	return c.UploadFile(ctx, "diagram.json", []byte(diagram))
}

// UploadFile uploads a single file to the server using file:upload. The
// payload is base64-encoded, matching the Wokwi wire format.
func (c *Client) UploadFile(ctx context.Context, name string, data []byte) error {
	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()

	if !connected {
		return fmt.Errorf("not connected")
	}

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

// UploadFirmwareSections uploads each flash section as its own file named
// "flash-<offset>.bin" and returns the sections (with File set to the remote
// name). The server runs the app by referencing these sections in sim:start.
//
// This mirrors wokwi-cli's uploadESP32Firmware / the python
// upload_idf_firmware: a single monolithic image will not boot, so the
// bootloader, partition table, and app must each be uploaded at their offset.
func (c *Client) UploadFirmwareSections(ctx context.Context, sections []FlashSection) ([]FlashSection, error) {
	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()

	if !connected {
		return nil, fmt.Errorf("not connected")
	}

	uploaded := make([]FlashSection, 0, len(sections))
	for _, s := range sections {
		remoteName := fmt.Sprintf("flash-%x.bin", s.Offset)
		data := s.Data
		if len(data) == 0 {
			var err error
			data, err = os.ReadFile(s.File)
			if err != nil {
				return nil, fmt.Errorf("read firmware section %s: %w", s.File, err)
			}
		}
		if err := c.UploadFile(ctx, remoteName, data); err != nil {
			return nil, fmt.Errorf("upload section %s: %w", remoteName, err)
		}
		uploaded = append(uploaded, FlashSection{Offset: s.Offset, File: remoteName})
	}
	return uploaded, nil
}

// ParseFlasherArgs parses an ESP-IDF flasher_args.json, returning the flash
// sections (offset + absolute source path) and the flash size in bytes when
// present. flash_files maps hex-offset strings to relative file paths.
func ParseFlasherArgs(path string) ([]FlashSection, *int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read flasher_args.json: %w", err)
	}

	var args struct {
		FlashFiles    map[string]string `json:"flash_files"`
		FlashSettings *struct {
			FlashSize string `json:"flash_size"`
		} `json:"flash_settings"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, nil, fmt.Errorf("parse flasher_args.json: %w", err)
	}
	if len(args.FlashFiles) == 0 {
		return nil, nil, fmt.Errorf("flasher_args.json has no flash_files")
	}

	base := filepath.Dir(path)
	sections := make([]FlashSection, 0, len(args.FlashFiles))
	for offsetStr, rel := range args.FlashFiles {
		offset, err := strconv.ParseInt(offsetStr, 16, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid flash offset %q: %w", offsetStr, err)
		}
		abs := rel
		if !filepath.IsAbs(rel) {
			abs = filepath.Join(base, rel)
		}
		sections = append(sections, FlashSection{Offset: int(offset), File: abs})
	}
	// Deterministic, ascending-offset order so callers can rely on a stable
	// sequence (the source map iterates in random order).
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].Offset < sections[j].Offset
	})

	var flashSize *int
	if args.FlashSettings != nil {
		if sz := parseFlashSize(args.FlashSettings.FlashSize); sz != nil {
			flashSize = sz
		}
	}
	return sections, flashSize, nil
}

// parseFlashSize converts a Wokwi/ESP flash size like "4MB" / "1MB" into
// bytes. Returns nil when it cannot be parsed.
func parseFlashSize(s string) *int {
	s = strings.TrimSpace(s)
	mult := 1
	if strings.HasSuffix(s, "MB") {
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "KB") {
		mult = 1024
		s = strings.TrimSuffix(s, "KB")
	}
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || val <= 0 {
		return nil
	}
	v := val * mult
	return &v
}

// UploadFirmwareFromFlasherArgs parses an ESP-IDF flasher_args.json and uploads
// every section. Returns the uploaded sections (remote names) and flash size.
func (c *Client) UploadFirmwareFromFlasherArgs(ctx context.Context, path string) ([]FlashSection, *int, error) {
	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()
	if !connected {
		return nil, nil, fmt.Errorf("not connected")
	}

	sections, flashSize, err := ParseFlasherArgs(path)
	if err != nil {
		return nil, nil, err
	}
	uploaded, err := c.UploadFirmwareSections(ctx, sections)
	if err != nil {
		return nil, nil, err
	}
	return uploaded, flashSize, nil
}

// UploadFirmware uploads the file at path as its basename and returns the
// remote name for reference in sim:start. This is the simple single-blob path
// used by the monitor when only an app image is available; the section-aware
// UploadFirmwareFromFlasherArgs is the correct path for a bootable ESP-IDF image
// (see wiki/wokwi-sim-api-analysis.md).
func (c *Client) UploadFirmware(ctx context.Context, path string) (string, error) {
	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()
	if !connected {
		return "", fmt.Errorf("not connected")
	}
	if path == "" {
		return "", fmt.Errorf("firmware path not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read firmware: %w", err)
	}
	name := filepath.Base(path)
	if err := c.UploadFile(ctx, name, data); err != nil {
		return "", err
	}
	return name, nil
}

// UploadELF uploads the ELF file (debug symbols) as "firmware.elf". Returns the
// uploaded name, or "" when elfPath is empty. The ELF is optional but speeds up
// some simulations.
func (c *Client) UploadELF(ctx context.Context, elfPath string) (string, error) {
	if elfPath == "" {
		return "", nil
	}
	data, err := os.ReadFile(elfPath)
	if err != nil {
		return "", fmt.Errorf("read ELF: %w", err)
	}
	if err := c.UploadFile(ctx, "firmware.elf", data); err != nil {
		return "", fmt.Errorf("upload ELF: %w", err)
	}
	return "firmware.elf", nil
}

// StartSimulation starts the simulation with the given parameters.
//
// firmware is either a single uploaded filename (string) or a list of flash
// sections ([]FlashSection) produced by UploadFirmwareSections. The Wokwi
// server accepts both shapes, so the JSON omits an empty field.
func (c *Client) StartSimulation(ctx context.Context, params SimStartParams) error {
	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()
	if !connected {
		return fmt.Errorf("not connected")
	}

	body := map[string]interface{}{}
	switch f := params.Firmware.(type) {
	case string:
		if f != "" {
			body["firmware"] = f
		}
	case []FlashSection:
		if len(f) > 0 {
			body["firmware"] = f
		}
	}
	if params.Elf != "" {
		body["elf"] = params.Elf
	}
	if len(params.Chips) > 0 {
		body["chips"] = params.Chips
	}
	if params.FlashSize != nil {
		body["flashSize"] = *params.FlashSize
	}
	if params.Pause {
		body["pause"] = true
	}

	_, err := c.transport.Request(ctx, "sim:start", body)
	if err != nil {
		return fmt.Errorf("failed to start simulation: %w", err)
	}
	log.Info().Msg("Simulation started")
	return nil
}

// PauseSimulation pauses the running simulation.
func (c *Client) PauseSimulation(ctx context.Context) error {
	_, err := c.transport.Request(ctx, "sim:pause", nil)
	return err
}

// ResumeSimulation resumes the simulation, optionally pausing after pauseAfter
// nanoseconds (nil = run freely).
func (c *Client) ResumeSimulation(ctx context.Context, pauseAfter *int) error {
	body := map[string]interface{}{}
	if pauseAfter != nil {
		body["pauseAfter"] = *pauseAfter
	}
	_, err := c.transport.Request(ctx, "sim:resume", body)
	return err
}

// RestartSimulation restarts the simulation, optionally starting paused.
func (c *Client) RestartSimulation(ctx context.Context, pause bool) error {
	body := map[string]interface{}{}
	if pause {
		body["pause"] = true
	}
	_, err := c.transport.Request(ctx, "sim:restart", body)
	return err
}

// ListenSerial subscribes to serial output by issuing the serial-monitor:listen
// command. It must be called before events arrive on SubscribeSerial.
func (c *Client) ListenSerial(ctx context.Context) error {
	_, err := c.transport.Request(ctx, "serial-monitor:listen", map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("serial-monitor:listen: %w", err)
	}
	return nil
}

// SubscribeSerial subscribes to serial-monitor:data events.
func (c *Client) SubscribeSerial() chan EventMessage {
	return c.transport.Subscribe("serial-monitor:data")
}

// UnsubscribeSerial removes a serial subscription.
func (c *Client) UnsubscribeSerial(ch chan EventMessage) {
	c.transport.Unsubscribe("serial-monitor:data", ch)
}

// ReadSerialBytes extracts the UTF-8 bytes from a serial-monitor:data event.
// The payload carries "bytes" as an array of integers on the wire.
func (c *Client) ReadSerialBytes(ev EventMessage) []byte {
	return BytesFromPayload(ev.Payload, "bytes")
}

// BytesFromPayload extracts a numeric byte array from an event payload map.
// It is exported for callers/tests that already hold an EventMessage.
func BytesFromPayload(payload map[string]interface{}, key string) []byte {
	if payload == nil {
		return nil
	}
	raw, ok := payload[key]
	if !ok {
		return nil
	}
	nums, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	b := make([]byte, 0, len(nums))
	for _, n := range nums {
		switch v := n.(type) {
		case float64:
			b = append(b, byte(v))
		case int:
			b = append(b, byte(v))
		case int64:
			b = append(b, byte(v))
		case uint8:
			b = append(b, v)
		}
	}
	return b
}

// WriteSerial writes bytes to the simulator's serial input
// (serial-monitor:write with a numeric byte array).
func (c *Client) WriteSerial(ctx context.Context, data []byte) error {
	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()
	if !connected {
		return fmt.Errorf("not connected")
	}

	bytes := make([]interface{}, len(data))
	for i, b := range data {
		bytes[i] = int(b)
	}

	_, err := c.transport.Request(ctx, "serial-monitor:write", map[string]interface{}{
		"bytes": bytes,
	})
	return err
}

// WaitForOutput blocks until pattern is found in the serial stream or the
// context times out.
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
		case event, ok := <-serialCh:
			if !ok {
				return fmt.Errorf("serial channel closed")
			}
			buffer = append(buffer, c.ReadSerialBytes(event)...)
			if strings.Contains(string(buffer), pattern) {
				return nil
			}
			if len(buffer) > 1<<16 {
				buffer = buffer[len(buffer)-(1<<16):]
			}
		}
	}
}

// StreamOutput writes all serial output to out until the context is cancelled.
// A cancelled/timed-out context is a normal stop and returns nil; only a write
// failure is reported as an error.
func (c *Client) StreamOutput(ctx context.Context, out io.Writer) error {
	serialCh := c.SubscribeSerial()
	defer c.UnsubscribeSerial(serialCh)

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-serialCh:
			if !ok {
				return nil
			}
			if _, err := out.Write(c.ReadSerialBytes(event)); err != nil {
				return err
			}
		}
	}
}
