package project

import (
	"os"
	"path/filepath"
	"sort"
)

// fileExists checks if a file exists and is not a directory
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isBuildDir checks if a directory is a build directory
// by looking for build system files (build.ninja, Makefile, etc.)
func isBuildDir(path string) bool {
	buildMarkers := []string{"build.ninja", "Makefile", "build.ninja.txt"}
	for _, marker := range buildMarkers {
		if fileExists(filepath.Join(path, marker)) {
			return true
		}
	}
	return false
}

// resolveBuildPath finds a file in ESP-IDF build directories
// Checks both root build dir and bootloader subdirectory
func resolveBuildPath(buildDir, filename string) string {
	candidates := []string{
		filepath.Join(buildDir, filename),
		filepath.Join(buildDir, "bootloader", filename),
		filepath.Join(buildDir, "partition_table", filename),
		filename,
	}

	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// findLargestBin finds the largest .bin file in a directory
// Used as fallback to find the app binary
func findLargestBin(buildDir string) string {
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return ""
	}

	var binFiles []struct {
		name string
		size int64
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".bin" {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			binFiles = append(binFiles, struct {
				name string
				size int64
			}{entry.Name(), info.Size()})
		}
	}

	if len(binFiles) == 0 {
		return ""
	}

	// Sort by size descending
	sort.Slice(binFiles, func(i, j int) bool {
		return binFiles[i].size > binFiles[j].size
	})

	return filepath.Join(buildDir, binFiles[0].name)
}
