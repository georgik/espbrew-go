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

	// Check if wasm_exec.js exists, copy if missing
	wasmExecJs := filepath.Join(webDir, "wasm_exec.js")
	if _, err := os.Stat(wasmExecJs); os.IsNotExist(err) {
		fmt.Printf("\nCopying wasm_exec.js...\n")

		// Try common locations
		goroot := os.Getenv("GOROOT")
		locations := []string{
			filepath.Join(goroot, "misc/wasm/wasm_exec.js"),
			filepath.Join(goroot, "src/crypto/tls/wasm/wasm_exec.js"),
		}

		copied := false
		for _, loc := range locations {
			if _, err := os.Stat(loc); err == nil {
				data, err := os.ReadFile(loc)
				if err == nil {
					if err := os.WriteFile(wasmExecJs, data, 0644); err == nil {
						fmt.Printf("  Copied from %s\n", loc)
						copied = true
						break
					}
				}
			}
		}

		if !copied {
			fmt.Printf("Warning: Could not find wasm_exec.js in Go SDK\n")
			fmt.Printf("Download from: https://raw.githubusercontent.com/golang/go/master/misc/wasm/wasm_exec.js\n")
		}
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
