package pages

import (
	"fmt"
	"strings"
)

// unprobedFormValues captures the fields a user fills in when registering a
// device that has not yet been probed (no DeviceID). It is deliberately free
// of any DOM access so that request construction can be unit-tested on the
// host (any GOOS), not only under js/wasm.
type unprobedFormValues struct {
	Path        string
	MAC         string
	Name        string
	ChipType    string
	CustomChip  string
	ChipRev     string
	FlashSize   string
	BoardModel  string
	Description string
	Aliases     string
	Tags        string
}

// buildUnprobedRequest assembles the PATCH body used to register an unprobed
// device from parsed form values. Pure: no DOM access, fully host-testable.
func buildUnprobedRequest(f unprobedFormValues) (map[string]interface{}, error) {
	// Preserve original ordering: chip type is required before validation.
	if f.ChipType == "" {
		return nil, fmt.Errorf("chip type is required")
	}
	if f.MAC != "" && !isValidMAC(f.MAC) {
		return nil, fmt.Errorf("invalid MAC address format. Use AA:BB:CC:DD:EE:FF")
	}

	// Custom chip option: fall back only when the custom value is non-empty,
	// mirroring the original form logic (a leftover "Custom" is kept as-is).
	chipType := f.ChipType
	if chipType == "Custom" && f.CustomChip != "" {
		chipType = f.CustomChip
	}

	req := map[string]interface{}{
		"path":        f.Path,
		"mac_address": f.MAC,
		"chip_type":   chipType,
		"aliases":     splitString(f.Aliases, ","),
		"tags":        splitString(f.Tags, ","),
		"name":        f.Name,
	}

	if f.ChipRev != "" {
		req["chip_rev"] = f.ChipRev
	}
	if f.FlashSize != "" {
		if flashSizeInt := parseFlashSize(f.FlashSize); flashSizeInt > 0 {
			req["flash_size"] = flashSizeInt
		}
	}
	if f.BoardModel != "" {
		req["board_model"] = f.BoardModel
	}
	if f.Description != "" {
		req["description"] = f.Description
	}

	return req, nil
}

// attributeFormValues captures the fields a user edits on a saved device's
// attribute page. Like unprobedFormValues it carries no DOM references.
type attributeFormValues struct {
	Name       string
	Aliases    string
	Tags       string
	Protected  bool
	ChipType   string // "" means "leave unchanged"
	CustomChip string // used when ChipType == "Custom"
}

// buildAttributeRequest assembles the PATCH body used to update an existing
// device's attributes. Pure: no DOM access, fully host-testable.
func buildAttributeRequest(f attributeFormValues) map[string]interface{} {
	req := map[string]interface{}{
		"aliases":   splitString(f.Aliases, ","),
		"tags":      splitString(f.Tags, ","),
		"protected": f.Protected,
		"name":      f.Name,
	}

	if f.ChipType == "Custom" {
		if f.CustomChip != "" {
			req["chip_type"] = f.CustomChip
		}
	} else if f.ChipType != "" {
		req["chip_type"] = f.ChipType
	}

	return req
}

// isValidMAC validates a MAC address in AA:BB:CC:DD:EE:FF format. Moved out of
// devices.go so it can be tested on the host (it is pure logic).
func isValidMAC(mac string) bool {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}
	for _, part := range parts {
		if len(part) != 2 {
			return false
		}
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// parseFlashSize parses a flash size string to an integer, stopping at the
// first non-digit (e.g. "4096KiB" -> 4096).
func parseFlashSize(s string) int {
	size := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			size = size*10 + int(c-'0')
		} else {
			break
		}
	}
	return size
}

// splitString splits s on sep, ignoring empty fields. Kept in this host-available
// file (moved out of the js-only devices.go) so the request builders above can
// use it while still compiling under any GOOS for host-side unit testing.
func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	parts := []string{}
	current := ""
	for _, c := range s {
		if string(c) == sep {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
