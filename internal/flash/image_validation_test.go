package flash

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/chips"
	"codeberg.org/georgik/espbrew-go/internal/flash/testutil"
)

// TestImageFormat validates the complete image format structure
func TestImageFormat(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	result, err := ConvertELFToESPImage(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatalf("ConvertELFToESPImage failed: %v", err)
	}

	parts, err := ParseMultiPartImage(result)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	// Find bootloader, partition table, and app
	var bootloader, partitionTable, app []byte
	var bootloaderOffset, partitionOffset, appOffset uint32

	for _, part := range parts {
		var name string
		switch part.Offset {
		case 0x0, 0x1000:
			name = "bootloader"
		case 0x8000:
			name = "partition_table"
		case 0x10000:
			name = "app"
		default:
			name = "unknown"
		}
		t.Logf("Part: %s offset=0x%x size=%d", name, part.Offset, len(part.Data))
		switch part.Offset {
		case 0x0, 0x1000:
			bootloader = part.Data
			bootloaderOffset = part.Offset
		case 0x8000:
			partitionTable = part.Data
			partitionOffset = part.Offset
		case 0x10000:
			app = part.Data
			appOffset = part.Offset
		}
	}

	// Validate bootloader
	if bootloader == nil {
		t.Fatal("Bootloader not found")
	}
	t.Logf("Bootloader: offset=0x%x size=%d magic=%x", bootloaderOffset, len(bootloader), bootloader[0])

	// Validate partition table
	if partitionTable == nil {
		t.Fatal("Partition table not found")
	}
	if len(partitionTable) < 32 {
		t.Fatalf("Partition table too small: %d bytes", len(partitionTable))
	}
	// Check partition table magic (0x50AA)
	magic := binary.LittleEndian.Uint16(partitionTable[0:2])
	if magic != 0x50AA {
		t.Errorf("Partition table magic = 0x%04x, want 0x50AA", magic)
	}
	t.Logf("Partition table: offset=0x%x size=%d entries=%d", partitionOffset, len(partitionTable), len(partitionTable)/32)

	// Validate app image
	if app == nil {
		t.Fatal("App image not found")
	}
	if len(app) < 24 {
		t.Fatalf("App image too small: %d bytes", len(app))
	}

	// Extended header validation
	appMagic := app[0]
	if appMagic != 0xE9 {
		t.Errorf("App magic = 0x%02x, want 0xE9", appMagic)
	}

	segmentCount := app[1]
	t.Logf("App: offset=0x%x size=%d segments=%d", appOffset, len(app), segmentCount)

	flashMode := app[2]
	flashSizeFreq := app[3]
	t.Logf("Flash mode: 0x%02x, Flash size/freq: 0x%02x", flashMode, flashSizeFreq)

	// Expected: 0x40 = (0x04 << 4) | 0x00 = 16MB @ 40MHz
	if flashSizeFreq != 0x40 {
		t.Errorf("Flash size/freq = 0x%02x, want 0x40 (16MB @ 40MHz)", flashSizeFreq)
	}

	// Entry point
	entry := binary.LittleEndian.Uint32(app[4:8])
	t.Logf("Entry point: 0x%08x", entry)

	// WP Pin
	wpPin := app[8]
	if wpPin != 0xEE {
		t.Errorf("WP pin = 0x%02x, want 0xEE (disabled)", wpPin)
	}

	// Chip ID
	chipID := binary.LittleEndian.Uint16(app[12:14])
	t.Logf("Chip ID: 0x%04x", chipID)

	// Append digest (byte 23 in header)
	appendDigest := app[23]
	if appendDigest != 1 {
		t.Errorf("Append digest = %d, want 1", appendDigest)
	}

	// Find segments
	offset := 24
	segCount := 0
	for offset < len(app)-8 && segCount < 10 {
		segAddr := binary.LittleEndian.Uint32(app[offset : offset+4])
		segLen := binary.LittleEndian.Uint32(app[offset+4 : offset+8])
		if segLen == 0 || segLen > 0x100000 {
			break
		}
		t.Logf("Segment %d: addr=0x%08x len=%d", segCount, segAddr, segLen)
		offset += 8
		if offset+int(segLen) > len(app) {
			t.Errorf("Segment %d extends beyond app image", segCount)
			break
		}
		offset += int(segLen)
		segCount++
	}

	t.Logf("Total segments found: %d", segCount)
}

// TestImageHeaderBytes validates critical header bytes match expected values
func TestImageHeaderBytes(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	result, err := ConvertELFToESPImage(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatalf("ConvertELFToESPImage failed: %v", err)
	}

	parts, err := ParseMultiPartImage(result)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	var appData []byte
	for _, part := range parts {
		if part.Offset == 0x10000 {
			appData = part.Data
			break
		}
	}

	if appData == nil {
		t.Fatal("App part not found")
	}

	// Expected first 32 bytes of app image
	// e9 05 02 40 8c 9c 37 40 ee 00 00 00 09 00 00 00 00 63 00 00 00 00 00 01 20
	// ExtendedImageHeader structure:
	// 0: Magic
	// 1: SegmentCount
	// 2: FlashMode
	// 3: FlashSizeFreq
	// 4-7: Entry
	// 8: WPPin
	// 9: ClkQDrv
	// 10: D_CSDrv
	// 11: GD_WPDrv
	// 12-13: ChipID
	// 14: MinRev
	// 15-16: MinChipRevFull
	// 17-18: MaxChipRevFull
	// 19-22: Reserved
	// 23: AppendDigest
	expected := []byte{
		0xE9,                   // 0: Magic
		0x05,                   // 1: Segment count
		0x02,                   // 2: Flash mode (DIO)
		0x40,                   // 3: Flash size/freq (16MB @ 40MHz)
		0x8C, 0x9C, 0x37, 0x40, // 4-7: Entry point
		0xEE,       // 8: WP pin
		0x00,       // 9: ClkQDrv
		0x00,       // 10: D_CSDrv
		0x00,       // 11: GD_WPDrv
		0x09, 0x00, // 12-13: Chip ID
		0x00,       // 14: MinRev
		0x00, 0x00, // 15-16: MinChipRevFull
		0x63, 0x00, // 17-18: MaxChipRevFull (0x0063)
		0x00, 0x00, 0x00, 0x00, // 19-22: Reserved
		0x01, // 23: Append digest
	}

	// Compare first 24 bytes (extended header)
	if len(appData) < 24 {
		t.Fatalf("App data too short: %d bytes", len(appData))
	}

	mismatches := 0
	for i := 0; i < 24; i++ {
		if appData[i] != expected[i] {
			t.Errorf("Byte %d: got 0x%02x, want 0x%02x", i, appData[i], expected[i])
			mismatches++
		}
	}

	if mismatches == 0 {
		t.Log("All header bytes match expected values")
	} else {
		t.Errorf("Found %d header byte mismatches", mismatches)
		t.Logf("Got:      % x", appData[:24])
		t.Logf("Expected: % x", expected)
	}
}

// TestVirtualDeviceWrite validates that virtual device write preserves data
func TestVirtualDeviceWrite(t *testing.T) {
	elfPath := testutil.TestELFPath()
	data, err := os.ReadFile(elfPath)
	if err != nil {
		t.Skip("Rust ESP binary not found")
	}

	result, err := ConvertELFToESPImage(data, chips.ChipESP32S3)
	if err != nil {
		t.Fatalf("ConvertELFToESPImage failed: %v", err)
	}

	parts, err := ParseMultiPartImage(result)
	if err != nil {
		t.Fatalf("ParseMultiPartImage failed: %v", err)
	}

	// Find app part
	var appPart *ImagePart
	for i := range parts {
		if parts[i].Offset == 0x10000 {
			appPart = &parts[i]
			break
		}
	}

	if appPart == nil {
		t.Fatal("App part not found")
	}

	// Verify app part header
	if len(appPart.Data) < 4 {
		t.Fatalf("App part too short: %d bytes", len(appPart.Data))
	}

	// Check critical bytes
	if appPart.Data[0] != 0xE9 {
		t.Errorf("Magic = 0x%02x, want 0xE9", appPart.Data[0])
	}
	if appPart.Data[3] != 0x40 {
		t.Errorf("Flash size/freq = 0x%02x, want 0x40", appPart.Data[3])
	}

	t.Logf("App part: offset=0x%x size=%d header=% x", appPart.Offset, len(appPart.Data), appPart.Data[:24])

	// Simulate virtual device write: copy data to a buffer
	virtualMem := make([]byte, 16*1024*1024)
	for i := range virtualMem {
		virtualMem[i] = 0xFF
	}

	// Write each part
	for _, part := range parts {
		copy(virtualMem[part.Offset:part.Offset+uint32(len(part.Data))], part.Data)
	}

	// Read back app image
	appReadback := virtualMem[0x10000 : 0x10000+uint32(len(appPart.Data))]

	// Verify write preserved data
	if !bytes.Equal(appReadback, appPart.Data) {
		t.Error("Virtual device write did not preserve app data")

		// Find first difference
		for i := 0; i < len(appPart.Data) && i < len(appReadback); i++ {
			if appReadback[i] != appPart.Data[i] {
				t.Logf("First difference at offset %d: got 0x%02x, want 0x%02x", i, appReadback[i], appPart.Data[i])
				break
			}
		}
	} else {
		t.Log("Virtual device write preserved all data correctly")
	}

	// Verify app header in virtual memory
	if appReadback[0] != 0xE9 {
		t.Errorf("Virtual mem magic = 0x%02x, want 0xE9", appReadback[0])
	}
	if appReadback[3] != 0x40 {
		t.Errorf("Virtual mem flash size/freq = 0x%02x, want 0x40", appReadback[3])
	}
}
