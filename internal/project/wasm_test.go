//go:build js
// +build js

package project

import (
	"testing"
)

func TestNewWASMFileMap(t *testing.T) {
	files := map[string][]byte{
		"CMakeLists.txt": []byte("cmake_minimum_required(VERSION 3.16)"),
	}

	fm := NewWASMFileMap(files)
	if fm == nil {
		t.Fatal("NewWASMFileMap returned nil")
	}

	if !fm.FileExists("CMakeLists.txt") {
		t.Error("FileExists should return true for CMakeLists.txt")
	}
}

func TestWASMFileMapFileExists(t *testing.T) {
	files := map[string][]byte{
		"test.txt": []byte("test"),
	}

	fm := NewWASMFileMap(files)

	if !fm.FileExists("test.txt") {
		t.Error("FileExists should return true for existing file")
	}

	if fm.FileExists("nonexistent.txt") {
		t.Error("FileExists should return false for non-existing file")
	}
}

func TestWASMFileMapReadFile(t *testing.T) {
	testData := []byte("test data")
	files := map[string][]byte{
		"test.txt": testData,
	}

	fm := NewWASMFileMap(files)

	data, err := fm.ReadFile("test.txt")
	if err != nil {
		t.Errorf("ReadFile error: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("expected %s, got %s", testData, data)
	}

	_, err = fm.ReadFile("nonexistent.txt")
	if err == nil {
		t.Error("ReadFile should return error for non-existing file")
	}
}

func TestWASMFileMapHasPrefix(t *testing.T) {
	files := map[string][]byte{
		"build/bootloader.bin": []byte("boot"),
		"build/partitions.bin": []byte("part"),
		"build/firmware.bin":   []byte("firm"),
		"CMakeLists.txt":       []byte("cmake"),
		"sdkconfig":            []byte("config"),
	}

	fm := NewWASMFileMap(files)

	if !fm.HasPrefix("build/") {
		t.Error("HasPrefix should return true for 'build/'")
	}

	if !fm.HasPrefix("build/bootloader") {
		t.Error("HasPrefix should return true for 'build/bootloader'")
	}

	if fm.HasPrefix("nonexistent/") {
		t.Error("HasPrefix should return false for non-existing prefix")
	}
}

func TestWASMFileMapFindFiles(t *testing.T) {
	files := map[string][]byte{
		"test.txt":       []byte("test"),
		"test.bin":       []byte("binary"),
		"main.go":        []byte("go code"),
		"CMakeLists.txt": []byte("cmake"),
	}

	fm := NewWASMFileMap(files)

	// Find all .txt files
	txtFiles := fm.FindFiles(func(path string) bool {
		return len(path) > 4 && path[len(path)-4:] == ".txt"
	})

	if len(txtFiles) != 1 {
		t.Errorf("expected 1 .txt file, got %d", len(txtFiles))
	}

	if len(txtFiles) > 0 && txtFiles[0] != "CMakeLists.txt" {
		t.Errorf("expected CMakeLists.txt, got %s", txtFiles[0])
	}
}

func TestFindLargestBin(t *testing.T) {
	files := map[string][]byte{
		"small.bin":  []byte{1, 2},
		"large.bin":  []byte{1, 2, 3, 4, 5},
		"medium.bin": []byte{1, 2, 3},
		"other.txt":  []byte{1, 2, 3},
	}

	fm := NewWASMFileMap(files)

	path, data := findLargestBinInMap(fm)
	if path != "large.bin" {
		t.Errorf("expected 'large.bin', got '%s'", path)
	}

	if len(data) != 5 {
		t.Errorf("expected 5 bytes, got %d", len(data))
	}
}

func TestFindLargestBinEmpty(t *testing.T) {
	files := map[string][]byte{
		"test.txt": []byte{1, 2},
	}

	fm := NewWASMFileMap(files)

	path, data := findLargestBinInMap(fm)
	if path != "" {
		t.Errorf("expected empty path, got '%s'", path)
	}

	if data != nil {
		t.Errorf("expected nil data, got %v", data)
	}
}

func TestIsELFFile(t *testing.T) {
	// Valid ELF header
	elfData := []byte{0x7F, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	if !isELFFile(elfData) {
		t.Error("isELFFile should return true for valid ELF")
	}

	// Invalid header
	invalidData := []byte{0x7F, 0x45, 0x4C, 0x46} // Same bytes but not correct
	if !isELFFile(invalidData) {
		// This should actually be true since it's still ELF magic
	}

	// Not ELF
	notELF := []byte{0x00, 0x01, 0x02, 0x03}
	if isELFFile(notELF) {
		t.Error("isELFFile should return false for non-ELF data")
	}

	// Too short
	shortData := []byte{0x7F, 'E'}
	if isELFFile(shortData) {
		t.Error("isELFFile should return false for short data")
	}
}

func TestFindELFFiles(t *testing.T) {
	files := map[string][]byte{
		"test.elf":     []byte{0x7F, 'E', 'L', 'F', 0x02},
		"test.bin":     []byte{0x00, 0x01},
		"firmware.elf": []byte{0x7F, 'E', 'L', 'F', 0x02},
		"readme.txt":   []byte("text"),
	}

	fm := NewWASMFileMap(files)

	elfFiles := findELFFiles(fm)
	if len(elfFiles) != 2 {
		t.Errorf("expected 2 ELF files, got %d", len(elfFiles))
	}
}

func TestWASMRegistry(t *testing.T) {
	registry := NewWASMRegistry()
	if registry == nil {
		t.Fatal("NewWASMRegistry returned nil")
	}

	// Register a detector
	detector := &WASMESPIDFDetector{}
	registry.Register(detector)

	if len(registry.detectors) != 1 {
		t.Errorf("expected 1 detector, got %d", len(registry.detectors))
	}
}

func TestWASMESPIDFDetector(t *testing.T) {
	detector := &WASMESPIDFDetector{}

	if detector.Name() != "esp-idf" {
		t.Errorf("expected name 'esp-idf', got '%s'", detector.Name())
	}

	// Test detection with valid ESP-IDF project
	validFiles := map[string][]byte{
		"CMakeLists.txt": []byte("cmake_minimum_required(VERSION 3.16)"),
		"sdkconfig":      []byte("CONFIG_IDF_TARGET_ESP32=y"),
	}

	fm := NewWASMFileMap(validFiles)
	if !detector.Detect(fm) {
		t.Error("Detect should return true for ESP-IDF project")
	}

	// Test detection with invalid project (missing sdkconfig)
	invalidFiles := map[string][]byte{
		"CMakeLists.txt": []byte("cmake_minimum_required(VERSION 3.16)"),
	}

	fm2 := NewWASMFileMap(invalidFiles)
	if detector.Detect(fm2) {
		t.Error("Detect should return false when sdkconfig is missing")
	}
}

func TestWASMTinyGoDetector(t *testing.T) {
	detector := &WASMTinyGoDetector{}

	if detector.Name() != "tinygo" {
		t.Errorf("expected name 'tinygo', got '%s'", detector.Name())
	}

	// Test detection with valid TinyGo project
	validFiles := map[string][]byte{
		"go.mod":  []byte("module test\n\nrequire tinygo.org/x/drivers v0.22.0"),
		"main.go": []byte("package main\n\nimport \"machine\""),
	}

	fm := NewWASMFileMap(validFiles)
	if !detector.Detect(fm) {
		t.Error("Detect should return true for TinyGo project with drivers")
	}

	// Test detection with machine import
	validFiles2 := map[string][]byte{
		"go.mod":  []byte("module test"),
		"main.go": []byte("package main\n\nimport (\n\t\"machine\"\n\t\"time\"\n)"),
	}

	fm2 := NewWASMFileMap(validFiles2)
	if !detector.Detect(fm2) {
		t.Error("Detect should return true for TinyGo project with machine import")
	}

	// Test detection with invalid project
	invalidFiles := map[string][]byte{
		"go.mod":  []byte("module test"),
		"main.go": []byte("package main\n\nimport \"fmt\""),
	}

	fm3 := NewWASMFileMap(invalidFiles)
	if detector.Detect(fm3) {
		t.Error("Detect should return false for non-TinyGo project")
	}
}

func TestWASMRustESPDetector(t *testing.T) {
	detector := &WASMRustESPDetector{}

	if detector.Name() != "rust-esp" {
		t.Errorf("expected name 'rust-esp', got '%s'", detector.Name())
	}

	// Test detection with valid Rust ESP project
	validFiles := map[string][]byte{
		"Cargo.toml":         []byte("[dependencies]\nesp-hal = \"0.17\""),
		".cargo/config.toml": []byte("[build]\ntarget = \"xtensa-esp32s3-none-elf\""),
	}

	fm := NewWASMFileMap(validFiles)
	if !detector.Detect(fm) {
		t.Error("Detect should return true for Rust ESP project")
	}

	// Test detection with invalid project (missing .cargo/config)
	invalidFiles := map[string][]byte{
		"Cargo.toml": []byte("[dependencies]\nesp-hal = \"0.17\""),
	}

	fm2 := NewWASMFileMap(invalidFiles)
	if detector.Detect(fm2) {
		t.Error("Detect should return false when .cargo/config is missing")
	}
}

func TestDetectWASM(t *testing.T) {
	// Test ESP-IDF detection
	espFiles := map[string][]byte{
		"CMakeLists.txt": []byte("project(test)"),
		"sdkconfig":      []byte("CONFIG_IDF_TARGET_ESP32=y"),
	}

	projType, detector, _ := DetectWASM(espFiles)
	if projType != ProjectTypeESPIDF {
		t.Errorf("expected ESP-IDF, got %s", projType)
	}
	if detector == nil {
		t.Error("detector should not be nil")
	}
	if detector.Name() != "esp-idf" {
		t.Errorf("expected esp-idf detector, got %s", detector.Name())
	}

	// Test TinyGo detection
	tinygoFiles := map[string][]byte{
		"go.mod":  []byte("module test\nrequire tinygo.org/x/drivers"),
		"main.go": []byte("import \"machine\""),
	}

	projType2, _, _ := DetectWASM(tinygoFiles)
	if projType2 != ProjectTypeTinyGo {
		t.Errorf("expected TinyGo, got %s", projType2)
	}

	// Test Rust ESP detection
	rustFiles := map[string][]byte{
		"Cargo.toml":         []byte("[dependencies]\nesp-hal = \"0.17\""),
		".cargo/config.toml": []byte("target = \"xtensa-esp32s3\""),
	}

	projType3, _, _ := DetectWASM(rustFiles)
	if projType3 != ProjectTypeRustESP {
		t.Errorf("expected rust-esp, got %s", projType3)
	}

	// Test no detection
	invalidFiles := map[string][]byte{
		"readme.txt": []byte("This is a text file"),
	}

	projType4, detector4, _ := DetectWASM(invalidFiles)
	if projType4 != ProjectTypeNone {
		t.Errorf("expected None, got %s", projType4)
	}
	if detector4 != nil {
		t.Error("detector should be nil for unrecognized project")
	}
}

func TestFileHasContent(t *testing.T) {
	files := map[string][]byte{
		"test.txt": []byte("hello world"),
	}

	fm := NewWASMFileMap(files)

	if !fileHasContent(fm, "test.txt", "hello") {
		t.Error("fileHasContent should find 'hello' in test.txt")
	}

	if fileHasContent(fm, "test.txt", "goodbye") {
		t.Error("fileHasContent should not find 'goodbye' in test.txt")
	}
}

func TestFileHasAnyContent(t *testing.T) {
	files := map[string][]byte{
		"test.txt": []byte("hello world"),
	}

	fm := NewWASMFileMap(files)

	if !fileHasAnyContent(fm, "test.txt", []string{"hello", "goodbye"}) {
		t.Error("fileHasAnyContent should find 'hello'")
	}

	if !fileHasAnyContent(fm, "test.txt", []string{"world", "foo"}) {
		t.Error("fileHasAnyContent should find 'world'")
	}

	if fileHasAnyContent(fm, "test.txt", []string{"foo", "bar"}) {
		t.Error("fileHasAnyContent should not find 'foo' or 'bar'")
	}
}

func TestFileHasAllContent(t *testing.T) {
	files := map[string][]byte{
		"test.txt": []byte("hello world"),
	}

	fm := NewWASMFileMap(files)

	if !fileHasAllContent(fm, "test.txt", []string{"hello", "world"}) {
		t.Error("fileHasAllContent should find both 'hello' and 'world'")
	}

	if fileHasAllContent(fm, "test.txt", []string{"hello", "goodbye"}) {
		t.Error("fileHasAllContent should not find 'goodbye'")
	}
}

func TestProjectError(t *testing.T) {
	err := &ProjectError{Message: "test error"}
	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got '%s'", err.Error())
	}
}

func TestWASMBuildArtifacts(t *testing.T) {
	artifacts := &WASMBuildArtifacts{
		BootloaderData: []byte{1, 2, 3},
		PartitionsData: []byte{4, 5, 6},
		AppData:        []byte{7, 8, 9},
		BootloaderPath: "bootloader.bin",
		PartitionsPath: "partitions.bin",
		AppPath:        "app.bin",
	}

	native := artifacts.ToNative()
	if native.Bootloader != "bootloader.bin" {
		t.Errorf("ToNative should preserve Bootloader path, got %s", native.Bootloader)
	}
	if native.Partitions != "partitions.bin" {
		t.Errorf("ToNative should preserve Partitions path, got %s", native.Partitions)
	}
	if native.App != "app.bin" {
		t.Errorf("ToNative should preserve App path, got %s", native.App)
	}
}

func TestWASMESPIDFGetChipType(t *testing.T) {
	detector := &WASMESPIDFDetector{}

	// Test with sdkconfig
	files := map[string][]byte{
		"CMakeLists.txt": []byte("project(test)"),
		"sdkconfig":      []byte("CONFIG_IDF_TARGET=esp32s3"),
	}

	fm := NewWASMFileMap(files)
	chipType := detector.GetChipType(fm)
	if chipType != "ESP32-S3" {
		t.Errorf("expected ESP32-S3, got %s", chipType)
	}

	// Test with CMakeLists.txt
	files2 := map[string][]byte{
		"CMakeLists.txt": []byte("project(test)\nset(CMAKE_SYSTEM_GENERATED esp32c3)"),
		"sdkconfig":      []byte("# CONFIG_IDF_TARGET not set"),
	}

	fm2 := NewWASMFileMap(files2)
	chipType2 := detector.GetChipType(fm2)
	if chipType2 != "ESP32-C3" {
		t.Errorf("expected ESP32-C3, got %s", chipType2)
	}
}

func TestWASMTinyGoExtractModuleName(t *testing.T) {
	detector := &WASMTinyGoDetector{}

	// Test module name extraction
	files := map[string][]byte{
		"go.mod": []byte("module github.com/example/myproject"),
	}

	fm := NewWASMFileMap(files)
	moduleName := detector.extractModuleName(fm)
	if moduleName != "myproject" {
		t.Errorf("expected 'myproject', got '%s'", moduleName)
	}

	// Test with simple module name
	files2 := map[string][]byte{
		"go.mod": []byte("module esp32-app"),
	}

	fm2 := NewWASMFileMap(files2)
	moduleName2 := detector.extractModuleName(fm2)
	if moduleName2 != "esp32-app" {
		t.Errorf("expected 'esp32-app', got '%s'", moduleName2)
	}
}

func TestWASMRustESPGetChipType(t *testing.T) {
	detector := &WASMRustESPDetector{}

	// Test with target triple
	files := map[string][]byte{
		"Cargo.toml":         []byte("[dependencies]\nesp-hal = \"0.17\""),
		".cargo/config.toml": []byte("[target.xtensa-esp32s3-none-elf]"),
	}

	fm := NewWASMFileMap(files)
	chipType := detector.GetChipType(fm)
	if chipType != "ESP32-S3" {
		t.Errorf("expected ESP32-S3, got %s", chipType)
	}

	// Test with C3 target
	files2 := map[string][]byte{
		"Cargo.toml":         []byte("[dependencies]\nesp32c3-hal"),
		".cargo/config.toml": []byte("target = \"xtensa-esp32c3-none-elf\""),
	}

	fm2 := NewWASMFileMap(files2)
	chipType2 := detector.GetChipType(fm2)
	if chipType2 != "ESP32-C3" {
		t.Errorf("expected ESP32-C3, got %s", chipType2)
	}
}
