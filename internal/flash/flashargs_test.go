package flash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFlashArgs_Valid(t *testing.T) {
	data := []byte(`--flash-mode dio --flash-freq 40m --flash-size 4MB
0x0 bootloader.bin
0x8000 partition-table.bin
0x10000 firmware.bin
`)

	args, err := ParseFlashArgs(data)
	if err != nil {
		t.Fatalf("ParseFlashArgs: %v", err)
	}

	if args.FlashMode != "dio" {
		t.Errorf("FlashMode = %s, want dio", args.FlashMode)
	}
	if args.FlashFreq != "40m" {
		t.Errorf("FlashFreq = %s, want 40m", args.FlashFreq)
	}
	if args.FlashSize != "4MB" {
		t.Errorf("FlashSize = %s, want 4MB", args.FlashSize)
	}
	if len(args.Files) != 3 {
		t.Fatalf("Files count = %d, want 3", len(args.Files))
	}

	if args.Files[0].Offset != 0x0 {
		t.Errorf("Files[0].Offset = 0x%X, want 0x0", args.Files[0].Offset)
	}
	if args.Files[1].Offset != 0x8000 {
		t.Errorf("Files[1].Offset = 0x%X, want 0x8000", args.Files[1].Offset)
	}
	if args.Files[2].Offset != 0x10000 {
		t.Errorf("Files[2].Offset = 0x%X, want 0x10000", args.Files[2].Offset)
	}
}

func TestParseFlashArgs_Empty(t *testing.T) {
	args, err := ParseFlashArgs([]byte{})
	if err != nil {
		t.Fatalf("ParseFlashArgs empty: %v", err)
	}
	if args == nil {
		t.Error("ParseFlashArgs returned nil for empty input")
	}
}

func TestParseFlashArgs_InvalidOffset(t *testing.T) {
	data := []byte(`--flash-mode dio
invalid_offset file.bin
`)

	args, err := ParseFlashArgs(data)
	if err != nil {
		t.Fatalf("ParseFlashArgs: %v", err)
	}
	// Should skip invalid offset lines
	if len(args.Files) != 0 {
		t.Errorf("Files count = %d, want 0 (invalid offset skipped)", len(args.Files))
	}
}

func TestResolveBuildPath(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	buildDir := filepath.Join(tmpDir, "build")
	_ = os.MkdirAll(buildDir, 0755)
	_ = os.MkdirAll(filepath.Join(buildDir, "bootloader"), 0755)

	// Create test files
	mainFile := filepath.Join(buildDir, "main.bin")
	if err := os.WriteFile(mainFile, []byte("main"), 0644); err != nil {
		t.Fatal(err)
	}

	bootFile := filepath.Join(buildDir, "bootloader", "bootloader.bin")
	if err := os.WriteFile(bootFile, []byte("boot"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test main file resolution
	result := ResolveBuildPath(buildDir, "main.bin")
	if result != mainFile {
		t.Errorf("ResolveBuildPath(main.bin) = %s, want %s", result, mainFile)
	}

	// Test bootloader file resolution
	result = ResolveBuildPath(buildDir, "bootloader.bin")
	if result != bootFile {
		t.Errorf("ResolveBuildPath(bootloader.bin) = %s, want %s", result, bootFile)
	}

	// Test non-existent file (should return original name)
	result = ResolveBuildPath(buildDir, "missing.bin")
	if result != "missing.bin" {
		t.Errorf("ResolveBuildPath(missing.bin) = %s, want missing.bin", result)
	}
}
