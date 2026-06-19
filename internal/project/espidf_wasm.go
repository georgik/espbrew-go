//go:build js
// +build js

package project

import (
	"strings"
)

// WASMESPIDFDetector detects ESP-IDF projects in WASM environment
type WASMESPIDFDetector struct{}

// Name returns "esp-idf"
func (d *WASMESPIDFDetector) Name() string {
	return "esp-idf"
}

// Detect checks for ESP-IDF project markers:
// - CMakeLists.txt in root
// - sdkconfig file (or sdkconfig.defaults)
func (d *WASMESPIDFDetector) Detect(files *WASMFileMap) bool {
	// Check for CMakeLists.txt at root
	hasCMake := files.FileExists("CMakeLists.txt")

	// Check for sdkconfig
	hasSDKConfig := files.FileExists("sdkconfig") ||
		files.FileExists("sdkconfig.defaults")

	return hasCMake && hasSDKConfig
}

// GetArtifacts returns paths to ESP-IDF build outputs
// ESP-IDF projects have build output structure:
// - build/bootloader/bootloader.bin
// - build/partition_table/partition-table.bin
// - build/<project>.bin or build/app.bin
func (d *WASMESPIDFDetector) GetArtifacts(files *WASMFileMap) (*BuildArtifacts, error) {
	wasmArtifacts, err := d.GetArtifactsWithStructure(files)
	if err != nil {
		return nil, err
	}
	return wasmArtifacts.ToNative(), nil
}

// GetArtifactsWithStructure attempts to determine build directory structure
// and returns artifacts with proper structure information
func (d *WASMESPIDFDetector) GetArtifactsWithStructure(files *WASMFileMap) (*WASMBuildArtifacts, error) {
	wasmArtifacts := &WASMBuildArtifacts{}

	// Determine if we're in build directory or project root
	inBuildDir := files.FileExists("bootloader.bin") || files.FileExists("bootloader/bootloader.bin")
	inProjectRoot := files.FileExists("CMakeLists.txt")

	// Common paths to try
	bootloaderCandidates := d.getBootloaderCandidates(inBuildDir, inProjectRoot)
	for _, path := range bootloaderCandidates {
		if data, err := files.ReadFile(path); err == nil && len(data) > 0 {
			wasmArtifacts.BootloaderData = data
			wasmArtifacts.BootloaderPath = path
			break
		}
	}

	partitionCandidates := d.getPartitionCandidates(inBuildDir, inProjectRoot)
	for _, path := range partitionCandidates {
		if data, err := files.ReadFile(path); err == nil && len(data) > 0 {
			wasmArtifacts.PartitionsData = data
			wasmArtifacts.PartitionsPath = path
			break
		}
	}

	appCandidates := d.getAppCandidates(inBuildDir, inProjectRoot)
	for _, path := range appCandidates {
		if data, err := files.ReadFile(path); err == nil && len(data) > 0 {
			wasmArtifacts.AppData = data
			wasmArtifacts.AppPath = path
			break
		}
	}

	// Fallback: find largest .bin that's not bootloader/partitions
	if wasmArtifacts.AppData == nil {
		for path, data := range files.files {
			if strings.HasSuffix(path, ".bin") &&
				!strings.Contains(path, "bootloader") &&
				!strings.Contains(path, "partition") &&
				len(data) > 10000 { // Minimum reasonable size
				if wasmArtifacts.AppData == nil || len(data) > len(wasmArtifacts.AppData) {
					wasmArtifacts.AppData = data
					wasmArtifacts.AppPath = path
				}
			}
		}
	}

	return wasmArtifacts, nil
}

// getBootloaderCandidates returns possible bootloader.bin paths
func (d *WASMESPIDFDetector) getBootloaderCandidates(inBuildDir, inProjectRoot bool) []string {
	var candidates []string

	if inBuildDir {
		candidates = append(candidates,
			"bootloader/bootloader.bin",
			"bootloader.bin",
		)
	}

	if inProjectRoot {
		candidates = append(candidates,
			"build/bootloader/bootloader.bin",
			"build/bootloader.bin",
		)
	}

	// Try flat paths too
	candidates = append(candidates, "bootloader.bin")

	return candidates
}

// getPartitionCandidates returns possible partition-table.bin paths
func (d *WASMESPIDFDetector) getPartitionCandidates(inBuildDir, inProjectRoot bool) []string {
	var candidates []string

	if inBuildDir {
		candidates = append(candidates,
			"partition_table/partition-table.bin",
			"partition-table.bin",
			"partitions.bin",
		)
	}

	if inProjectRoot {
		candidates = append(candidates,
			"build/partition_table/partition-table.bin",
			"build/partition-table.bin",
			"build/partitions.bin",
		)
	}

	// Try flat paths
	candidates = append(candidates, "partition-table.bin", "partitions.bin")

	return candidates
}

// getAppCandidates returns possible app/firmware.bin paths
func (d *WASMESPIDFDetector) getAppCandidates(inBuildDir, inProjectRoot bool) []string {
	var candidates []string

	// Standard names
	names := []string{"firmware.bin", "app.bin"}

	if inBuildDir {
		for _, name := range names {
			candidates = append(candidates, name)
		}
	}

	if inProjectRoot {
		for _, name := range names {
			candidates = append(candidates, "build/"+name)
		}
	}

	// Try with project name (would need to extract from CMakeLists.txt)
	candidates = append(candidates,
		"build/myapp.bin",
		"myapp.bin",
	)

	return candidates
}

// FindBuildDir attempts to determine the build directory from file structure
func (d *WASMESPIDFDetector) FindBuildDir(files *WASMFileMap) (string, error) {
	// Check if we're already in build directory
	if files.FileExists("bootloader.bin") || files.FileExists("bootloader/bootloader.bin") {
		return "build", nil
	}

	// Check if build directory exists
	if files.FileExists("build/bootloader.bin") || files.FileExists("build/bootloader/bootloader.bin") {
		return "build", nil
	}

	// Can't determine build directory from flat file list
	// Assume current directory is project root
	return "", nil
}

// ExtractProjectName attempts to extract project name from CMakeLists.txt
func (d *WASMESPIDFDetector) ExtractProjectName(files *WASMFileMap) string {
	data, err := files.ReadFileString("CMakeLists.txt")
	if err != nil {
		return ""
	}

	// Look for project() call
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "project(") {
			// Extract name between project( and )
			start := strings.Index(line, "(")
			if start == -1 {
				continue
			}
			line = line[start+1:]
			end := strings.Index(line, ")")
			if end == -1 {
				continue
			}
			name := strings.TrimSpace(line[:end])
			// Remove quotes if present
			name = strings.Trim(name, "\"")
			return name
		}
	}

	return ""
}

// HasFlashArgs checks if flash_args file exists
func (d *WASMESPIDFDetector) HasFlashArgs(files *WASMFileMap) bool {
	return files.FileExists("flash_args") ||
		files.FileExists("build/flash_args")
}

// GetChipType attempts to determine chip type from sdkconfig or CMakeLists.txt
func (d *WASMESPIDFDetector) GetChipType(files *WASMFileMap) string {
	// Try sdkconfig first
	configData, err := files.ReadFileString("sdkconfig")
	if err == nil {
		// Look for CONFIG_IDF_TARGET
		lines := strings.Split(configData, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "CONFIG_IDF_TARGET=") {
				// Extract value
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					target := strings.Trim(strings.TrimSpace(parts[1]), "\"")
					return "ESP32-" + strings.ToUpper(target)
				}
			}
		}
	}

	// Try to parse from CMakeLists.txt
	cmakeData, _ := files.ReadFileString("CMakeLists.txt")
	if strings.Contains(cmakeData, "esp32s3") {
		return "ESP32-S3"
	}
	if strings.Contains(cmakeData, "esp32c3") {
		return "ESP32-C3"
	}
	if strings.Contains(cmakeData, "esp32s2") {
		return "ESP32-S2"
	}

	// Default
	return "ESP32"
}

// ValidateArtifacts checks if required artifacts are present
func (d *WASMESPIDFDetector) ValidateArtifacts(artifacts *BuildArtifacts) error {
	if artifacts.App == "" {
		return &ProjectError{Message: "app/firmware binary path not found"}
	}
	return nil
}

// ValidateArtifactsWASM checks WASM artifacts
func (d *WASMESPIDFDetector) ValidateArtifactsWASM(artifacts *WASMBuildArtifacts) error {
	if artifacts.AppData == nil || len(artifacts.AppData) == 0 {
		return &ProjectError{Message: "app/firmware binary not found"}
	}
	return nil
}
