package wokwi

import (
	"encoding/json"
	"errors"
	"fmt"

	"codeberg.org/georgik/espbrew-go/pkg/protocol"
)

// DefaultChipTypes defines supported ESP chip types for Wokwi
var DefaultChipTypes = []string{
	"ESP32",
	"ESP32-S2",
	"ESP32-S3",
	"ESP32-C3",
	"ESP32-C6",
	"ESP32-H2",
	"ATmega328P",
	"RP2040",
}

// BoardTypeMapping maps chip types to Wokwi board types
var BoardTypeMapping = map[string]string{
	"ESP32":              "chip-esp32",
	"ESP32-S2":           "chip-esp32s2",
	"ESP32-S3":           "chip-esp32s3",
	"ESP32-C3":           "chip-esp32c3",
	"ESP32-C6":           "chip-esp32c6",
	"ESP32-H2":           "chip-esp32h2",
	"ESP32-S3-Box-3":     "board-esp32-s3-box-3",
	"ESP32-S3-DevKitC-1": "board-esp32-s3-devkitc-1",
}

// DefaultDiagram returns a default ESP32-S3-Box-3 diagram
func DefaultDiagram() string {
	diagram := map[string]interface{}{
		"version": 1,
		"author":  "ESPBrew",
		"editor":  "wokwi",
		"parts": []map[string]interface{}{
			{
				"type":  "board-esp32-s3-box-3",
				"id":    "esp",
				"attrs": map[string]interface{}{},
			},
		},
		"connections": [][]interface{}{
			{"esp:G43", "$serialMonitor:RX", ""},
			{"esp:G44", "$serialMonitor:TX", ""},
		},
	}

	data, _ := json.Marshal(diagram)
	return string(data)
}

// ValidateDiagramJSON validates the diagram JSON structure
func ValidateDiagramJSON(diagramJSON string) error {
	if diagramJSON == "" {
		return errors.New("diagram_json cannot be empty")
	}

	var diagram map[string]interface{}
	if err := json.Unmarshal([]byte(diagramJSON), &diagram); err != nil {
		return fmt.Errorf("invalid diagram JSON: %w", err)
	}

	// Check for required fields
	version, ok := diagram["version"].(float64)
	if !ok || version != 1 {
		return errors.New("diagram must have version 1")
	}

	parts, ok := diagram["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return errors.New("diagram must have at least one part")
	}

	return nil
}

// GetBoardTypeForChip returns the Wokwi board type for a given chip type
func GetBoardTypeForChip(chipType string) (string, error) {
	if boardType, ok := BoardTypeMapping[chipType]; ok {
		return boardType, nil
	}

	// Default to chip format if not found
	return fmt.Sprintf("chip-%s", formatChipName(chipType)), nil
}

// formatChipName converts chip type to Wokwi-compatible format
func formatChipName(chipType string) string {
	// Convert "ESP32-S3" to "esp32s3"
	result := ""
	for _, c := range chipType {
		if c == '-' {
			continue
		}
		result += string(c)
	}
	return result
}

// CreateConfig creates a WokwiConfig from the given parameters
func CreateConfig(chipType, diagramJSON string) (*protocol.WokwiConfig, error) {
	config := &protocol.WokwiConfig{
		ChipType:    chipType,
		DiagramJSON: diagramJSON,
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	if err := ValidateDiagramJSON(diagramJSON); err != nil {
		return nil, fmt.Errorf("invalid diagram: %w", err)
	}

	return config, nil
}

// CreateDefaultConfig creates a WokwiConfig with default diagram
func CreateDefaultConfig(chipType string) (*protocol.WokwiConfig, error) {
	diagram := DefaultDiagram()
	return CreateConfig(chipType, diagram)
}

// IsSupportedChipType checks if a chip type is supported by Wokwi
func IsSupportedChipType(chipType string) bool {
	for _, supported := range DefaultChipTypes {
		if supported == chipType {
			return true
		}
	}
	return false
}
