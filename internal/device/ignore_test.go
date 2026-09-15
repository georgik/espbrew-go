package device

import (
	"testing"
)

func TestIsIgnoredPort(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"macOS bluetooth incoming", "/dev/cu.Bluetooth-Incoming-Port", true},
		{"bluetooth (case-insensitive)", "/dev/cu.BLUETOOTH-Incoming-Port", true},
		{"incoming substring", "/dev/cu.Something-Incoming", true},
		{"buds", "/dev/cu.Earbuds-1", true},
		{"debug console", "/dev/cu.ESP32S3-Debug-Console", true},
		{"real usb serial", "/dev/cu.usbserial-1420", false},
		{"ttyACM esp", "/dev/ttyACM0", false},
		{"ttyUSB esp", "/dev/ttyUSB0", false},
		{"usbmodem", "/dev/cu.usbmodem14201", false},
		{"by-id esp", "/dev/serial/by-id/usb-Espressif_XXX", false},
		{"windows com", "COM3", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIgnoredPort(tt.path); got != tt.want {
				t.Errorf("IsIgnoredPort(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestFilterIgnoredPorts(t *testing.T) {
	ports := []Port{
		{Path: "/dev/cu.usbserial-14201", RealPath: "/dev/cu.usbserial-14201"},
		{Path: "/dev/cu.Bluetooth-Incoming-Port", RealPath: "/dev/cu.Bluetooth-Incoming-Port"},
		{Path: "/dev/ttyACM0", RealPath: "/dev/ttyACM0"},
		{Path: "/dev/serial/by-id/usb-Espressif_x", RealPath: "/dev/cu.usbserial-ESP"},
	}

	filtered := FilterIgnoredPorts(ports)

	// Bluetooth-Incoming-Port must be dropped; the other three remain, in order.
	if len(filtered) != 3 {
		t.Fatalf("expected 3 ports after filtering, got %d", len(filtered))
	}
	for _, p := range filtered {
		if IsIgnoredPort(p.Path) {
			t.Errorf("ignored port leaked through filter: %q", p.Path)
		}
	}

	wantPaths := []string{
		"/dev/cu.usbserial-14201",
		"/dev/ttyACM0",
		"/dev/serial/by-id/usb-Espressif_x",
	}
	for i, want := range wantPaths {
		if filtered[i].Path != want {
			t.Errorf("filtered[%d].Path = %q, want %q", i, filtered[i].Path, want)
		}
	}
}

func TestFilterIgnoredPorts_empty(t *testing.T) {
	if got := FilterIgnoredPorts(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
	if got := FilterIgnoredPorts([]Port{}); len(got) != 0 {
		t.Errorf("expected empty for empty input, got %v", got)
	}
}

// RealPath is also checked, so a port whose stable path looks fine but whose
// real path is ignored is still dropped.
func TestFilterIgnoredPorts_checksRealPath(t *testing.T) {
	ports := []Port{
		{Path: "/dev/serial/by-id/usb-Espressif_x", RealPath: "/dev/cu.Bluetooth-Incoming-Port"},
		{Path: "/dev/ttyACM0", RealPath: "/dev/ttyACM0"},
	}
	got := FilterIgnoredPorts(ports)
	if len(got) != 1 || got[0].Path != "/dev/ttyACM0" {
		t.Fatalf("expected only ttyACM0 to survive, got %v", got)
	}
}

func TestScannerIsLikelyESPExcludesIgnored(t *testing.T) {
	scanner := NewScanner()
	ignoredPaths := []string{
		"/dev/cu.Bluetooth-Incoming-Port",
		"/dev/cu.Earbuds-1",
		"/dev/cu.ESP32S3-Debug-Console",
	}
	for _, p := range ignoredPaths {
		if scanner.isLikelyESP(p) {
			t.Errorf("isLikelyESP(%q) = true, want false (ignored port)", p)
		}
	}

	// Real ESP ports must still be considered likely ESP.
	espPaths := []string{"/dev/cu.usbserial-14201", "/dev/ttyACM0", "/dev/ttyUSB0"}
	for _, p := range espPaths {
		if !scanner.isLikelyESP(p) {
			t.Errorf("isLikelyESP(%q) = false, want true (ESP port)", p)
		}
	}
}

func TestAddIgnoredPortPattern(t *testing.T) {
	// Save and restore the global ignore list to avoid cross-test pollution.
	original := append([]string(nil), ignoredPortPatterns...)
	defer func() { ignoredPortPatterns = original }()

	AddIgnoredPortPattern("__custom_non_esp_marker__")

	if !IsIgnoredPort("/dev/ttyACM0__custom_non_esp_marker__") {
		t.Error("expected custom pattern to mark port as ignored")
	}
	// A plain ESP port is unaffected by the custom pattern.
	if IsIgnoredPort("/dev/ttyACM0") {
		t.Error("plain ESP port should not be ignored")
	}

	// Adding the same pattern again must be a no-op (no duplicates).
	countBefore := len(ignoredPortPatterns)
	AddIgnoredPortPattern("__custom_non_esp_marker__")
	if len(ignoredPortPatterns) != countBefore {
		t.Error("AddIgnoredPortPattern should be idempotent")
	}

	// Empty / whitespace patterns are ignored.
	AddIgnoredPortPattern("   ")
	if len(ignoredPortPatterns) != countBefore {
		t.Error("AddIgnoredPortPattern should ignore empty patterns")
	}
}
