package project

import (
	"fmt"
	"path/filepath"
)

// ESPIDFDetector detects ESP-IDF projects
type ESPIDFDetector struct{}

// Name returns "esp-idf"
func (d *ESPIDFDetector) Name() string {
	return "esp-idf"
}

// Detect checks for ESP-IDF project markers:
// - CMakeLists.txt in root
// - sdkconfig file (or sdkconfig.defaults)
func (d *ESPIDFDetector) Detect(dir string) bool {
	hasCMake := fileExists(filepath.Join(dir, "CMakeLists.txt"))
	hasSDKConfig := fileExists(filepath.Join(dir, "sdkconfig")) ||
		fileExists(filepath.Join(dir, "sdkconfig.defaults"))
	return hasCMake && hasSDKConfig
}

// FindBuildDir locates the ESP-IDF build directory.
// Checks: build/, then looks for build/ subdirectory in common locations.
func (d *ESPIDFDetector) FindBuildDir(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "build"),
		filepath.Join(dir, "..", "build"),
		"build", // Relative to current dir
	}

	for _, candidate := range candidates {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if dirExists(absPath) && isBuildDir(absPath) {
			return absPath, nil
		}
	}

	return "", fmt.Errorf("build directory not found (checked build/)")
}

// GetArtifacts returns paths to ESP-IDF build outputs
func (d *ESPIDFDetector) GetArtifacts(buildDir string) (*BuildArtifacts, error) {
	artifacts := &BuildArtifacts{
		BuildDir:  buildDir,
		FlashArgs: filepath.Join(buildDir, "flash_args"),
	}

	// Standard ESP-IDF build structure
	// build/bootloader/bootloader.bin
	if path := resolveBuildPath(buildDir, "bootloader.bin"); path != "" {
		artifacts.Bootloader = path
	}

	// build/partition_table/partition-table.bin
	if path := resolveBuildPath(buildDir, "partition-table.bin"); path != "" {
		artifacts.Partitions = path
	}

	// build/<project_name>.bin - try common names first
	for _, name := range []string{"firmware.bin", "app.bin"} {
		if path := resolveBuildPath(buildDir, name); path != "" {
			artifacts.App = path
			break
		}
	}

	// If no app found, list .bin files in build dir and pick the largest
	if artifacts.App == "" {
		if path := findLargestBin(buildDir); path != "" {
			artifacts.App = path
		}
	}

	return artifacts, nil
}
