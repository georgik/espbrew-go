//go:build js
// +build js

package project

import (
	"strings"
)

// WASMTinyGoDetector detects TinyGo ESP32 projects in WASM environment
type WASMTinyGoDetector struct{}

// Name returns "tinygo"
func (d *WASMTinyGoDetector) Name() string {
	return "tinygo"
}

// Detect checks for TinyGo ESP project markers:
// - go.mod with tinygo.org/x dependencies
// - OR source files importing "machine" package (TinyGo hardware abstraction)
func (d *WASMTinyGoDetector) Detect(files *WASMFileMap) bool {
	// Check for go.mod
	if !files.FileExists("go.mod") {
		return false
	}

	// Check for TinyGo dependencies in go.mod
	hasTinyGoDeps := d.hasTinyGoDependencies(files)
	if hasTinyGoDeps {
		return true
	}

	// Fallback: check for machine package import in source files
	return d.hasMachineImport(files)
}

// hasTinyGoDependencies checks if go.mod contains TinyGo-related dependencies
func (d *WASMTinyGoDetector) hasTinyGoDependencies(files *WASMFileMap) bool {
	data, err := files.ReadFileString("go.mod")
	if err != nil {
		return false
	}

	tinygoDeps := []string{
		"tinygo.org/x/drivers",
		"tinygo.org/x/tinygl-font",
		"tinygo.org/x/adapters",
		"tinygo.org/x/console",
		"tinygo.org/x/adc",
		"tinygo.org/x/rand",
	}

	for _, dep := range tinygoDeps {
		if strings.Contains(data, dep) {
			return true
		}
	}

	return false
}

// hasMachineImport checks if any .go file imports "machine" package
func (d *WASMTinyGoDetector) hasMachineImport(files *WASMFileMap) bool {
	// Search for .go files (excluding test files)
	for path := range files.files {
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			if d.checkFileForMachineImport(files, path) {
				return true
			}
		}
	}
	return false
}

// checkFileForMachineImport checks a single file for machine import
func (d *WASMTinyGoDetector) checkFileForMachineImport(files *WASMFileMap, path string) bool {
	data, err := files.ReadFileString(path)
	if err != nil {
		return false
	}

	// Simple string matching for machine import in various forms:
	// import "machine"
	// import `machine`
	// import machine " ..."
	// import . "machine"
	// import m "machine"
	// import (
	//     "machine"
	//     m "machine"
	//     . "machine"
	// )

	patterns := []string{
		"\"machine\"",
		"`machine`",
		"machine\",", // for cases like m "machine"
		"machine\"",  // for cases like machine "..."
	}

	for _, pattern := range patterns {
		if strings.Contains(data, pattern) {
			// Verify it's actually an import statement by checking nearby context
			idx := strings.Index(data, pattern)
			if idx > 0 {
				// Check if "import" appears before this pattern
				beforeImport := data[max(idx-100, 0):idx]
				if strings.Contains(beforeImport, "import") {
					return true
				}
			}
		}
	}

	return false
}

// GetArtifacts returns paths to TinyGo build outputs
// TinyGo produces ELF file (or .bin if converted) named after module
func (d *WASMTinyGoDetector) GetArtifacts(files *WASMFileMap) (*BuildArtifacts, error) {
	wasmArtifacts, err := d.GetArtifactsWithStructure(files)
	if err != nil {
		return nil, err
	}
	return wasmArtifacts.ToNative(), nil
}

// GetArtifactsWithStructure returns WASM build artifacts with structure info
func (d *WASMTinyGoDetector) GetArtifactsWithStructure(files *WASMFileMap) (*WASMBuildArtifacts, error) {
	artifacts := &WASMBuildArtifacts{}

	// Try to extract module name from go.mod
	moduleName := d.extractModuleName(files)
	if moduleName != "" {
		// Look for ELF file matching module name
		elfPaths := []string{
			moduleName,
			moduleName + ".elf",
			"/" + moduleName,
		}

		for _, path := range elfPaths {
			if data, err := files.ReadFile(path); err == nil && isELFFile(data) {
				artifacts.AppData = data
				artifacts.AppPath = path
				return artifacts, nil
			}
		}
	}

	// Fallback: find any ELF file in the map
	elfFiles := findELFFiles(files)
	if len(elfFiles) > 0 {
		// Prefer the one matching module name or the largest one
		for _, path := range elfFiles {
			if data, ok := files.files[path]; ok {
				if artifacts.AppData == nil || len(data) > len(artifacts.AppData) {
					artifacts.AppData = data
					artifacts.AppPath = path
				}
			}
		}
	}

	// Also check for .bin files (TinyGo can output .bin directly)
	if artifacts.AppData == nil {
		binPaths := []string{
			moduleName + ".bin",
			"firmware.bin",
			"app.bin",
		}

		for _, path := range binPaths {
			if data, err := files.ReadFile(path); err == nil && len(data) > 0 {
				artifacts.AppData = data
				artifacts.AppPath = path
				return artifacts, nil
			}
		}
	}

	return artifacts, nil
}

// extractModuleName reads the module name from go.mod
func (d *WASMTinyGoDetector) extractModuleName(files *WASMFileMap) string {
	data, err := files.ReadFileString("go.mod")
	if err != nil {
		return ""
	}

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Return last path component of module name
				modulePath := parts[1]
				slashes := strings.Split(modulePath, "/")
				return slashes[len(slashes)-1]
			}
		}
	}

	return ""
}

// GetChipType attempts to determine chip type from go.mod or imports
func (d *WASMTinyGoDetector) GetChipType(files *WASMFileMap) string {
	// Check for chip-specific imports in .go files
	for path, data := range files.files {
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			strData := string(data)

			// Check for machine package imports that indicate chip type
			if strings.Contains(strData, "machine/esp32") {
				if strings.Contains(strData, "ESP32-S3") {
					return "ESP32-S3"
				}
				if strings.Contains(strData, "ESP32-C3") {
					return "ESP32-C3"
				}
				return "ESP32"
			}

			// Check for ESP32-S3 specific imports
			if strings.Contains(strData, "esp32-s3") || strings.Contains(strData, "ESP32S3") {
				return "ESP32-S3"
			}

			// Check for ESP32-C3 specific imports
			if strings.Contains(strData, "esp32-c3") || strings.Contains(strData, "ESP32C3") {
				return "ESP32-C3"
			}

			// Check for ESP32-C6 specific imports
			if strings.Contains(strData, "esp32-c6") || strings.Contains(strData, "ESP32C6") {
				return "ESP32-C6"
			}
		}
	}

	// Fallback: check go.mod for target tags
	if data, err := files.ReadFileString("go.mod"); err == nil {
		// Check for build tags or comments indicating target
		if strings.Contains(data, "esp32s3") || strings.Contains(data, "ESP32-S3") {
			return "ESP32-S3"
		}
		if strings.Contains(data, "esp32c3") || strings.Contains(data, "ESP32-C3") {
			return "ESP32-C3"
		}
		if strings.Contains(data, "esp32c6") || strings.Contains(data, "ESP32-C6") {
			return "ESP32-C6"
		}
	}

	return "ESP32" // Default
}

// ValidateArtifacts checks if required artifacts are present
func (d *WASMTinyGoDetector) ValidateArtifacts(artifacts *BuildArtifacts) error {
	if artifacts.App == "" {
		return &ProjectError{Message: "binary (ELF or .bin) path not found"}
	}
	return nil
}

// ValidateArtifactsWASM checks WASM artifacts
func (d *WASMTinyGoDetector) ValidateArtifactsWASM(artifacts *WASMBuildArtifacts) error {
	if artifacts.AppData == nil || len(artifacts.AppData) == 0 {
		return &ProjectError{Message: "binary (ELF or .bin) not found"}
	}
	return nil
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
