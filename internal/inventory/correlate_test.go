package inventory

import (
	"codeberg.org/georgik/espbrew-go/internal/device"
	"testing"
)

func TestCorrelatePorts(t *testing.T) {
	tests := []struct {
		name             string
		records          []PortRecord
		wantBoards       int
		wantUnidentified int
	}{
		{
			name: "single identified port",
			records: []PortRecord{
				{
					Path:       "/dev/ttyUSB0",
					MAC:        "84:0d:8e:18:8a:d0",
					Chip:       "ESP32",
					PortType:   device.PortTypeUART,
					Identified: true,
				},
			},
			wantBoards:       1,
			wantUnidentified: 0,
		},
		{
			name: "multiple ports same board",
			records: []PortRecord{
				{
					Path:       "/dev/cu.usbmodem1201",
					MAC:        "30:ed:a0:e4:6a:d0",
					Chip:       "ESP32-C5",
					PortType:   device.PortTypeUSBSerialJTAG,
					Identified: true,
				},
				{
					Path:       "/dev/cu.usbserial-110",
					MAC:        "30:ed:a0:e4:6a:d0",
					Chip:       "ESP32-C5",
					PortType:   device.PortTypeUART,
					Identified: true,
				},
			},
			wantBoards:       1,
			wantUnidentified: 0,
		},
		{
			name: "two different boards",
			records: []PortRecord{
				{
					Path:       "/dev/ttyUSB0",
					MAC:        "84:0d:8e:18:8a:d0",
					Chip:       "ESP32",
					PortType:   device.PortTypeUART,
					Identified: true,
				},
				{
					Path:       "/dev/ttyUSB1",
					MAC:        "30:ed:a0:e4:ca:88",
					Chip:       "ESP32-C5",
					PortType:   device.PortTypeUART,
					Identified: true,
				},
			},
			wantBoards:       2,
			wantUnidentified: 0,
		},
		{
			name: "mixed identified and unidentified",
			records: []PortRecord{
				{
					Path:       "/dev/ttyUSB0",
					MAC:        "84:0d:8e:18:8a:d0",
					Chip:       "ESP32",
					PortType:   device.PortTypeUART,
					Identified: true,
				},
				{
					Path:       "/dev/ttyUSB1",
					PortType:   device.PortTypeUART,
					Identified: false,
					ProbeError: "Timeout",
				},
			},
			wantBoards:       1,
			wantUnidentified: 1,
		},
		{
			name:             "empty records",
			records:          []PortRecord{},
			wantBoards:       0,
			wantUnidentified: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boards, unidentified := CorrelatePorts(tt.records)
			if len(boards) != tt.wantBoards {
				t.Errorf("CorrelatePorts() boards count = %d, want %d", len(boards), tt.wantBoards)
			}
			if len(unidentified) != tt.wantUnidentified {
				t.Errorf("CorrelatePorts() unidentified count = %d, want %d", len(unidentified), tt.wantUnidentified)
			}
		})
	}
}

func TestAttachUnidentifiedUSJ(t *testing.T) {
	tests := []struct {
		name         string
		boards       []BoardIdentity
		unidentified []PortRecord
		wantPorts    int
		wantHasJTAG  bool
	}{
		{
			name: "attach USJ by USB serial matching MAC",
			boards: []BoardIdentity{
				{
					MAC:  "30:ed:a0:e4:6a:d0",
					Chip: "ESP32-C5",
					Ports: []PortRecord{
						{
							Path:       "/dev/cu.usbserial-110",
							PortType:   device.PortTypeUART,
							Identified: true,
						},
					},
					HasJTAG: false,
				},
			},
			unidentified: []PortRecord{
				{
					Path:       "/dev/cu.usbmodem1201",
					PortType:   device.PortTypeUSBSerialJTAG,
					USBSerial:  "30:ED:A0:E4:6A:D0",
					Identified: false,
				},
			},
			wantPorts:   2,
			wantHasJTAG: true,
		},
		{
			name: "no match - different MAC",
			boards: []BoardIdentity{
				{
					MAC:  "84:0d:8e:18:8a:d0",
					Chip: "ESP32",
					Ports: []PortRecord{
						{
							Path:       "/dev/ttyUSB0",
							PortType:   device.PortTypeUART,
							Identified: true,
						},
					},
				},
			},
			unidentified: []PortRecord{
				{
					Path:       "/dev/cu.usbmodem1201",
					PortType:   device.PortTypeUSBSerialJTAG,
					USBSerial:  "30:ED:A0:E4:6A:D0",
					Identified: false,
				},
			},
			wantPorts:   1,
			wantHasJTAG: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AttachUnidentifiedUSJ(tt.boards, tt.unidentified)
			if len(tt.boards[0].Ports) != tt.wantPorts {
				t.Errorf("AttachUnidentifiedUSJ() ports count = %d, want %d", len(tt.boards[0].Ports), tt.wantPorts)
			}
			if tt.boards[0].HasJTAG != tt.wantHasJTAG {
				t.Errorf("AttachUnidentifiedUSJ() HasJTAG = %v, want %v", tt.boards[0].HasJTAG, tt.wantHasJTAG)
			}
		})
	}
}

func TestRecommendPort(t *testing.T) {
	tests := []struct {
		name       string
		ports      []PortRecord
		wantFlash  string
		wantReason string
	}{
		{
			name: "prefer USB Serial/JTAG",
			ports: []PortRecord{
				{
					Path:     "/dev/cu.usbserial-110",
					PortType: device.PortTypeUART,
				},
				{
					Path:     "/dev/cu.usbmodem1201",
					PortType: device.PortTypeUSBSerialJTAG,
				},
			},
			wantFlash:  "/dev/cu.usbmodem1201",
			wantReason: "usb_serial_jtag_preferred",
		},
		{
			name: "only UART port",
			ports: []PortRecord{
				{
					Path:       "/dev/ttyUSB0",
					PortType:   device.PortTypeUART,
					Identified: true,
				},
			},
			wantFlash:  "/dev/ttyUSB0",
			wantReason: "only_identified_port",
		},
		{
			name: "first identified port",
			ports: []PortRecord{
				{
					Path:       "/dev/ttyUSB0",
					PortType:   device.PortTypeUART,
					Identified: true,
				},
				{
					Path:     "/dev/ttyUSB1",
					PortType: device.PortTypeUART,
				},
			},
			wantFlash:  "/dev/ttyUSB0",
			wantReason: "first_identified_port",
		},
		{
			name: "fallback first port",
			ports: []PortRecord{
				{
					Path:     "/dev/ttyUSB0",
					PortType: device.PortTypeUART,
				},
			},
			wantFlash:  "/dev/ttyUSB0",
			wantReason: "fallback_first_port",
		},
		{
			name:       "empty ports",
			ports:      []PortRecord{},
			wantFlash:  "",
			wantReason: "no_ports",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecommendPort(tt.ports)
			if got.FlashPort != tt.wantFlash {
				t.Errorf("RecommendPort() FlashPort = %v, want %v", got.FlashPort, tt.wantFlash)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("RecommendPort() Reason = %v, want %v", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		mac  string
		want string
	}{
		{"84:0d:8e:18:8a:d0", "840d8e188ad0"},
		{"84-0d-8e-18-8a-d0", "840d8e188ad0"},
		{"84.0d.8e.18.8a.d0", "840d8e188ad0"},
		{"84:0D:8E:18:8A:D0", "840d8e188ad0"},
		{"840d8e188ad0", "840d8e188ad0"},
	}

	for _, tt := range tests {
		t.Run(tt.mac, func(t *testing.T) {
			got := normalizeMAC(tt.mac)
			if got != tt.want {
				t.Errorf("normalizeMAC() = %v, want %v", got, tt.want)
			}
		})
	}
}
