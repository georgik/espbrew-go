package snap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OutputFormat defines the output format for snapshot results.
type OutputFormat string

const (
	// OutputFormatJSON outputs full metadata and base64-encoded image
	OutputFormatJSON OutputFormat = "json"
	// OutputFormatText outputs human-readable formatted sections
	OutputFormatText OutputFormat = "text"
	// OutputFormatCompact outputs a single-line summary
	OutputFormatCompact OutputFormat = "compact"
)

// Handler handles output formatting and writing for snapshot results.
type Handler struct {
	format  OutputFormat
	output  io.Writer
	saveDir string
}

// NewHandler creates a new output handler with the specified format.
// If saveDir is non-empty, images and metadata will be saved to disk.
func NewHandler(format OutputFormat, output io.Writer, saveDir string) *Handler {
	return &Handler{
		format:  format,
		output:  output,
		saveDir: saveDir,
	}
}

// Write outputs the snapshot result in the configured format.
// If saveDir is configured, also saves image and metadata files.
func (h *Handler) Write(result *SnapResult) error {
	if result == nil {
		return fmt.Errorf("cannot write nil result")
	}

	// Save files if saveDir is configured
	if h.saveDir != "" {
		if err := h.saveFiles(result); err != nil {
			return fmt.Errorf("failed to save files: %w", err)
		}
	}

	// Write formatted output
	switch h.format {
	case OutputFormatJSON:
		return h.writeJSON(result)
	case OutputFormatText:
		return h.writeText(result)
	case OutputFormatCompact:
		return h.writeCompact(result)
	default:
		return fmt.Errorf("unknown output format: %s", h.format)
	}
}

// writeJSON outputs the result as JSON with base64-encoded image.
func (h *Handler) writeJSON(result *SnapResult) error {
	// Ensure image is base64-encoded
	if len(result.ImageData) > 0 && result.ImageBase64 == "" {
		result.ImageBase64 = base64.StdEncoding.EncodeToString(result.ImageData)
	}

	encoder := json.NewEncoder(h.output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result.ToMap())
}

// writeText outputs human-readable formatted sections.
func (h *Handler) writeText(result *SnapResult) error {
	var b strings.Builder
	m := result.Metadata

	// Header section
	b.WriteString("=== Snapshot Result ===\n\n")

	// Status section
	b.WriteString(fmt.Sprintf("Status:    %s\n", m.Status))
	b.WriteString(fmt.Sprintf("Snap ID:   %s\n", m.SnapID))
	b.WriteString(fmt.Sprintf("Timestamp: %s\n", m.Timestamp.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Duration:  %d ms\n", m.Duration))
	if m.Error != "" {
		b.WriteString(fmt.Sprintf("Error:     %s\n", m.Error))
	}
	b.WriteString("\n")

	// Device section
	b.WriteString("--- Device ---\n")
	if m.DevicePath != "" {
		b.WriteString(fmt.Sprintf("Path:     %s\n", m.DevicePath))
	}
	if m.DeviceNode != "" {
		b.WriteString(fmt.Sprintf("Node:     %s\n", m.DeviceNode))
	}
	if m.ChipID != "" {
		b.WriteString(fmt.Sprintf("Chip ID:  %s\n", m.ChipID))
	}
	if m.ChipName != "" {
		b.WriteString(fmt.Sprintf("Chip:     %s\n", m.ChipName))
	}
	b.WriteString("\n")

	// Flash section
	if m.FlashEnabled || m.FlashSkipped {
		b.WriteString("--- Flash ---\n")
		b.WriteString(fmt.Sprintf("Enabled:  %v\n", m.FlashEnabled))
		if m.FlashSkipped {
			b.WriteString(fmt.Sprintf("Skipped:  %v\n", m.FlashSkipped))
		}
		if m.FlashFirmware != "" {
			b.WriteString(fmt.Sprintf("Firmware: %s\n", m.FlashFirmware))
		}
		b.WriteString(fmt.Sprintf("Offset:   0x%x\n", m.FlashOffset))
		if m.FlashHashBefore != "" {
			b.WriteString(fmt.Sprintf("Hash Before: %s\n", m.FlashHashBefore))
		}
		if m.FlashHashAfter != "" {
			b.WriteString(fmt.Sprintf("Hash After:  %s\n", m.FlashHashAfter))
		}
		b.WriteString("\n")
	}

	// Monitor section
	if m.MonitorEnabled {
		b.WriteString("--- Serial Monitor ---\n")
		b.WriteString(fmt.Sprintf("Duration: %v\n", m.MonitorDuration))
		b.WriteString(fmt.Sprintf("Baud:     %d\n", m.MonitorBaudRate))
		b.WriteString(fmt.Sprintf("Entries:  %d\n", m.LogEntryCount))
		if len(result.Logs) > 0 {
			b.WriteString("\nLog entries:\n")
			for i, entry := range result.Logs {
				ts := entry.Timestamp.Format("15:04:05.000")
				prefix := ts
				if entry.Level != "" {
					prefix = fmt.Sprintf("%s [%s]", ts, entry.Level)
				}
				if entry.Source != "" {
					prefix = fmt.Sprintf("%s %s:", prefix, entry.Source)
				}
				b.WriteString(fmt.Sprintf("  [%2d] %s %s\n", i+1, prefix, entry.Message))
			}
		}
		b.WriteString("\n")
	}

	// Capture section
	if m.CaptureEnabled {
		b.WriteString("--- Camera Capture ---\n")
		b.WriteString(fmt.Sprintf("Enabled:    %v\n", m.CaptureEnabled))
		if m.CameraID != "" {
			b.WriteString(fmt.Sprintf("Camera:     %s\n", m.CameraID))
		}
		if m.ImageFormat != "" {
			b.WriteString(fmt.Sprintf("Format:     %s\n", m.ImageFormat))
		}
		b.WriteString(fmt.Sprintf("Image Size: %d bytes\n", m.ImageSize))
		if len(result.ImageData) > 0 {
			b.WriteString(fmt.Sprintf("Image:      %d bytes (use --save to persist)\n", len(result.ImageData)))
		}
		b.WriteString("\n")
	}

	b.WriteString("========================\n")

	_, err := io.WriteString(h.output, b.String())
	return err
}

// writeCompact outputs a single-line summary with key fields.
func (h *Handler) writeCompact(result *SnapResult) error {
	m := result.Metadata

	parts := []string{
		m.SnapID,
		string(m.Status),
	}

	// Add duration
	parts = append(parts, fmt.Sprintf("%dms", m.Duration))

	// Add device info
	if m.DeviceNode != "" {
		parts = append(parts, m.DeviceNode)
	}
	if m.ChipName != "" {
		parts = append(parts, m.ChipName)
	}

	// Add flash info
	if m.FlashEnabled {
		parts = append(parts, "flash")
		if m.FlashFirmware != "" {
			parts = append(parts, filepath.Base(m.FlashFirmware))
		}
	}

	// Add monitor info
	if m.MonitorEnabled {
		parts = append(parts, fmt.Sprintf("monitor:%d", m.LogEntryCount))
	}

	// Add capture info
	if m.CaptureEnabled {
		parts = append(parts, fmt.Sprintf("capture:%d", m.ImageSize))
	}

	line := strings.Join(parts, " ") + "\n"
	_, err := io.WriteString(h.output, line)
	return err
}

// saveFiles saves the image and metadata to disk.
func (h *Handler) saveFiles(result *SnapResult) error {
	// Create save directory if it doesn't exist
	if err := os.MkdirAll(h.saveDir, 0755); err != nil {
		return fmt.Errorf("failed to create save directory: %w", err)
	}

	// Save image if present
	if len(result.ImageData) > 0 {
		imagePath := filepath.Join(h.saveDir, fmt.Sprintf("snap-%s.jpg", result.Metadata.SnapID))
		if err := os.WriteFile(imagePath, result.ImageData, 0644); err != nil {
			return fmt.Errorf("failed to write image: %w", err)
		}
	}

	// Save metadata as JSON
	metaPath := filepath.Join(h.saveDir, fmt.Sprintf("snap-%s.json", result.Metadata.SnapID))
	data, err := json.MarshalIndent(result.Metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}
