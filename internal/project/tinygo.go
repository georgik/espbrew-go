package project

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// TinyGoDetector detects TinyGo ESP32 projects
type TinyGoDetector struct{}

// Name returns "tinygo"
func (d *TinyGoDetector) Name() string {
	return "tinygo"
}

// Detect checks for TinyGo ESP project markers:
// - go.mod with tinygo.org/x dependencies
// - OR source files importing "machine" package (TinyGo hardware abstraction)
func (d *TinyGoDetector) Detect(dir string) bool {
	hasGoMod := fileExists(filepath.Join(dir, "go.mod"))
	if !hasGoMod {
		return false
	}

	// Check for TinyGo dependencies in go.mod
	hasTinyGoDeps, _ := d.hasTinyGoDependencies(filepath.Join(dir, "go.mod"))
	if hasTinyGoDeps {
		return true
	}

	// Fallback: check for machine package import in source files
	return d.hasMachineImport(dir)
}

// hasTinyGoDependencies checks if go.mod contains TinyGo-related dependencies
func (d *TinyGoDetector) hasTinyGoDependencies(goModPath string) (bool, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	tinygoDeps := []string{
		"tinygo.org/x/drivers",
		"tinygo.org/x/tinygl-font",
		"tinygo.org/x/adapters",
		"tinygo.org/x/console",
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for _, dep := range tinygoDeps {
			if strings.Contains(line, dep) {
				return true, nil
			}
		}
	}
	return false, scanner.Err()
}

// hasMachineImport checks if any .go file imports "machine" package
func (d *TinyGoDetector) hasMachineImport(dir string) bool {
	// Search for .go files (excluding test files for now)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if d.checkFileForMachineImport(filePath) {
			return true
		}
	}

	return false
}

// checkFileForMachineImport checks a single file for machine import
func (d *TinyGoDetector) checkFileForMachineImport(filePath string) bool {
	content, err := os.ReadFile(filePath)
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
	strContent := string(content)

	// Look for import statement followed by machine package reference
	patterns := []string{
		"\"machine\"",
		"`machine`",
		"machine\",", // for cases like m "machine"
		"machine\"",  // for cases like machine "..."
	}

	for _, pattern := range patterns {
		if strings.Contains(strContent, pattern) {
			// Verify it's actually an import statement by checking nearby context
			idx := strings.Index(strContent, pattern)
			if idx > 0 {
				// Check if "import" appears before this pattern
				beforeImport := strContent[max(0, idx-100):idx]
				if strings.Contains(beforeImport, "import") {
					return true
				}
			}
		}
	}

	return false
}

// FindBuildDir locates TinyGo build output
// TinyGo outputs to current directory (no target/ subdirectory like Rust)
func (d *TinyGoDetector) FindBuildDir(dir string) (string, error) {
	// TinyGo builds output to the project directory itself
	// Return the absolute path of the project directory
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	// Verify it's a valid directory
	if dirExists(absPath) {
		return absPath, nil
	}

	return "", os.ErrNotExist
}

// GetArtifacts returns paths to TinyGo build outputs
// TinyGo produces ELF file (or .bin if converted) named after module
func (d *TinyGoDetector) GetArtifacts(buildDir string) (*BuildArtifacts, error) {
	artifacts := &BuildArtifacts{
		BuildDir: buildDir,
	}

	// Try to extract module name from go.mod
	moduleName := d.extractModuleName(filepath.Join(buildDir, "go.mod"))
	if moduleName != "" {
		// Look for ELF file matching module name
		elfPath := filepath.Join(buildDir, moduleName)
		if fileExists(elfPath) {
			artifacts.App = elfPath
			return artifacts, nil
		}
	}

	// Fallback: find any ELF file in build directory
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return artifacts, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip test files and build artifacts
		name := entry.Name()
		if strings.HasSuffix(name, "_test.go") ||
			strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, ".mod") ||
			strings.HasSuffix(name, ".sum") ||
			strings.HasSuffix(name, ".txt") {
			continue
		}

		fullPath := filepath.Join(buildDir, name)
		info, _ := entry.Info()

		// Check if it's an ELF file (executable and large enough)
		if info.Mode().Perm()&0111 != 0 && info.Size() > 10000 {
			artifacts.App = fullPath
			return artifacts, nil
		}

		// Also check for ELF magic without exec bit
		data, err := os.ReadFile(fullPath)
		if err == nil && len(data) > 4 {
			if data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F' {
				artifacts.App = fullPath
				return artifacts, nil
			}
		}
	}

	return artifacts, nil
}

// extractModuleName reads the module name from go.mod
func (d *TinyGoDetector) extractModuleName(goModPath string) string {
	f, err := os.Open(goModPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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
