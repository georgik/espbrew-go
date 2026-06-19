//go:build js
// +build js

package project

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"
)

// WASMFileMap provides file access for WASM using in-memory file map
type WASMFileMap struct {
	files map[string][]byte
}

// NewWASMFileMap creates a new file map from folder files
func NewWASMFileMap(files map[string][]byte) *WASMFileMap {
	return &WASMFileMap{
		files: files,
	}
}

// FileExists checks if a file exists in the map
func (fm *WASMFileMap) FileExists(path string) bool {
	_, ok := fm.files[path]
	return ok
}

// ReadFile reads file content from the map
func (fm *WASMFileMap) ReadFile(path string) ([]byte, error) {
	if data, ok := fm.files[path]; ok {
		return data, nil
	}
	return nil, ErrFileNotFound
}

// ReadFileString reads file content as string
func (fm *WASMFileMap) ReadFileString(path string) (string, error) {
	data, err := fm.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// HasPrefix checks if any file starts with the given path prefix
// Useful for finding files in subdirectories
func (fm *WASMFileMap) HasPrefix(prefix string) bool {
	for path := range fm.files {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// FindFiles returns all files that match the given predicate
func (fm *WASMFileMap) FindFiles(pred func(string) bool) []string {
	var matches []string
	for path := range fm.files {
		if pred(path) {
			matches = append(matches, path)
		}
	}
	return matches
}

// JoinPath joins path elements - WASM-safe version of filepath.Join
func (fm *WASMFileMap) JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// WASMDetector is the WASM-compatible detector interface
type WASMDetector interface {
	// Name returns the detector name
	Name() string

	// Detect checks if the files match this project type
	Detect(files *WASMFileMap) bool

	// GetArtifacts returns build artifacts from the file map
	GetArtifacts(files *WASMFileMap) (*BuildArtifacts, error)
}

// WASMBuildArtifacts contains in-memory build artifacts for WASM
type WASMBuildArtifacts struct {
	BootloaderData []byte
	PartitionsData []byte
	AppData        []byte
	FlashArgsData  []byte

	// Original paths (for reference)
	BootloaderPath string
	PartitionsPath string
	AppPath        string
	FlashArgsPath  string
}

// ToNative converts WASM artifacts to native BuildArtifacts structure
// For compatibility with existing code
func (wa *WASMBuildArtifacts) ToNative() *BuildArtifacts {
	return &BuildArtifacts{
		Bootloader: wa.BootloaderPath,
		Partitions: wa.PartitionsPath,
		App:        wa.AppPath,
		FlashArgs:  wa.FlashArgsPath,
	}
}

// WASMRegistry holds WASM-compatible detectors
type WASMRegistry struct {
	detectors []WASMDetector
}

// NewWASMRegistry creates a new WASM detector registry
func NewWASMRegistry() *WASMRegistry {
	return &WASMRegistry{
		detectors: []WASMDetector{},
	}
}

// Register adds a detector to the registry
func (r *WASMRegistry) Register(d WASMDetector) {
	r.detectors = append(r.detectors, d)
}

// Detect finds the project type and returns the appropriate detector
func (r *WASMRegistry) Detect(files *WASMFileMap) (ProjectType, WASMDetector) {
	for _, d := range r.detectors {
		if d.Detect(files) {
			return ProjectType(d.Name()), d
		}
	}
	return ProjectTypeNone, nil
}

// DetectWASM is a convenience function that detects project type from files
func DetectWASM(files map[string][]byte) (ProjectType, WASMDetector, *WASMFileMap) {
	fileMap := NewWASMFileMap(files)

	registry := NewWASMRegistry()
	registry.Register(&WASMESPIDFDetector{})
	registry.Register(&WASMRustESPDetector{})
	registry.Register(&WASMTinyGoDetector{})

	projType, detector := registry.Detect(fileMap)
	return projType, detector, fileMap
}

// Helper functions for WASM detectors

// fileHasContent checks if a file contains specific content
func fileHasContent(files *WASMFileMap, path string, content string) bool {
	data, err := files.ReadFileString(path)
	if err != nil {
		return false
	}
	return strings.Contains(data, content)
}

// fileHasAnyContent checks if a file contains any of the given content strings
func fileHasAnyContent(files *WASMFileMap, path string, contents []string) bool {
	data, err := files.ReadFileString(path)
	if err != nil {
		return false
	}
	for _, content := range contents {
		if strings.Contains(data, content) {
			return true
		}
	}
	return false
}

// fileHasAllContent checks if a file contains all of the given content strings
func fileHasAllContent(files *WASMFileMap, path string, contents []string) bool {
	data, err := files.ReadFileString(path)
	if err != nil {
		return false
	}
	for _, content := range contents {
		if !strings.Contains(data, content) {
			return false
		}
	}
	return true
}

// scanLines scans file content and calls predicate for each line
// Returns true if predicate returns true for any line
func scanLines(data []byte, predicate func(string) bool) bool {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if predicate(scanner.Text()) {
			return true
		}
	}
	return false
}

// findLargestBinInMap finds the largest .bin file in the map
func findLargestBinInMap(files *WASMFileMap) (string, []byte) {
	var largestPath string
	var largestData []byte
	var largestSize int64

	for path, data := range files.files {
		if strings.HasSuffix(path, ".bin") {
			size := int64(len(data))
			if size > largestSize {
				largestSize = size
				largestPath = path
				largestData = data
			}
		}
	}

	return largestPath, largestData
}

// ELF header check
func isELFFile(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	return data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
}

// Find ELF files in the map
func findELFFiles(files *WASMFileMap) []string {
	var elfFiles []string
	for path, data := range files.files {
		if isELFFile(data) {
			elfFiles = append(elfFiles, path)
		}
	}
	return elfFiles
}

// Errors
var (
	ErrFileNotFound = &ProjectError{Message: "file not found"}
)

// ProjectError represents a project detection error
type ProjectError struct {
	Message string
}

func (e *ProjectError) Error() string {
	return e.Message
}
