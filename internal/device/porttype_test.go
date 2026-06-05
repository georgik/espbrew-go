package device

import "testing"

func TestDetectPortType(t *testing.T) {
	tests := []struct {
		name     string
		vid      uint16
		pid      uint16
		product  string
		path     string
		wantType PortType
	}{
		{
			name:     "USB Serial/JTAG from product name",
			vid:      ESP_VID,
			pid:      ESP_PID_S3,
			product:  "USB JTAG/serial debug unit",
			path:     "/dev/ttyUSB0",
			wantType: PortTypeUSBSerialJTAG,
		},
		{
			name:     "External UART",
			vid:      0x10c4,
			pid:      0xea60,
			product:  "CP2102N USB to UART Bridge",
			path:     "/dev/ttyUSB0",
			wantType: PortTypeUART,
		},
		{
			name:     "S3 native USB",
			vid:      ESP_VID,
			pid:      ESP_PID_S3,
			product:  "ESP32-S3",
			path:     "/dev/cu.usbmodem1201",
			wantType: PortTypeUSBSerialJTAG,
		},
		{
			name:     "C3 native USB",
			vid:      ESP_VID,
			pid:      ESP_PID_C3,
			product:  "ESP32-C3",
			path:     "/dev/tty.usbmodem1201",
			wantType: PortTypeUSBSerialJTAG,
		},
		{
			name:     "C6 native USB",
			vid:      ESP_VID,
			pid:      ESP_PID_C6,
			product:  "ESP32-C6",
			path:     "/dev/cu.usbmodem1201",
			wantType: PortTypeUSBSerialJTAG,
		},
		{
			name:     "S3 with external UART",
			vid:      ESP_VID,
			pid:      ESP_PID_S3,
			product:  "ESP32-S3",
			path:     "/dev/ttyUSB0",
			wantType: PortTypeUART,
		},
		{
			name:     "JTAG with underscore",
			vid:      ESP_VID,
			pid:      ESP_PID_S3,
			product:  "USB JTAG_serial debug unit",
			path:     "/dev/ttyUSB0",
			wantType: PortTypeUSBSerialJTAG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectPortType(tt.vid, tt.pid, tt.product, tt.path)
			if got != tt.wantType {
				t.Errorf("DetectPortType() = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestIsNativeUSBPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/dev/cu.usbmodem1201", true},
		{"/dev/tty.usbmodem1201", true},
		{"/dev/ttyUSB0", false},
		{"/dev/ttyACM0", false},
		{"/dev/cu.usbserial-110", false},
		{"/dev/cu.SLAB_USBtoUART", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsNativeUSBPath(tt.path)
			if got != tt.want {
				t.Errorf("IsNativeUSBPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
