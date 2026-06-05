package device

import (
	"testing"
	"time"
)

func TestUnknownTracker(t *testing.T) {
	tracker := NewUnknownTracker()

	// Test RecordFailure
	tracker.RecordFailure("/dev/ttyUSB0", 0x10c4, 0xea60, "CP2102", "001", "1-1", "timeout")

	port, ok := tracker.Get("/dev/ttyUSB0")
	if !ok {
		t.Fatal("RecordFailure() did not store port")
	}

	if port.Path != "/dev/ttyUSB0" {
		t.Errorf("Path = %v, want /dev/ttyUSB0", port.Path)
	}
	if port.ProbeCount != 1 {
		t.Errorf("ProbeCount = %v, want 1", port.ProbeCount)
	}
	if port.LastError != "timeout" {
		t.Errorf("LastError = %v, want timeout", port.LastError)
	}

	// Test RecordSuccess removes port
	tracker.RecordSuccess("/dev/ttyUSB0")
	_, ok = tracker.Get("/dev/ttyUSB0")
	if ok {
		t.Error("RecordSuccess() did not remove port")
	}

	// Test multiple failures increment count
	tracker.RecordFailure("/dev/ttyUSB1", 0x10c4, 0xea60, "CP2102", "002", "1-2", "busy")
	tracker.RecordFailure("/dev/ttyUSB1", 0x10c4, 0xea60, "CP2102", "002", "1-2", "timeout")

	port, ok = tracker.Get("/dev/ttyUSB1")
	if !ok {
		t.Fatal("second RecordFailure() did not store port")
	}
	if port.ProbeCount != 2 {
		t.Errorf("ProbeCount = %v, want 2", port.ProbeCount)
	}
	if port.LastError != "timeout" {
		t.Errorf("LastError = %v, want timeout (last error)", port.LastError)
	}

	// Test List
	list := tracker.List()
	if len(list) != 1 {
		t.Errorf("List() = %v, want 1 entry", len(list))
	}

	// Test Remove
	tracker.Remove("/dev/ttyUSB1")
	if tracker.Count() != 0 {
		t.Error("Remove() did not remove port")
	}
}

func TestUnknownTrackerCleanup(t *testing.T) {
	tracker := NewUnknownTracker()
	oldTime := time.Now().Add(-2 * time.Hour)

	// Create an old port
	tracker.RecordFailure("/dev/ttyUSB0", 0x10c4, 0xea60, "CP2102", "001", "1-1", "timeout")
	// Manually set the time to old (since RecordFailure uses current time)
	tracker.mu.Lock()
	if p, ok := tracker.ports["/dev/ttyUSB0"]; ok {
		p.LastSeen = oldTime
	}
	tracker.mu.Unlock()

	// Create a recent port
	tracker.RecordFailure("/dev/ttyUSB1", 0x10c4, 0xea60, "CP2102", "002", "1-2", "timeout")

	// Cleanup ports older than 1 hour
	removed := tracker.Cleanup(time.Hour)

	if len(removed) != 1 {
		t.Errorf("Cleanup() removed %d, want 1", len(removed))
	}
	if removed[0] != "/dev/ttyUSB0" {
		t.Errorf("Cleanup() removed %v, want /dev/ttyUSB0", removed[0])
	}

	// Recent port should still exist
	_, ok := tracker.Get("/dev/ttyUSB1")
	if !ok {
		t.Error("Cleanup() removed recent port")
	}

	// Old port should be gone
	_, ok = tracker.Get("/dev/ttyUSB0")
	if ok {
		t.Error("Cleanup() did not remove old port")
	}
}

func TestUnknownTrackerPortTypeDetection(t *testing.T) {
	tracker := NewUnknownTracker()

	// USJ port
	tracker.RecordFailure("/dev/cu.usbmodem1201", ESP_VID, ESP_PID_S3, "USB JTAG/serial", "", "", "timeout")
	port, _ := tracker.Get("/dev/cu.usbmodem1201")
	if port.PortType != PortTypeUSBSerialJTAG {
		t.Errorf("PortType = %v, want %v", port.PortType, PortTypeUSBSerialJTAG)
	}

	// UART port
	tracker.RecordFailure("/dev/ttyUSB0", 0x10c4, 0xea60, "CP2102N", "", "", "busy")
	port, _ = tracker.Get("/dev/ttyUSB0")
	if port.PortType != PortTypeUART {
		t.Errorf("PortType = %v, want %v", port.PortType, PortTypeUART)
	}
}
