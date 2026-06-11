// Command to build Go WASM for the UI
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	// Get the directory where this command is run
	cmdDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Determine paths
	// Assuming cmd/wasm-compiler is the working directory
	projectRoot := filepath.Dir(filepath.Dir(cmdDir))
	uiDir := filepath.Join(projectRoot, "internal", "ui")
	webDir := filepath.Join(projectRoot, "web")
	outputFile := filepath.Join(webDir, "main.wasm")

	// Create web directory if it doesn't exist
	if err := os.MkdirAll(webDir, 0755); err != nil {
		log.Fatalf("Failed to create web directory: %v", err)
	}

	// Build command
	args := []string{
		"build",
		"-o", outputFile,
		uiDir,
	}

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(),
		"GOOS=js",
		"GOARCH=wasm",
	)

	// Set stdout and stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the build
	fmt.Printf("Building WASM UI...\n")
	fmt.Printf("  Source: %s\n", uiDir)
	fmt.Printf("  Output: %s\n", outputFile)

	if err := cmd.Run(); err != nil {
		log.Fatalf("Build failed: %v", err)
	}

	fmt.Printf("\nBuild successful!\n")
	fmt.Printf("WASM file: %s\n", outputFile)
	fmt.Printf("File size: %d bytes\n", fileSize(outputFile))

	// Check if wasm_exec.js exists
	wasmExecJs := filepath.Join(webDir, "wasm_exec.js")
	if _, err := os.Stat(wasmExecJs); os.IsNotExist(err) {
		fmt.Printf("\nWarning: %s not found\n", wasmExecJs)
		fmt.Printf("Copy it from Go SDK: cp $(go env GOROOT)/misc/wasm/wasm_exec.js %s\n", webDir)
	}
}

// fileSize returns the size of a file
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
