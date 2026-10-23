package flash

import (
	"os"
	"path/filepath"
	"testing"
)

// TestVendorInName checks the 4-hex vendor-id match against representative
// /dev/serial/by-id names, including the CH9102 board this feature targets and
// the Espressif native-USB boards that must NOT be treated as bridges.
func TestVendorInName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"usb-1a86_USB_Single_Serial_5971084553-if00", true},   // WCH CH9102 (waveshare)
		{"usb-1a86_CH340USB_UART_CP2102_if00", true},           // WCH CH340
		{"usb-3171_CH341USB_SERIAL_if00", true},                // WCH CH341
		{"usb-10c4_CP2102USB_TO_UART_BRIDGE_if00", true},       // Silicon Labs CP2102
		{"usb-0403_ftdi_ft4232_usb_serial_if01", true},         // FTDI
		{"usb-303a_Espressif_USB_JTAG_serial_...-if00", false}, // native USB-JTAG (S3/C3)
		{"usb-303a_Espressif_USB_CDC_serial_...-if00", false},  // native USB CDC
		{"usb-1a87_USB_Hub_if00", false},                       // a hub with a different id
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := vendorInName(tc.name); got != tc.want {
				t.Errorf("vendorInName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestVendorInNameBounded ensures a 4-hex vendor id never matches a longer id
// that merely starts with it (e.g. "1a86" must not match "1a860").
func TestVendorInNameBounded(t *testing.T) {
	if vendorInName("usb-1a860_USB_Single_Serial_if00") {
		t.Fatal("1a860 must not match the 1a86 bridge vendor")
	}
}

// TestDetectBridgeBoardIn creates a fake /dev/serial/by-id tree and confirms the
// port is matched to the right by-id name and the vendor is recognised.
func TestDetectBridgeBoardIn(t *testing.T) {
	dir := t.TempDir()

	// Two boards: a CH9102 bridge on ttyACM5 and an Espressif native board on
	// ttyACM0. The symlink name (not target) carries the vendor id.
	mustSymlink(t, dir, "usb-1a86_USB_Single_Serial_5971084553-if00", "ttyACM5")
	mustSymlink(t, dir, "usb-303a_Espressif_USB_JTAG_serial_ABC-if00", "ttyACM0")

	if !detectBridgeBoardIn("/dev/ttyACM5", dir) {
		t.Error("ttyACM5 (CH9102) should be detected as a bridge")
	}
	if detectBridgeBoardIn("/dev/ttyACM0", dir) {
		t.Error("ttyACM0 (Espressif native) must NOT be detected as a bridge")
	}
	if detectBridgeBoardIn("/dev/ttyACM9", dir) {
		t.Error("an unknown port must not be detected as a bridge")
	}
}

// TestDetectBridgeBoardMissingDir confirms the safe default: when the by-id
// directory is absent the function returns false rather than erroring.
func TestDetectBridgeBoardMissingDir(t *testing.T) {
	if detectBridgeBoardIn("/dev/ttyACM5", filepath.Join(t.TempDir(), "nope")) {
		t.Error("missing by-id dir must return false")
	}
}

func mustSymlink(t *testing.T, dir, name, target string) {
	t.Helper()
	if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
}
