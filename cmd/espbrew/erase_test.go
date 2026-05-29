package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestEraseCommandExists(t *testing.T) {
	// Verify erase command is registered
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "erase" {
			found = true
			break
		}
	}
	if !found {
		t.Error("erase command not found")
	}
}

func TestEraseCommandFlags(t *testing.T) {
	// Check that erase command has required flags
	var eraseCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "erase" {
			eraseCmd = cmd
			break
		}
	}

	if eraseCmd == nil {
		t.Fatal("erase command not found")
	}

	// Check for required flags
	flagNames := []string{"cluster", "device", "port", "address", "size", "all"}
	for _, name := range flagNames {
		if flag := eraseCmd.Flags().Lookup(name); flag == nil {
			t.Errorf("flag %s not found on erase command", name)
		}
	}
}

func TestParseHex(t *testing.T) {
	tests := []struct {
		input    string
		expected uint32
		hasError bool
	}{
		{"0", 0, false},
		{"1000", 0x1000, false},
		{"10000", 0x10000, false},
		{"FFFFFFFF", 0xFFFFFFFF, false},
		{"abc", 0x0ABC, false},
		{"ABC", 0x0ABC, false},
		{"100000000", 0, true}, // Too large
		{"xyz", 0, true},       // Invalid hex
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseHex(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("expected error for input %s, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %s: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("for input %s, expected %x, got %x", tt.input, tt.expected, result)
				}
			}
		})
	}
}
