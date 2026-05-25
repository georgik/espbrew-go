package flash

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FlashArgs represents parsed ESP-IDF flash_args content.
type FlashArgs struct {
	FlashMode string
	FlashFreq string
	FlashSize string
	Files     []FlashFile
}

// FlashFile represents a file to flash with its offset.
type FlashFile struct {
	Offset uint32
	Path   string
}

// ParseFlashArgs parses ESP-IDF flash_args format.
// Format:
//
//	Line 1: --flash-mode <mode> --flash-freq <freq> --flash-size <size>
//	Line 2+: <offset> <filename>
func ParseFlashArgs(data []byte) (*FlashArgs, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	args := &FlashArgs{}

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if lineNum == 1 {
			// Parse flash settings
			parts := strings.Fields(line)
			for i := 0; i < len(parts); i++ {
				switch parts[i] {
				case "--flash-mode":
					if i+1 < len(parts) {
						args.FlashMode = parts[i+1]
						i++
					}
				case "--flash-freq":
					if i+1 < len(parts) {
						args.FlashFreq = parts[i+1]
						i++
					}
				case "--flash-size":
					if i+1 < len(parts) {
						args.FlashSize = parts[i+1]
						i++
					}
				}
			}
		} else {
			// Parse file lines: <offset> <filename>
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var offset uint64
				_, err := fmt.Sscanf(parts[0], "0x%x", &offset)
				if err != nil {
					// Try decimal
					_, err = fmt.Sscanf(parts[0], "%d", &offset)
					if err != nil {
						continue // Skip invalid lines
					}
				}
				args.Files = append(args.Files, FlashFile{
					Offset: uint32(offset),
					Path:   parts[1],
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse flash_args: %w", err)
	}

	return args, nil
}

// FindFlashArgs searches for flash_args in common build directories.
func FindFlashArgs(buildDir string) (string, error) {
	// Common paths to check
	candidates := []string{
		filepath.Join(buildDir, "flash_args"),
		filepath.Join(buildDir, "build", "flash_args"),
		"flash_args",
		"build/flash_args",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", os.ErrNotExist
}

// ResolveBuildPath finds a file in ESP-IDF build directories.
// Search order:
//  1. {build-dir}/{filename}
//  2. {build-dir}/bootloader/{filename}
//  3. {filename}
func ResolveBuildPath(buildDir, filename string) string {
	candidates := []string{
		filepath.Join(buildDir, filename),
		filepath.Join(buildDir, "bootloader", filename),
		filename,
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return filename // Fallback to original
}
