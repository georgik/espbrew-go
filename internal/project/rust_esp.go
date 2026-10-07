package project

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// RustESPDetector detects Rust no_std ESP projects
type RustESPDetector struct{}

// Name returns "rust-esp"
func (d *RustESPDetector) Name() string {
	return "rust-esp"
}

// Detect checks for Rust ESP project markers:
// - Cargo.toml with ESP HAL dependencies
// - .cargo/config.toml with xtensa or riscv target
func (d *RustESPDetector) Detect(dir string) bool {
	hasCargoToml := fileExists(filepath.Join(dir, "Cargo.toml"))
	if !hasCargoToml {
		return false
	}

	// Check for ESP dependencies in Cargo.toml
	hasESPDeps, _ := d.hasESPDependencies(filepath.Join(dir, "Cargo.toml"))
	if !hasESPDeps {
		return false
	}

	// Check for .cargo config with ESP target triple
	cargoConfig := filepath.Join(dir, ".cargo", "config.toml")
	if !fileExists(cargoConfig) {
		cargoConfig = filepath.Join(dir, ".cargo", "config")
	}
	if !fileExists(cargoConfig) {
		return false
	}

	hasESPTarget, _ := d.hasESPTarget(cargoConfig)
	return hasESPTarget
}

// hasESPDependencies checks if Cargo.toml contains ESP-related dependencies
func (d *RustESPDetector) hasESPDependencies(cargoPath string) (bool, error) {
	f, err := os.Open(cargoPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	espCrates := []string{
		"esp-hal", "esp-backtrace", "esp-println", "esp-alloc",
		"esp-idf-hal", "esp-idf-sys", "esp32-hal",
		"esp8266-hal", "esp-wifi",
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		for _, crateName := range espCrates {
			if strings.Contains(line, crateName) {
				return true, nil
			}
		}
	}
	return false, scanner.Err()
}

// hasESPTarget checks if .cargo/config has xtensa or riscv target
func (d *RustESPDetector) hasESPTarget(configPath string) (bool, error) {
	f, err := os.Open(configPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	espTargets := []string{
		"xtensa-esp32", "xtensa-esp32s2", "xtensa-esp32s3",
		"xtensa-esp32c3", "xtensa-esp32c6", "xtensa-esp32h2",
		"riscv32imac-unknown-none-elf", "riscv32imc-unknown-none-elf",
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return false, err
	}

	for _, target := range espTargets {
		if strings.Contains(string(content), target) {
			return true, nil
		}
	}

	return false, nil
}

// FindBuildDir locates the Rust target directory
func (d *RustESPDetector) FindBuildDir(dir string) (string, error) {
	// First, try to determine the target triple from .cargo/config
	cargoConfig := filepath.Join(dir, ".cargo", "config.toml")
	if !fileExists(cargoConfig) {
		cargoConfig = filepath.Join(dir, ".cargo", "config")
	}

	targetTriple, err := d.extractTargetTriple(cargoConfig)
	if err != nil {
		// Fallback to searching for target directories
		return d.findTargetDir(dir)
	}

	targetDir := filepath.Join(dir, "target", targetTriple, "release")
	if dirExists(targetDir) {
		return targetDir, nil
	}

	// Try the fallback
	return d.findTargetDir(dir)
}

// extractTargetTriple reads the target triple from .cargo/config and defers the
// parsing to the shared ExtractTargetTriple helper so the detector and the
// artifact packer stay in lockstep.
func (d *RustESPDetector) extractTargetTriple(configPath string) (string, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	if triple := ExtractTargetTriple(string(content)); triple != "" {
		return triple, nil
	}
	return "", os.ErrNotExist
}

// findTargetDir searches for target directories with ESP target triples
func (d *RustESPDetector) findTargetDir(dir string) (string, error) {
	targetBase := filepath.Join(dir, "target")
	if !dirExists(targetBase) {
		return "", os.ErrNotExist
	}

	espTargets := []string{
		"xtensa-esp32s3-none-elf", "xtensa-esp32s2-none-elf", "xtensa-esp32-none-elf",
		"xtensa-esp32c3-none-elf", "xtensa-esp32c6-none-elf", "xtensa-esp32h2-none-elf",
		"riscv32imac-unknown-none-elf", "riscv32imc-unknown-none-elf",
	}

	for _, target := range espTargets {
		targetDir := filepath.Join(targetBase, target, "release")
		if dirExists(targetDir) {
			return targetDir, nil
		}
	}

	return "", os.ErrNotExist
}

// GetArtifacts returns paths to Rust ESP build outputs. The ELF is selected by
// the shared FindELF helper so the detector and the artifact packer agree on
// exactly which binary is flashed.
func (d *RustESPDetector) GetArtifacts(buildDir string) (*BuildArtifacts, error) {
	artifacts := &BuildArtifacts{
		BuildDir: buildDir,
	}
	if elf, err := FindELF(buildDir); err == nil {
		artifacts.App = elf
	}
	return artifacts, nil
}
