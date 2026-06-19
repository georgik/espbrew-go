//go:build js
// +build js

package project

import (
	"strings"
)

// WASMRustESPDetector detects Rust no_std ESP projects in WASM environment
type WASMRustESPDetector struct{}

// Name returns "rust-esp"
func (d *WASMRustESPDetector) Name() string {
	return "rust-esp"
}

// Detect checks for Rust ESP project markers:
// - Cargo.toml with ESP HAL dependencies
// - .cargo/config.toml with xtensa or riscv target
func (d *WASMRustESPDetector) Detect(files *WASMFileMap) bool {
	// Check for Cargo.toml
	if !files.FileExists("Cargo.toml") {
		return false
	}

	// Check for ESP dependencies in Cargo.toml
	hasESPDeps := d.hasESPDependencies(files)
	if !hasESPDeps {
		return false
	}

	// Check for .cargo config with ESP target
	cargoConfig := ".cargo/config.toml"
	if !files.FileExists(cargoConfig) {
		cargoConfig = ".cargo/config"
	}
	if !files.FileExists(cargoConfig) {
		return false
	}

	hasESPTarget := d.hasESPTarget(files, cargoConfig)
	return hasESPTarget
}

// hasESPDependencies checks if Cargo.toml contains ESP-related dependencies
func (d *WASMRustESPDetector) hasESPDependencies(files *WASMFileMap) bool {
	data, err := files.ReadFileString("Cargo.toml")
	if err != nil {
		return false
	}

	espCrates := []string{
		"esp-hal", "esp-backtrace", "esp-println", "esp-alloc",
		"esp-idf-hal", "esp-idf-sys", "esp32-hal",
		"esp8266-hal", "esp-wifi", "esp32c3-hal", "esp32s3-hal",
	}

	lowerData := strings.ToLower(data)
	for _, crate := range espCrates {
		if strings.Contains(lowerData, crate) {
			return true
		}
	}

	return false
}

// hasESPTarget checks if .cargo/config has xtensa or riscv target
func (d *WASMRustESPDetector) hasESPTarget(files *WASMFileMap, configPath string) bool {
	data, err := files.ReadFileString(configPath)
	if err != nil {
		return false
	}

	espTargets := []string{
		"xtensa-esp32", "xtensa-esp32s2", "xtensa-esp32s3",
		"xtensa-esp32c3", "xtensa-esp32c6", "xtensa-esp32h2",
		"riscv32imac-unknown-none-elf", "riscv32imc-unknown-none-elf",
	}

	for _, target := range espTargets {
		if strings.Contains(data, target) {
			return true
		}
	}

	return false
}

// GetArtifacts returns paths to Rust ESP build outputs
// Rust projects output ELF binaries in target/{triple}/release/
func (d *WASMRustESPDetector) GetArtifacts(files *WASMFileMap) (*BuildArtifacts, error) {
	wasmArtifacts, err := d.GetArtifactsWithStructure(files)
	if err != nil {
		return nil, err
	}
	return wasmArtifacts.ToNative(), nil
}

// GetArtifactsWithStructure returns WASM build artifacts with structure info
func (d *WASMRustESPDetector) GetArtifactsWithStructure(files *WASMFileMap) (*WASMBuildArtifacts, error) {
	artifacts := &WASMBuildArtifacts{}

	// Determine target triple from .cargo/config
	targetTriple := d.extractTargetTriple(files)

	// Try to find the ELF binary
	// Common locations:
	// - target/{triple}/release/{project_name}
	// - target/{triple}/release/{binary_name}
	// - target/release/{project_name}

	var candidates []string

	if targetTriple != "" {
		candidates = append(candidates,
			"target/"+targetTriple+"/release/",
			targetTriple+"/release/",
		)
	}

	candidates = append(candidates, "target/release/")

	// Find ELF files in candidate directories
	for _, prefix := range candidates {
		for path, data := range files.files {
			if strings.HasPrefix(path, prefix) && isELFFile(data) {
				// Skip .d, .o files and other build artifacts
				if !strings.HasSuffix(path, ".d") &&
					!strings.HasSuffix(path, ".o") &&
					!strings.HasSuffix(path, ".a") &&
					!strings.HasSuffix(path, ".rmeta") &&
					!strings.HasSuffix(path, ".rlib") {

					// Prefer the main binary (usually the project name)
					artifacts.AppData = data
					artifacts.AppPath = path
					break
				}
			}
		}
		if artifacts.AppData != nil {
			break
		}
	}

	// Fallback: find any ELF file
	if artifacts.AppData == nil {
		elfFiles := findELFFiles(files)
		if len(elfFiles) > 0 {
			// Prefer the largest ELF
			for _, path := range elfFiles {
				if data, ok := files.files[path]; ok {
					if artifacts.AppData == nil || len(data) > len(artifacts.AppData) {
						artifacts.AppData = data
						artifacts.AppPath = path
					}
				}
			}
		}
	}

	return artifacts, nil
}

// ExtractTargetTriple reads the target triple from .cargo/config (exported)
func (d *WASMRustESPDetector) ExtractTargetTriple(files *WASMFileMap) string {
	return d.extractTargetTriple(files)
}

// extractTargetTriple reads the target triple from .cargo/config
func (d *WASMRustESPDetector) extractTargetTriple(files *WASMFileMap) string {
	configPath := ".cargo/config.toml"
	if !files.FileExists(configPath) {
		configPath = ".cargo/config"
	}

	data, err := files.ReadFileString(configPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for target section
		if strings.HasPrefix(line, "[target.") {
			// Extract target triple
			start := strings.Index(line, "[target.")
			if start >= 0 {
				rest := line[start+8:] // Skip "[target."
				end := strings.Index(rest, "]")
				if end > 0 {
					return rest[:end]
				}
			}
		}

		// Check for build.target
		if strings.HasPrefix(line, "target") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				target := strings.Trim(strings.TrimSpace(parts[1]), "\"")
				if target != "" && !strings.HasPrefix(target, "$") {
					return target
				}
			}
		}
	}

	return ""
}

// GetChipType attempts to determine chip type from target triple
func (d *WASMRustESPDetector) GetChipType(files *WASMFileMap) string {
	targetTriple := d.extractTargetTriple(files)

	// Map target triples to chip types
	chipMap := map[string]string{
		"xtensa-esp32-none-elf":        "ESP32",
		"xtensa-esp32s2-none-elf":      "ESP32-S2",
		"xtensa-esp32s3-none-elf":      "ESP32-S3",
		"xtensa-esp32c3-none-elf":      "ESP32-C3",
		"xtensa-esp32c6-none-elf":      "ESP32-C6",
		"xtensa-esp32h2-none-elf":      "ESP32-H2",
		"riscv32imac-unknown-none-elf": "ESP32-C6", // RISC-V variant
		"riscv32imc-unknown-none-elf":  "ESP32-C6",
	}

	for triple, chip := range chipMap {
		if strings.Contains(targetTriple, triple) {
			return chip
		}
	}

	// Fallback: check Cargo.toml for chip-specific dependencies
	if data, err := files.ReadFileString("Cargo.toml"); err == nil {
		if strings.Contains(data, "esp32s3") || strings.Contains(data, "esp-hal/esp32s3") {
			return "ESP32-S3"
		}
		if strings.Contains(data, "esp32c3") || strings.Contains(data, "esp-hal/esp32c3") {
			return "ESP32-C3"
		}
		if strings.Contains(data, "esp32s2") || strings.Contains(data, "esp-hal/esp32s2") {
			return "ESP32-S2"
		}
		if strings.Contains(data, "esp32c6") || strings.Contains(data, "esp-hal/esp32c6") {
			return "ESP32-C6"
		}
	}

	return "ESP32" // Default
}

// ValidateArtifacts checks if required artifacts are present
func (d *WASMRustESPDetector) ValidateArtifacts(artifacts *BuildArtifacts) error {
	if artifacts.App == "" {
		return &ProjectError{Message: "ELF binary path not found"}
	}
	return nil
}

// ValidateArtifactsWASM checks WASM artifacts
func (d *WASMRustESPDetector) ValidateArtifactsWASM(artifacts *WASMBuildArtifacts) error {
	if artifacts.AppData == nil || len(artifacts.AppData) == 0 {
		return &ProjectError{Message: "ELF binary not found"}
	}
	return nil
}
