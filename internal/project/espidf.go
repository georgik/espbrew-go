package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/georgik/espbrew-go/internal/flash"
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

	// Honor the partition table: the build's flash_args lists every image that must
	// be flashed, including extra data partitions that are not part of the standard
	// bootloader/partitions/app trio (for example a pre-populated FAT image emitted
	// by fatfs_create_spiflash_image, e.g. storage.bin). Detect those here so they
	// are flashed instead of silently dropped.
	artifacts.ExtraFiles = d.detectExtraFiles(buildDir, artifacts)

	return artifacts, nil
}

// detectExtraFiles parses the build's flash_args and returns the images that are
// not the standard bootloader/partitions/app slots, preserving their flash offsets.
// This is what makes additional partitions (from a custom partitions.csv) reachable.
func (d *ESPIDFDetector) detectExtraFiles(buildDir string, artifacts *BuildArtifacts) []ExtraFile {
	data, err := os.ReadFile(artifacts.FlashArgs)
	if err != nil {
		// No flash_args -> nothing extra to discover; fall back to the standard slots.
		return nil
	}

	fa, err := flash.ParseFlashArgs(data)
	if err != nil {
		// A malformed flash_args should not break detection; ignore it.
		return nil
	}

	// Collect the resolved absolute paths of the standard slots so we can skip them.
	standard := make(map[string]bool)
	for _, p := range []string{artifacts.Bootloader, artifacts.Partitions, artifacts.App} {
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			standard[abs] = true
		}
	}

	var extra []ExtraFile
	seen := make(map[string]bool)
	for _, f := range fa.Files {
		resolved := resolveBuildPath(buildDir, f.Path)
		if resolved == "" {
			// File referenced by flash_args is missing on disk; skip it.
			continue
		}

		abs, err := filepath.Abs(resolved)
		if err != nil {
			abs = resolved
		}

		// Skip anything that is one of the standard slots, and avoid duplicates.
		if standard[abs] || seen[abs] {
			continue
		}
		seen[abs] = true

		extra = append(extra, ExtraFile{
			Path:   resolved,
			Offset: f.Offset,
			Name:   filepath.Base(resolved),
		})
	}

	return extra
}
