package flash

import (
	"os"
	"path/filepath"
	"testing"
)

// TestVendorInName checks the 4-hex vendor-id match against representative
// /dev/serial/by-id names, including the CH9102 board this feature targets and
// the Espressif native-USB boards that must NOT be treated as bridges.
//
// vendorInName is a pure string match with no OS dependency, so it is tested on
// every platform (Linux, macOS, Windows).
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

// mustSymlink is a small shared test helper (also used by the Linux-only
// bridge_linux_test.go) that creates a symlink or fails the test.
func mustSymlink(t *testing.T, dir, name, target string) {
	t.Helper()
	if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
}
