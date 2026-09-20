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

	// Honor the partition table: the build's flash_args is the authoritative flash
	// plan. It lists every image that must be flashed, together with the exact flash
	// offset ESP-IDF assigned to it (bootloader / partitions / app / any extra data
	// partitions such as a pre-populated FAT image emitted by
	// fatfs_create_spiflash_image, e.g. storage.bin). Using these offsets — rather
	// than preset ones — is what makes a custom partition table work: each image lands
	// at its real partition instead of colliding or leaving the factory slot empty.
	artifacts.FlashFiles = d.parseFlashFiles(buildDir, artifacts)

	// The "extra" partitions are simply the flash_files entries that are not one of
	// the standard bootloader/partitions/app slots. Keep them surfaced separately so
	// callers that only care about the trio can still see what else was flashed.
	artifacts.ExtraFiles = d.extraFilesFrom(buildDir,
		[]string{artifacts.Bootloader, artifacts.Partitions, artifacts.App},
		artifacts.FlashFiles)

	return artifacts, nil
}

// parseFlashFiles reads the build's flash_args and returns the authoritative,
// ordered list of every image to flash with its real flash offset. Missing files
// are skipped, duplicates are collapsed, and a missing/malformed flash_args yields
// an empty slice (callers fall back to preset offsets in that case). It never
// returns an error so a broken flash_args can never block detection.
func (d *ESPIDFDetector) parseFlashFiles(buildDir string, artifacts *BuildArtifacts) []FlashFile {
	data, err := os.ReadFile(artifacts.FlashArgs)
	if err != nil {
		return nil
	}

	fa, err := flash.ParseFlashArgs(data)
	if err != nil {
		// A malformed flash_args should not break detection; ignore it.
		return nil
	}

	files := make([]FlashFile, 0, len(fa.Files))
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
		if seen[abs] {
			// Avoid flashing the same image twice.
			continue
		}
		seen[abs] = true

		files = append(files, FlashFile{
			Path:   resolved,
			Offset: f.Offset,
			Name:   filepath.Base(resolved),
		})
	}

	return files
}

// extraFilesFrom returns the flash files that are not one of the standard
// bootloader/partitions/app slots.
func (d *ESPIDFDetector) extraFilesFrom(buildDir string, standard []string, files []FlashFile) []ExtraFile {
	standardSet := make(map[string]bool)
	for _, p := range standard {
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			standardSet[abs] = true
		}
	}

	extra := make([]ExtraFile, 0)
	for _, f := range files {
		if abs, err := filepath.Abs(f.Path); err == nil {
			if standardSet[abs] {
				continue
			}
		}
		extra = append(extra, ExtraFile{
			Path:   f.Path,
			Offset: f.Offset,
			Name:   f.Name,
		})
	}

	return extra
}
