// update-stubs copies stub TOML files from the espflash submodule to internal/espflash/stubs/
// Usage: go run tools/update-stubs.go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	sourceDir := "vendor/espflash/espflash/resources/stubs"
	targetDir := "internal/espflash/stubs"

	// Ensure target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating target directory: %v\n", err)
		os.Exit(1)
	}

	// Read source directory
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading source directory %s: %v\n", sourceDir, err)
		fmt.Fprintf(os.Stderr, "Make sure the submodule is initialized: git submodule update --init --recursive\n")
		os.Exit(1)
	}

	// Copy each stub file
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".toml" {
			continue
		}

		sourcePath := filepath.Join(sourceDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())

		if err := copyFile(sourcePath, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error copying %s: %v\n", entry.Name(), err)
			os.Exit(1)
		}
		count++
	}

	fmt.Printf("Updated %d stub files from %s to %s\n", count, sourceDir, targetDir)
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
