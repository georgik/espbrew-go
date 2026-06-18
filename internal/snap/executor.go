package snap

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/backend/wokwi"
	"codeberg.org/georgik/espbrew-go/internal/camera"
	"codeberg.org/georgik/espbrew-go/internal/monitor"
	"codeberg.org/georgik/espbrew-go/pkg/protocol"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Executor orchestrates the creation of device snapshots.
// It coordinates serial monitoring and camera capture to produce
// a snapshot of the device state. Flashing is not performed.
type Executor struct {
	// Device configuration
	device     string               // Serial port path or device ID
	deviceInfo *protocol.DeviceInfo // Device info from cluster (optional)
	backend    protocol.Monitor     // Backend monitor instance

	// Snapshot options
	duration time.Duration // How long to monitor serial output

	// Camera configuration
	cameraID      string // Camera device identifier
	displayPreset bool   // Apply display photography preset on Linux

	// Serial monitor configuration
	baudRate int // Baud rate for serial communication

	// Control flags
	skipFlash bool // Always true - flashing is not supported
	noCapture bool // Disable camera capture
	noMonitor bool // Disable serial monitoring

	// Internal state
	mu      sync.Mutex
	result  *SnapResult
	logChan chan SerialLogEntry
	imageCh chan []byte
}

// NewExecutor creates a new snapshot executor with the given configuration.
//
// Parameters:
//   - device: Serial port path (e.g., "/dev/ttyUSB0")
//   - duration: How long to monitor serial output
//
// Returns a configured Executor ready to run.
func NewExecutor(device string, duration time.Duration) *Executor {
	return &Executor{
		device:    device,
		duration:  duration,
		baudRate:  115200, // Default baud rate
		skipFlash: true,   // Flashing is not supported
		logChan:   make(chan SerialLogEntry, 1000),
		imageCh:   make(chan []byte, 1),
		result:    &SnapResult{},
	}
}

// NewExecutorWithDevice creates a new snapshot executor with device info.
//
// Parameters:
//   - deviceInfo: Device information from cluster
//   - duration: How long to monitor serial output
//
// Returns a configured Executor ready to run with backend routing.
func NewExecutorWithDevice(deviceInfo *protocol.DeviceInfo, duration time.Duration) *Executor {
	device := deviceInfo.Path
	if deviceInfo.Backend != protocol.BackendPhysical && deviceInfo.Backend != "" {
		// For simulators, use device ID as identifier
		device = deviceInfo.DeviceID
	}

	return &Executor{
		device:     device,
		deviceInfo: deviceInfo,
		duration:   duration,
		baudRate:   115200, // Default baud rate
		skipFlash:  true,   // Flashing is not supported
		logChan:    make(chan SerialLogEntry, 1000),
		imageCh:    make(chan []byte, 1),
		result:     &SnapResult{},
	}
}

// SetBackendMonitor sets a custom backend monitor (for testing or advanced usage)
func (e *Executor) SetBackendMonitor(monitor protocol.Monitor) {
	e.backend = monitor
}

// GetDevice returns the device identifier
func (e *Executor) GetDevice() string {
	return e.device
}

// GetDeviceInfo returns the device info (may be nil for physical devices)
func (e *Executor) GetDeviceInfo() *protocol.DeviceInfo {
	return e.deviceInfo
}

// Run executes the snapshot workflow.
// It performs the following steps in order:
// 1. Generate unique snapshot ID
// 2. Optionally monitor serial output
// 3. Optionally capture camera image
// 4. Assemble and return the result
//
// The method blocks until the snapshot is complete or the context is cancelled.
func (e *Executor) Run(ctx context.Context) (*SnapResult, error) {
	startTime := time.Now()
	snapID := uuid.New().String()

	e.mu.Lock()
	e.result.Metadata.SnapID = snapID
	e.result.Metadata.Timestamp = startTime
	e.result.Metadata.DevicePath = e.device
	e.result.Metadata.FlashSkipped = true
	e.mu.Unlock()

	log.Info().
		Str("snap_id", snapID).
		Str("device", e.device).
		Dur("duration", e.duration).
		Msg("Starting snapshot")

	var status SnapStatus

	// Run monitor and capture in parallel
	var wg sync.WaitGroup
	var monitorErr, captureErr error

	// Start serial monitoring if enabled
	if !e.noMonitor {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitorErr = e.monitorSerial(ctx)
		}()
	}

	// Start image capture if enabled
	if !e.noCapture {
		wg.Add(1)
		go func() {
			defer wg.Done()
			captureErr = e.captureImage(ctx)
		}()
	}

	// Wait for both to complete
	wg.Wait()

	// Update status based on errors
	if monitorErr != nil {
		log.Warn().Err(monitorErr).Str("snap_id", snapID).Msg("Serial monitoring failed")
		status = SnapStatusPartial
	}
	if captureErr != nil {
		log.Warn().Err(captureErr).Str("snap_id", snapID).Msg("Image capture failed")
		if status == "" {
			status = SnapStatusPartial
		}
	}

	// Finalize result
	e.mu.Lock()
	defer e.mu.Unlock()

	e.result.Metadata.Duration = time.Since(startTime).Milliseconds()
	e.result.Metadata.LogEntryCount = len(e.result.Logs)

	if status == "" {
		status = SnapStatusSuccess
	}
	e.result.Metadata.Status = status

	// Encode image to base64 if available
	if len(e.result.ImageData) > 0 {
		e.result.ImageBase64 = base64.StdEncoding.EncodeToString(e.result.ImageData)
	}

	log.Info().
		Str("snap_id", snapID).
		Str("status", string(status)).
		Int64("duration_ms", e.result.Metadata.Duration).
		Int("log_count", len(e.result.Logs)).
		Int("image_size", len(e.result.ImageData)).
		Msg("Snapshot completed")

	return e.result, nil
}

// monitorSerial captures serial output from the device for the configured duration.
// It parses log lines and extracts log levels when possible.
func (e *Executor) monitorSerial(ctx context.Context) error {
	e.mu.Lock()
	e.result.Metadata.MonitorEnabled = true
	e.result.Metadata.MonitorDuration = e.duration
	e.result.Metadata.MonitorBaudRate = e.baudRate
	e.mu.Unlock()

	// Check if backend monitor is set (for simulators)
	if e.backend != nil {
		return e.monitorWithBackend(ctx)
	}

	// Check if device info is available with wokwi backend
	if e.deviceInfo != nil && e.deviceInfo.Backend == protocol.BackendWokwi {
		return e.monitorWokwi(ctx)
	}

	// Default to physical device monitoring
	return e.monitorPhysical(ctx)
}

// monitorWithBackend uses the configured backend monitor
func (e *Executor) monitorWithBackend(ctx context.Context) error {
	log.Debug().
		Str("device", e.device).
		Dur("duration", e.duration).
		Msg("Starting backend monitor")

	if err := e.backend.Start(ctx); err != nil {
		return fmt.Errorf("start backend monitor: %w", err)
	}
	defer e.backend.Stop()

	// Create context with timeout
	monitorCtx, cancel := context.WithTimeout(ctx, e.duration)
	defer cancel()

	entryCount := 0

	// Read log entries from backend
	for {
		select {
		case <-monitorCtx.Done():
			log.Debug().Int("entries", entryCount).Msg("Backend monitoring completed")
			return nil

		case entry, ok := <-e.backend.Output():
			if !ok {
				// Channel closed
				return nil
			}
			e.addLogEntry(entry.Data)
			entryCount++

		case <-time.After(100 * time.Millisecond):
			// Keep select alive
		}
	}
}

// monitorWokwi handles Wokwi simulator monitoring
func (e *Executor) monitorWokwi(ctx context.Context) error {
	log.Debug().
		Str("device", e.device).
		Dur("duration", e.duration).
		Msg("Starting Wokwi simulator monitor")

	// Create Wokwi monitor
	wokwiMonitor, err := wokwi.NewMonitor(e.deviceInfo)
	if err != nil {
		return fmt.Errorf("create wokwi monitor: %w", err)
	}

	// For now, we need to set the ELF path
	// This will be passed via flash operation or stored separately
	// TODO: Integrate with firmware storage

	e.backend = wokwiMonitor
	return e.monitorWithBackend(ctx)
}

// monitorPhysical handles physical device monitoring
func (e *Executor) monitorPhysical(ctx context.Context) error {
	log.Debug().
		Str("device", e.device).
		Int("baud", e.baudRate).
		Dur("duration", e.duration).
		Msg("Starting physical serial monitor")

	// Create context with timeout FIRST
	monitorCtx, cancel := context.WithTimeout(ctx, e.duration)
	defer cancel()

	// Create monitor session
	sessionID := uuid.New().String()
	session := monitor.NewStreamSession(sessionID, monitor.StreamConfig{
		Port:       e.device,
		BaudRate:   e.baudRate,
		Timeout:    e.duration,
		TimeoutCtx: monitorCtx,
	})

	if err := session.Start(); err != nil {
		return fmt.Errorf("start monitor: %w", err)
	}
	defer session.Close()

	// Buffer for incomplete lines
	var lineBuffer []byte
	entryCount := 0

	// Read serial data
	for {
		select {
		case <-monitorCtx.Done():
			// Flush remaining buffer
			if len(lineBuffer) > 0 {
				e.addLogEntry(string(lineBuffer))
				entryCount++
			}
			log.Debug().Int("entries", entryCount).Msg("Serial monitoring completed")
			return nil

		case data := <-session.Data():
			// Process incoming data line by line
			for _, b := range data {
				if b == '\n' || b == '\r' {
					if len(lineBuffer) > 0 {
						e.addLogEntry(string(lineBuffer))
						entryCount++
						lineBuffer = nil
					}
				} else if b >= 32 && b <= 126 { // Printable ASCII
					lineBuffer = append(lineBuffer, b)
				}
			}

		case err := <-session.Errors():
			if err != nil {
				log.Warn().Err(err).Msg("Serial monitor error")
				return err
			}
		}
	}
}

// addLogEntry adds a log entry with level detection
func (e *Executor) addLogEntry(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}

	entry := SerialLogEntry{
		Timestamp: time.Now(),
		Message:   message,
		Level:     e.detectLogLevel(message),
	}

	e.mu.Lock()
	e.result.Logs = append(e.result.Logs, entry)
	e.mu.Unlock()
}

// detectLogLevel attempts to detect log level from message
func (e *Executor) detectLogLevel(message string) string {
	msgLower := strings.ToLower(message)

	// Check for common log level patterns
	patterns := map[string]string{
		"error":   "error",
		"err":     "error",
		"fail":    "error",
		"fatal":   "error",
		"warn":    "warn",
		"warning": "warn",
		"info":    "info",
		"debug":   "debug",
		"trace":   "debug",
		"e!":      "error", // ESP logging format
		"w!":      "warn",  // ESP logging format
		"i!":      "info",  // ESP logging format
		"d!":      "debug", // ESP logging format
	}

	for pattern, level := range patterns {
		if strings.Contains(msgLower, pattern) {
			return level
		}
	}

	return "info" // Default level
}

// captureImage captures an image from the configured camera.
// It waits for the image data and stores it in the result.
func (e *Executor) captureImage(ctx context.Context) error {
	e.mu.Lock()
	e.result.Metadata.CaptureEnabled = true
	e.result.Metadata.CameraID = e.cameraID
	e.mu.Unlock()

	log.Debug().Str("camera", e.cameraID).Msg("Capturing image")

	// Apply camera controls if display preset is enabled
	if e.displayPreset {
		if err := e.applyCameraControls(); err != nil {
			log.Warn().Err(err).Msg("Failed to apply camera controls, continuing with defaults")
		}
	}

	// Create camera capturer
	capturer, err := camera.NewCapturerWithStore()
	if err != nil {
		return fmt.Errorf("create capturer: %w", err)
	}

	// Capture image
	req := &camera.CaptureRequest{
		CameraID: e.cameraID,
		Width:    640, // Default resolution
		Height:   480, // Default resolution
		Format:   "jpg",
		Quality:  85,
		Timeout:  5 * time.Second,
	}

	result, err := capturer.Capture(ctx, req)
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}

	// Store image data in result
	e.mu.Lock()
	e.result.ImageData = result.Data
	e.result.Metadata.ImageSize = len(result.Data)
	e.result.Metadata.ImageFormat = result.Format
	e.mu.Unlock()

	log.Info().
		Int("size", len(result.Data)).
		Int("width", result.Width).
		Int("height", result.Height).
		Msg("Image captured successfully")

	return nil
}

// applyCameraControls applies the display photography preset to the camera
// This only works on Linux with V4L2-compatible cameras
func (e *Executor) applyCameraControls() error {
	// Get the camera device path
	devicePath := e.cameraID
	if devicePath == "" {
		// Try to auto-detect on Linux
		if camera.ControllerAvailable() {
			devicePath = "/dev/video0" // Default device
		} else {
			log.Debug().Msg("Camera controls not available on this platform")
			return nil
		}
	}

	// Create camera controller
	ctrl, err := camera.NewController(devicePath)
	if err != nil {
		return fmt.Errorf("create camera controller: %w", err)
	}
	defer ctrl.Close()

	log.Info().
		Str("camera", devicePath).
		Msg("Applying display photography preset")

	// Apply the display preset
	if err := ctrl.SetDisplayPreset(); err != nil {
		return fmt.Errorf("set display preset: %w", err)
	}

	// Get and log the applied settings
	settings, err := ctrl.GetSettings()
	if err == nil {
		log.Info().
			Int32("brightness", settings["brightness"]).
			Int32("contrast", settings["contrast"]).
			Int32("sharpness", settings["sharpness"]).
			Int32("saturation", settings["saturation"]).
			Msg("Camera settings applied")
	}

	return nil
}

// SetBaudRate sets the baud rate for serial communication
func (e *Executor) SetBaudRate(baud int) {
	e.baudRate = baud
}

// SetCameraID sets the camera device identifier
func (e *Executor) SetCameraID(cameraID string) {
	e.cameraID = cameraID
}

// SetNoCapture sets whether to disable camera capture
func (e *Executor) SetNoCapture(noCapture bool) {
	e.noCapture = noCapture
}

// SetNoMonitor sets whether to disable serial monitoring
func (e *Executor) SetNoMonitor(noMonitor bool) {
	e.noMonitor = noMonitor
}

// GetSkipFlash returns whether flashing is skipped (always true)
func (e *Executor) GetSkipFlash() bool {
	return true
}

// GetNoCapture returns whether camera capture is disabled
func (e *Executor) GetNoCapture() bool {
	return e.noCapture
}

// GetNoMonitor returns whether serial monitoring is disabled
func (e *Executor) GetNoMonitor() bool {
	return e.noMonitor
}

// SetDisplayPreset enables display photography preset for camera controls
// When enabled, applies optimized settings for capturing glowing/backlit displays
// Only works on Linux with V4L2 cameras
func (e *Executor) SetDisplayPreset(enabled bool) {
	e.displayPreset = enabled
}

// GetDisplayPreset returns whether display preset is enabled
func (e *Executor) GetDisplayPreset() bool {
	return e.displayPreset
}
