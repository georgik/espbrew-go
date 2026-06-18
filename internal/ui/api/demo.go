//go:build js
// +build js

package api

import (
	"syscall/js"
)

var demoMode bool

// InitDemoMode initializes demo mode detection from URL parameters
// Checks for ?demo=true, ?demo=1, or #demo in URL
// Must be called before any API calls to ensure demo mode is active
func InitDemoMode() {
	global := js.Global()
	location := global.Get("location")

	if location.IsUndefined() || location.IsNull() {
		return
	}

	search := location.Get("search").String()
	hash := location.Get("hash").String()

	// Check URL parameters ?demo=true or ?demo=1
	demoMode = containsParam(search, "demo=true") || containsParam(search, "demo=1")

	// Also check hash fragment #demo
	if !demoMode && len(hash) > 0 {
		demoMode = contains(hash, "demo")
	}
}

// DemoModeEnabled returns true if demo mode is active
func DemoModeEnabled() bool {
	return demoMode
}

// containsParam checks if URL contains a parameter (handles & and ? separators)
func containsParam(search, param string) bool {
	if len(search) == 0 {
		return false
	}

	// Check with ? prefix
	if contains(search, "?"+param) {
		return true
	}

	// Check with & prefix
	if contains(search, "&"+param) {
		return true
	}

	return false
}

// contains checks if substr exists in s
func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockImageURL returns a demo image data URI for the given capture path
// Generates SVG placeholders inline for demo mode
func mockImageURL(path string) string {
	// Generate a deterministic seed based on path
	seed := 0
	for i := 0; i < len(path); i++ {
		seed += int(path[i])
	}

	// Use different dimensions based on whether it's a main capture or subimage
	if contains(path, "subimages") {
		// Subimages are smaller crops
		return generateSVGPlaceholder(200, 150, "6c5ce7", "Device Capture")
	}

	// Main captures are larger
	width := 800
	height := 600

	// Generate a tech-style placeholder with varying colors based on seed
	colors := []string{"#6c5ce7", "#00cec9", "#0984e3", "#e84393", "#fdcb6e"}
	color := colors[seed%len(colors)]

	return generateSVGPlaceholder(width, height, color, "ESP Capture")
}

// generateSVGPlaceholder creates an SVG data URI placeholder image
func generateSVGPlaceholder(width, height int, bgColor, text string) string {
	// Create a simple SVG with the given background color and text
	svg := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ` + formatInt(width) + ` ` + formatInt(height) + `">
  <rect width="100%" height="100%" fill="` + bgColor + `"/>
  <text x="50%" y="50%" font-family="Arial, sans-serif" font-size="24" fill="white" text-anchor="middle" dominant-baseline="middle">` + text + `</text>
</svg>`

	// Encode to base64
	encoded := base64Encode(svg)
	return "data:image/svg+xml;base64," + encoded
}

// base64Encode performs base64 encoding on a string
func base64Encode(s string) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := ""
	var n uint32
	for i := 0; i < len(s); i++ {
		n = n<<8 | uint32(s[i])
		if i%3 == 2 {
			result += string(chars[(n>>18)&0x3F])
			result += string(chars[(n>>12)&0x3F])
			result += string(chars[(n>>6)&0x3F])
			result += string(chars[n&0x3F])
			n = 0
		}
	}
	remaining := len(s) % 3
	if remaining > 0 {
		n = n << (8 * (3 - remaining))
		result += string(chars[(n>>18)&0x3F])
		if remaining == 2 {
			result += string(chars[(n>>12)&0x3F])
			result += string(chars[(n>>6)&0x3F])
		} else {
			result += string(chars[(n>>12)&0x3F])
			result += "="
		}
		result += "="
	}
	return result
}

// formatInt formats an integer as a string
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}

	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}

	digits := []byte{}
	for n > 0 {
		digits = append([]byte{'0' + byte(n%10)}, digits...)
		n /= 10
	}

	return sign + string(digits)
}

// Mock data generators for demo mode

// mockStatus returns mock cluster status for demo mode
func mockStatus() *StatusResponse {
	return &StatusResponse{
		Nodes: []NodeStatus{
			{
				ID:      "leader-node-001",
				Address: "192.168.1.100:8080",
				Role:    "leader",
				Mode:    ModeOperational,
			},
			{
				ID:      "worker-node-001",
				Address: "192.168.1.101:8080",
				Role:    "worker",
				Mode:    ModeOperational,
			},
			{
				ID:      "worker-node-002",
				Address: "192.168.1.102:8080",
				Role:    "worker",
				Mode:    ModeOperational,
			},
		},
		Mode:        ModeOperational,
		DeviceCount: 6,
		CameraCount: 2,
	}
}

// mockDevices returns mock device list for demo mode
func mockDevices() []Device {
	return []Device{
		{
			DeviceID:  "esp32-devkit-a",
			Path:      "/dev/ttyUSB0",
			ChipType:  "ESP32",
			Status:    "available",
			Aliases:   []string{"ESP32 DevKit A"},
			NodeID:    "worker-node-001",
			Protected: false,
			Backend:   "serial",
			BackendConfig: map[string]interface{}{
				"baud_rate": 115200,
			},
		},
		{
			DeviceID:  "esp32-cam-001",
			Path:      "/dev/ttyUSB1",
			ChipType:  "ESP32",
			Status:    "busy",
			Aliases:   []string{"ESP32-CAM"},
			NodeID:    "worker-node-001",
			Protected: false,
			Backend:   "serial",
			BackendConfig: map[string]interface{}{
				"baud_rate": 115200,
			},
		},
		{
			DeviceID:  "esp8266-generic",
			Path:      "/dev/ttyUSB2",
			ChipType:  "ESP8266",
			Status:    "available",
			Aliases:   []string{"NodeMCU"},
			NodeID:    "worker-node-002",
			Protected: false,
			Backend:   "serial",
			BackendConfig: map[string]interface{}{
				"baud_rate": 115200,
			},
		},
		{
			DeviceID:  "esp32-s3-devkit",
			Path:      "/dev/ttyACM1",
			ChipType:  "ESP32-S3",
			Status:    "available",
			Aliases:   []string{"ESP32-S3 DevKit"},
			NodeID:    "leader-node-001",
			Protected: false,
			Backend:   "serial",
			BackendConfig: map[string]interface{}{
				"baud_rate": 115200,
			},
		},
		{
			DeviceID:  "esp32-c3-mini",
			Path:      "/dev/ttyUSB3",
			ChipType:  "ESP32-C3",
			Status:    "available",
			Aliases:   []string{"ESP32-C3 Mini"},
			NodeID:    "worker-node-002",
			Protected: false,
			Backend:   "serial",
			BackendConfig: map[string]interface{}{
				"baud_rate": 115200,
			},
		},
		{
			DeviceID:  "esp32s2-box",
			Path:      "/dev/ttyUSB4",
			ChipType:  "ESP32-S2",
			Status:    "disabled",
			Aliases:   []string{"ESP32-S2 Box"},
			NodeID:    "worker-node-001",
			Protected: false,
			Disabled:  true,
			Backend:   "serial",
			BackendConfig: map[string]interface{}{
				"baud_rate": 115200,
			},
		},
	}
}

// mockDevice returns a specific mock device by ID
func mockDevice(deviceID string) *Device {
	devices := mockDevices()
	for i := range devices {
		if devices[i].DeviceID == deviceID {
			return &devices[i]
		}
	}
	return nil
}

// mockCameras returns mock camera list for demo mode
func mockCameras() []Camera {
	return []Camera{
		{
			ID:      "cam-hd-webcam",
			Name:    "HD Webcam 1080p",
			Path:    "/dev/video0",
			Status:  "available",
			Backend: "v4l2",
			NodeID:  "leader-node-001",
		},
		{
			ID:      "cam-esp32cam",
			Name:    "ESP32-CAM OV2640",
			Path:    "http://192.168.1.100",
			Status:  "busy",
			Backend: "http",
			NodeID:  "worker-node-001",
		},
		{
			ID:      "cam-usb-720p",
			Name:    "USB 720p Camera",
			Path:    "/dev/video1",
			Status:  "available",
			Backend: "v4l2",
			NodeID:  "leader-node-001",
		},
	}
}

// mockCamera returns a specific mock camera by ID
func mockCamera(cameraID string) *Camera {
	cameras := mockCameras()
	for i := range cameras {
		if cameras[i].ID == cameraID {
			return &cameras[i]
		}
	}
	return nil
}

// mockCameraControls returns mock camera controls for demo mode
func mockCameraControls(cameraID string) *CameraControlsResponse {
	return &CameraControlsResponse{
		Current: map[string]int32{
			"brightness":                128,
			"contrast":                  32,
			"saturation":                64,
			"sharpness":                 4,
			"gain":                      0,
			"focus_absolute":            120,
			"exposure_absolute":         256,
			"white_balance_temperature": 4000,
		},
		Available: true,
		Platform:  "linux",
		DisplayPreset: map[string]int32{
			"brightness": 180,
			"contrast":   60,
			"saturation": 80,
			"sharpness":  6,
		},
		FocusPresets: map[string]int32{
			"close":   200,
			"display": 120,
			"far":     0,
		},
		Ranges: map[string]ControlRange{
			"brightness":                {Min: -64, Max: 64, Current: 128},
			"contrast":                  {Min: 0, Max: 64, Current: 32},
			"saturation":                {Min: 0, Max: 128, Current: 64},
			"sharpness":                 {Min: 0, Max: 8, Current: 4},
			"gain":                      {Min: 0, Max: 100, Current: 0},
			"focus_absolute":            {Min: 0, Max: 255, Current: 120},
			"exposure_absolute":         {Min: 1, Max: 10000, Current: 256},
			"white_balance_temperature": {Min: 2000, Max: 10000, Current: 4000},
		},
	}
}

// mockCameraSettings returns mock camera settings for demo mode
func mockCameraSettings(cameraID string) *CameraSettings {
	return &CameraSettings{
		CameraID:         cameraID,
		Name:             "Demo Camera Settings",
		Brightness:       128,
		Contrast:         32,
		Saturation:       64,
		Sharpness:        4,
		Gain:             0,
		Focus:            120,
		Exposure:         256,
		WhiteBalance:     4000,
		AutoExposure:     false,
		AutoFocus:        false,
		AutoWhiteBalance: false,
	}
}

// mockCaptures returns mock capture list for demo mode
func mockCaptures() []Capture {
	return []Capture{
		{
			Path:       mockImageURL("/captures/2026-06-18/esp32-board-001.jpg"),
			Filename:   "esp32-board-001.jpg",
			CameraID:   "cam-hd-webcam",
			CameraName: "HD Webcam 1080p",
			Timestamp:  1718698200000,
			Size:       245678,
		},
		{
			Path:       mockImageURL("/captures/2026-06-18/esp32-board-002.jpg"),
			Filename:   "esp32-board-002.jpg",
			CameraID:   "cam-hd-webcam",
			CameraName: "HD Webcam 1080p",
			Timestamp:  1718698260000,
			Size:       238456,
		},
		{
			Path:       mockImageURL("/captures/2026-06-18/esp32-cam-test.jpg"),
			Filename:   "esp32-cam-test.jpg",
			CameraID:   "cam-esp32cam",
			CameraName: "ESP32-CAM OV2640",
			Timestamp:  1718698320000,
			Size:       156234,
		},
		{
			Path:       mockImageURL("/captures/2026-06-17/production-run-01.jpg"),
			Filename:   "production-run-01.jpg",
			CameraID:   "cam-hd-webcam",
			CameraName: "HD Webcam 1080p",
			Timestamp:  1718611800000,
			Size:       267890,
		},
		{
			Path:       mockImageURL("/captures/2026-06-17/production-run-02.jpg"),
			Filename:   "production-run-02.jpg",
			CameraID:   "cam-hd-webcam",
			CameraName: "HD Webcam 1080p",
			Timestamp:  1718611860000,
			Size:       254321,
		},
	}
}

// mockCapturesPaginated returns mock captures with pagination
func mockCapturesPaginated(page, limit int) ([]Capture, int, int) {
	allCaptures := mockCaptures()

	// Calculate pagination
	total := len(allCaptures)
	totalPages := (total + limit - 1) / limit

	// Calculate slice bounds
	start := (page - 1) * limit
	if start >= total {
		return []Capture{}, total, totalPages
	}

	end := start + limit
	if end > total {
		end = total
	}

	return allCaptures[start:end], total, totalPages
}

// mockCameraMappings returns mock camera mappings for demo mode
func mockCameraMappings(cameraID string) *CameraMappingsResponse {
	return &CameraMappingsResponse{
		CameraID: cameraID,
		Calibration: &CalibrationInfo{
			Version:     1,
			Description: "Initial calibration for demo",
		},
		Mappings: []DeviceMappingWithDevice{
			{
				ID:         "mapping-001",
				DeviceID:   "esp32-devkit-a",
				CameraID:   cameraID,
				CameraName: "HD Webcam 1080p",
				Bounds: BoundingBox{
					X:      0.1,
					Y:      0.1,
					Width:  0.3,
					Height: 0.3,
				},
				CalibrationVersion: 1,
				Adjustment: ImageAdjustment{
					Brightness: 10,
					Contrast:   5,
					Saturation: 0,
				},
				CreatedAt: "2026-06-18T10:00:00Z",
				UpdatedAt: "2026-06-18T10:00:00Z",
				Device: &DeviceInfo{
					DeviceID: "esp32-devkit-a",
					ChipType: "ESP32",
					Aliases:  []string{"ESP32 DevKit A"},
				},
			},
			{
				ID:         "mapping-002",
				DeviceID:   "esp32-cam-001",
				CameraID:   cameraID,
				CameraName: "HD Webcam 1080p",
				Bounds: BoundingBox{
					X:      0.6,
					Y:      0.1,
					Width:  0.3,
					Height: 0.3,
				},
				CalibrationVersion: 1,
				Adjustment: ImageAdjustment{
					Brightness: 15,
					Contrast:   8,
					Saturation: 5,
				},
				CreatedAt: "2026-06-18T10:05:00Z",
				UpdatedAt: "2026-06-18T10:05:00Z",
				Device: &DeviceInfo{
					DeviceID: "esp32-cam-001",
					ChipType: "ESP32",
					Aliases:  []string{"ESP32-CAM"},
				},
			},
		},
	}
}

// mockCaptureResponse returns mock capture response for demo mode
func mockCaptureResponse() *CaptureResponse {
	return &CaptureResponse{
		Status:    "success",
		Path:      mockImageURL("/captures/2026-06-18/demo-capture.jpg"),
		CameraID:  "cam-hd-webcam",
		Timestamp: 1718698380000,
	}
}

// mockFlashUploadResponse returns mock flash upload response for demo mode
func mockFlashUploadResponse() *FlashUploadResponse {
	return &FlashUploadResponse{
		FileID: "demo-firmware-001",
		Size:   1048576, // 1MB
	}
}

// mockFlashJobResponse returns mock flash job response for demo mode
func mockFlashJobResponse() *FlashJobResponse {
	return &FlashJobResponse{
		JobID:      "demo-flash-job-001",
		Status:     "pending",
		DevicePath: "/dev/ttyUSB0",
	}
}

// mockFlashProgress returns mock flash progress for demo mode
func mockFlashProgress(jobID string) *FlashProgress {
	return &FlashProgress{
		JobID:    jobID,
		Status:   "completed",
		Progress: 100,
		Message:  "Flash completed successfully",
	}
}

// mockCaptureDeviceCaptures returns mock device-specific subimages for a capture
func mockCaptureDeviceCaptures(capturePath string) []DeviceCaptureInfo {
	return []DeviceCaptureInfo{
		{
			DeviceID: "esp32-devkit-a",
			Bounds: BoundingBox{
				X:      0.1,
				Y:      0.1,
				Width:  0.3,
				Height: 0.3,
			},
			Subimage: mockImageURL("/captures/2026-06-18/subimages/esp32-devkit-a.jpg"),
			Adjustment: ImageAdjustment{
				Brightness: 10,
				Contrast:   5,
				Saturation: 0,
			},
			GeneratedAt: "2026-06-18T10:30:00Z",
		},
		{
			DeviceID: "esp32-cam-001",
			Bounds: BoundingBox{
				X:      0.6,
				Y:      0.1,
				Width:  0.3,
				Height: 0.3,
			},
			Subimage: mockImageURL("/captures/2026-06-18/subimages/esp32-cam-001.jpg"),
			Adjustment: ImageAdjustment{
				Brightness: 15,
				Contrast:   8,
				Saturation: 5,
			},
			GeneratedAt: "2026-06-18T10:30:01Z",
		},
	}
}

// mockDeviceCaptures returns mock captures for a specific device
func mockDeviceCaptures(deviceID string) []Capture {
	captures := mockCaptures()
	if len(captures) > 0 {
		return captures[:2]
	}
	return []Capture{}
}

// mockBoundingBox returns mock bounding box mapping for demo mode
func mockBoundingBox(mappingID string) *DeviceBoundingBoxMapping {
	return &DeviceBoundingBoxMapping{
		ID:         mappingID,
		DeviceID:   "esp32-devkit-a",
		CameraID:   "cam-hd-webcam",
		CameraName: "HD Webcam 1080p",
		Bounds: BoundingBox{
			X:      0.1,
			Y:      0.1,
			Width:  0.3,
			Height: 0.3,
		},
		CalibrationVersion: 1,
		Adjustment: ImageAdjustment{
			Brightness: 10,
			Contrast:   5,
			Saturation: 0,
		},
	}
}

// mockCreateMappingResponse returns mock create mapping response for demo mode
func mockCreateMappingResponse() *CreateMappingResponse {
	return &CreateMappingResponse{
		ID:      "mapping-new-001",
		Status:  "created",
		Message: "Mapping created successfully",
	}
}

// mockCalibration returns mock calibration info for demo mode
func mockCalibration(cameraID string) *CalibrationInfo {
	return &CalibrationInfo{
		Version:     1,
		Description: "Demo calibration",
	}
}

// mockESPBootSequence returns a realistic ESP32 boot sequence for demo mode
func mockESPBootSequence() []string {
	return []string{
		"ESP-ROM:esp32s3-20210327",
		"Build:Mar 27 2021",
		"rst:0x15 (USB_UART_CHIP_RESET),boot:0xb (SPI_FAST_FLASH_BOOT)",
		"Saved PC:0x40378ae6",
		"--- 0x40378ae6: esp_cpu_wait_for_intr at /path/to/esp-idf/components/esp_hw_support/cpu.c:64",
		"SPIWP:0xee",
		"mode:DIO, clock div:1",
		"load:0x3fce2820,len:0x14f0",
		"load:0x403c8700,len:0xda0",
		"--- 0x403c8700: _stext at ??:?",
		"load:0x403cb700,len:0x2f58",
		"entry 0x403c8908",
		"--- 0x403c8908: call_start_cpu0 at /path/to/esp-idf/components/bootloader/subproject/main/bootloader_start.c:27",
		"I (24) boot: ESP-IDF v5.1.1 2nd stage bootloader",
		"I (25) boot: compile time Jan 15 2024 12:00:00",
		"I (25) boot: Multicore bootloader",
		"I (27) boot: chip revision: v0.1",
		"I (30) boot: efuse block revision: v1.2",
		"I (33) boot.esp32s3: Boot SPI Speed : 80MHz",
		"I (37) boot.esp32s3: SPI Mode       : DIO",
		"I (41) boot.esp32s3: SPI Flash Size : 2MB",
		"I (45) boot: Enabling RNG early entropy source...",
		"I (49) boot: Partition Table:",
		"I (52) boot: ## Label            Usage          Type ST Offset   Length",
		"I (58) boot:  0 nvs              WiFi data        01 02 00009000 00006000",
		"I (65) boot:  1 phy_init         RF data          01 01 0000f000 00001000",
		"I (71) boot:  2 factory          factory app      00 00 00010000 00100000",
		"I (78) boot: End of partition table",
		"I (81) esp_image: segment 0: paddr=00010020 vaddr=3c010020 size=07294h ( 29332) map",
		"I (94) esp_image: segment 1: paddr=000172bc vaddr=3fc8f800 size=02e0ch ( 11788) load",
		"I (98) esp_image: segment 2: paddr=0001a0d0 vaddr=40374000 size=05f48h ( 24392) load",
		"I (109) esp_image: segment 3: paddr=00020020 vaddr=42000020 size=0de14h ( 56852) map",
		"I (121) esp_image: segment 4: paddr=0002de3c vaddr=40379f48 size=057cch ( 22476) load",
		"I (126) esp_image: segment 5: paddr=00033610 vaddr=50000000 size=00024h (    36) load",
		"I (132) boot: Loaded app from partition at offset 0x10000",
		"I (132) boot: Disabling RNG early entropy source...",
		"I (147) cpu_start: Multicore app",
		"I (155) cpu_start: GPIO 44 and 43 are used as console UART I/O pins",
		"I (156) cpu_start: Pro cpu start user code",
		"I (156) cpu_start: cpu freq: 160000000 Hz",
		"I (158) app_init: Application information:",
		"I (162) app_init: Project name:     esp_application",
		"I (166) app_init: App version:      1.0.0",
		"I (172) app_init: Compile time:     Jan 15 2024 12:00:00",
		"I (177) app_init: ELF file SHA256:  <hash>",
		"I (181) app_init: ESP-IDF:          v5.1.1",
		"I (187) efuse_init: Min chip rev:     v0.0",
		"I (191) efuse_init: Max chip rev:     v0.99",
		"I (195) efuse_init: Chip rev:         v0.1",
		"I (199) heap_init: Initializing. RAM available for dynamic allocation:",
		"I (205) heap_init: At 3FC92F30 len 000567E0 (345 KiB): RAM",
		"I (210) heap_init: At 3FCE9710 len 00005724 (21 KiB): RAM",
		"I (215) heap_init: At 3FCF0000 len 00008000 (32 KiB): DRAM",
		"I (221) heap_init: At 600FE000 len 00001FE8 (7 KiB): RTCRAM",
		"I (227) spi_flash: detected chip: gd",
		"I (229) spi_flash: flash io: dio",
		"W (232) spi_flash: Detected size(16384k) larger than the size in the binary image header(2048k). Using the size in the binary image header.",
		"I (245) sleep_gpio: Configure to isolate all GPIO pins in sleep state",
		"I (251) sleep_gpio: Enable automatic switching of GPIO sleep configuration",
		"I (258) main_task: Started on CPU0",
		"I (268) main_task: Calling app_main()",
		"ESP32-S3 demo application started",
		"I (278) main_task: Returned from app_main()",
	}
}

// mockMonitorWebSocket creates a mock monitor WebSocket for demo mode
// It simulates a serial monitor connection by streaming boot sequence
type mockMonitorWebSocket struct {
	onMessage func(string)
	onError   func(error)
	onClose   func()
	connected bool
	closed    bool
}

// NewMockMonitorWebSocket creates a mock monitor for demo mode
func NewMockMonitorWebSocket(config *MonitorConfig) *mockMonitorWebSocket {
	mw := &mockMonitorWebSocket{
		connected: true,
		closed:    false,
	}

	// Start streaming boot sequence after short delay
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if mw.closed {
			return nil
		}
		mw.streamBootSequence()
		return nil
	}), 500)

	return mw
}

// streamBootSequence streams the ESP boot sequence line by line
func (mw *mockMonitorWebSocket) streamBootSequence() {
	if mw.closed {
		return
	}

	bootSequence := mockESPBootSequence()

	// Stream each line with realistic timing
	for i, line := range bootSequence {
		if mw.closed {
			return
		}

		// Add index and line to simulate format
		lineWithNewline := line + "\n"

		// Calculate delay (faster at start, slower during boot)
		delay := 50
		if i > 5 && i < 20 {
			delay = 100 // Slower during bootloader
		} else if i >= 20 {
			delay = 75 // Normal speed during app init
		}

		// Use setTimeout to stream each line
		currentDelay := i * delay
		js.Global().Get("setTimeout").Invoke(js.FuncOf(func(lineIndex int, lineContent string) func(js.Value, []js.Value) interface{} {
			return func(this js.Value, args []js.Value) interface{} {
				if mw.closed || mw.onMessage == nil {
					return nil
				}
				mw.onMessage(lineContent)
				return nil
			}
		}(i, lineWithNewline)), currentDelay)
	}

	// After boot completes, show periodic activity
	js.Global().Get("setInterval").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if mw.closed || mw.onMessage == nil {
			return nil
		}
		// Show occasional activity message
		activityMessages := []string{
			"I (1234) heap_init: Free heap: 234567 bytes",
			"I (2345) wifi: WiFi event: STA_START",
			"I (3456) wifi: WiFi event: STA_CONNECTED",
			"I (4567) tcpip: TCP/IP initialized",
		}
		msg := activityMessages[int(js.Global().Get("Math").Call("random").Float()*4)]
		mw.onMessage(msg + "\n")
		return nil
	}), 5000)
}

// SetMessageHandler sets the message handler for mock monitor
func (mw *mockMonitorWebSocket) SetMessageHandler(handler func(string)) {
	mw.onMessage = handler
}

// SetErrorHandler sets the error handler for mock monitor
func (mw *mockMonitorWebSocket) SetErrorHandler(handler func(error)) {
	mw.onError = handler
}

// SetCloseHandler sets the close handler for mock monitor
func (mw *mockMonitorWebSocket) SetCloseHandler(handler func()) {
	mw.onClose = handler
}

// Send simulates sending data to the serial port in demo mode
func (mw *mockMonitorWebSocket) Send(data string) error {
	if !mw.connected {
		return &MonitorError{Message: "Not connected"}
	}
	// Echo back what was sent (simulating local echo)
	if mw.onMessage != nil {
		mw.onMessage(data)
	}
	return nil
}

// Close closes the mock monitor connection
func (mw *mockMonitorWebSocket) Close() {
	mw.connected = false
	mw.closed = true
	if mw.onClose != nil {
		mw.onClose()
	}
}

// IsConnected returns the connection status
func (mw *mockMonitorWebSocket) IsConnected() bool {
	return mw.connected
}
