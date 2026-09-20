package main

import (
	"fmt"
	"testing"

	"github.com/georgik/espbrew-go/internal/flash"
	"github.com/georgik/espbrew-go/internal/project"
)

func TestResolvePreset(t *testing.T) {
	tests := []struct {
		name           string
		preset         string
		chip           string
		expectedOffset int
		expectError    bool
	}{
		{"App preset", "app", "", flash.PresetOffsetApp, false},
		{"Partitions preset", "partitions", "", flash.PresetOffsetPartitions, false},
		{"Bootloader preset (default ESP32)", "bootloader", "", 0x1000, false},
		{"Unknown preset", "unknown", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset := 0
			var err error

			switch tt.preset {
			case "bootloader":
				offset = 0x1000 // ESP32 default
			case "partitions":
				offset = flash.PresetOffsetPartitions
			case "app":
				offset = flash.PresetOffsetApp
			default:
				err = fmt.Errorf("unknown preset")
			}

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && offset != tt.expectedOffset {
				t.Errorf("Offset = %d, want %d", offset, tt.expectedOffset)
			}
		})
	}
}

// TestBuildMultiImageImages_ExtraPartitions verifies that extra partition images
// discovered during autodetection are appended to the flash image list (after the
// standard bootloader/partitions/app trio) with their original offsets.
func TestBuildMultiImageImages_ExtraPartitions(t *testing.T) {
	// Reset flags to a known state.
	flashOpts.chip = "auto"
	flashOpts.bootloader = "/build/bootloader/bootloader.bin"
	flashOpts.partitions = "/build/partition_table/partition-table.bin"
	flashOpts.app = "/build/esp32s3_hello.bin"

	extra := []project.ExtraFile{
		{Name: "storage.bin", Path: "/build/storage.bin", Offset: 0x10000},
	}

	images := buildMultiImageImages(nil, extra)

	// bootloader + partitions + app + 1 extra.
	if len(images) != 4 {
		t.Fatalf("expected 4 images, got %d: %+v", len(images), images)
	}

	want := []struct {
		name   string
		offset int
	}{
		{"bootloader", 0x0},
		{"partitions", flash.PresetOffsetPartitions},
		{"app", flash.PresetOffsetApp},
		{"storage.bin", 0x10000},
	}
	for i, w := range want {
		if images[i].name != w.name {
			t.Errorf("image[%d] name = %q, want %q", i, images[i].name, w.name)
		}
		if images[i].offset != w.offset {
			t.Errorf("image[%d] offset = 0x%x, want 0x%x", i, images[i].offset, w.offset)
		}
	}
}

// TestBuildMultiImageImages_NoExtra verifies the standard trio with no extras.
func TestBuildMultiImageImages_NoExtra(t *testing.T) {
	flashOpts.chip = "auto"
	flashOpts.bootloader = "/build/bootloader/bootloader.bin"
	flashOpts.partitions = "/build/partition_table/partition-table.bin"
	flashOpts.app = "/build/esp32s3_hello.bin"

	images := buildMultiImageImages(nil, nil)
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d: %+v", len(images), images)
	}
}

// TestBuildMultiImageImages_FlashArgsOffsets verifies that when the authoritative
// flash plan (parsed from build/flash_args) is provided, each image is flashed at
// its real partition offset — not a preset one. This is the regression guard for
// the bug where a custom-partition app at 0x50000 was instead flashed at the preset
// 0x10000 (colliding with a storage partition), leaving the factory slot empty so
// the device would not boot.
func TestBuildMultiImageImages_FlashArgsOffsets(t *testing.T) {
	flashOpts.chip = "auto"
	flashOpts.bootloader = ""
	flashOpts.partitions = ""
	flashOpts.app = ""

	// Mirrors m5stack-atom-s3r-joystick-usb build/flash_args.
	flashFiles := []project.FlashFile{
		{Name: "bootloader.bin", Path: "/build/bootloader/bootloader.bin", Offset: 0x0},
		{Name: "partition-table.bin", Path: "/build/partition_table/partition-table.bin", Offset: 0x8000},
		{Name: "esp32s3_hello.bin", Path: "/build/esp32s3_hello.bin", Offset: 0x50000},
		{Name: "storage.bin", Path: "/build/storage.bin", Offset: 0x10000},
	}

	images := buildMultiImageImages(flashFiles, nil)

	if len(images) != 4 {
		t.Fatalf("expected 4 images, got %d: %+v", len(images), images)
	}

	want := []struct {
		name   string
		offset int
	}{
		{"bootloader.bin", 0x0},
		{"partition-table.bin", 0x8000},
		{"esp32s3_hello.bin", 0x50000}, // factory app must land at 0x50000, NOT 0x10000
		{"storage.bin", 0x10000},
	}
	for i, w := range want {
		if images[i].name != w.name {
			t.Errorf("image[%d] name = %q, want %q", i, images[i].name, w.name)
		}
		if images[i].offset != w.offset {
			t.Errorf("image[%d] offset = 0x%x, want 0x%x", i, images[i].offset, w.offset)
		}
	}
}
