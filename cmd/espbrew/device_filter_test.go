package main

import (
	"testing"

	"github.com/georgik/espbrew-go/internal/cluster"
)

// filterTestDevices builds a fixed set of devices used across the filter tests.
func filterTestDevices() []cluster.DeviceInfo {
	return []cluster.DeviceInfo{
		{
			DeviceID:   "esp-30:30:F9:5A:8F:D4-if00",
			Path:       "/dev/serial/by-id/box3",
			State:      cluster.DeviceAvailable,
			ChipType:   "ESP32-S3",
			BoardModel: "ESP32-S3-BOX-3",
			Aliases:    []string{"esp32-s3-box-3"},
			Tags:       []string{"bench"},
		},
		{
			DeviceID:   "esp-98:88:E0:D4:D2:58-if00",
			Path:       "/dev/serial/by-id/c3",
			State:      cluster.DeviceAvailable,
			ChipType:   "ESP32-C3",
			BoardModel: "ESP32-C3-LCDKIT",
			Aliases:    []string{"esp32-c3-lcdkit"},
			Tags:       []string{"handheld"},
		},
		{
			DeviceID: "wokwi:esp32-c3",
			Path:     "wokwi:esp32-c3",
			State:    cluster.DeviceAvailable,
			ChipType: "ESP32-C3",
			Aliases:  []string{},
			Tags:     []string{},
		},
		{
			DeviceID: "esp-s3-other",
			Path:     "/dev/serial/by-id/s3-other",
			State:    cluster.DeviceReserved,
			ChipType: "ESP32-S3",
			Aliases:  []string{"reserved-s3"},
			Tags:     []string{"bench"},
		},
	}
}

func TestFilterDevices_NoFilterReturnsAll(t *testing.T) {
	devices := filterTestDevices()
	got, err := filterDevices(devices, "", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(devices) {
		t.Fatalf("expected %d devices, got %d", len(devices), len(got))
	}
}

func TestFilterDevices_ByAlias(t *testing.T) {
	devices := filterTestDevices()
	got, err := filterDevices(devices, "", "", "esp32-c3-lcdkit", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Path != "/dev/serial/by-id/c3" {
		t.Fatalf("expected only c3-lcdkit, got %+v", got)
	}
}

func TestFilterDevices_ByAliasNoMatch(t *testing.T) {
	devices := filterTestDevices()
	_, err := filterDevices(devices, "", "", "does-not-exist", nil)
	if err == nil {
		t.Fatal("expected error for non-matching alias, got nil")
	}
}

func TestFilterDevices_ByChip(t *testing.T) {
	devices := filterTestDevices()
	got, err := filterDevices(devices, "", "ESP32-C3", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// c3-lcdkit + wokwi:esp32-c3 both match the C3 chip.
	if len(got) != 2 {
		t.Fatalf("expected 2 C3 devices, got %d: %+v", len(got), got)
	}
	for _, d := range got {
		if d.ChipType != "ESP32-C3" {
			t.Errorf("device %s has chip %q, want ESP32-C3", d.Path, d.ChipType)
		}
	}
}

func TestFilterDevices_ByBoardModel(t *testing.T) {
	devices := filterTestDevices()
	got, err := filterDevices(devices, "ESP32-S3-BOX-3", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Path != "/dev/serial/by-id/box3" {
		t.Fatalf("expected only box-3, got %+v", got)
	}
}

func TestFilterDevices_ByTag(t *testing.T) {
	devices := filterTestDevices()
	got, err := filterDevices(devices, "", "", "", []string{"bench"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// box-3 + reserved-s3 carry the "bench" tag.
	if len(got) != 2 {
		t.Fatalf("expected 2 bench devices, got %d: %+v", len(got), got)
	}
}

func TestFilterDevices_CombinedCriteria(t *testing.T) {
	devices := filterTestDevices()
	// ESP32-S3 + bench tag: box-3 (available) + reserved-s3.
	got, err := filterDevices(devices, "", "ESP32-S3", "", []string{"bench"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 devices, got %d: %+v", len(got), got)
	}
}
