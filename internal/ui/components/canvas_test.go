//go:build js
// +build js

package components

import (
	"testing"
)

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{5, "5"},
		{9, "9"},
		{10, "10"},
		{11, "11"},
		{42, "42"},
		{99, "99"},
		{100, "100"},
		{123, "123"},
		{999, "999"},
		{1000, "1000"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatInt(tt.input)
			if result != tt.expected {
				t.Errorf("formatInt(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatIntEdgeCases(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{-1, "0"},   // Negative numbers should return 0
		{-100, "0"}, // Negative numbers should return 0
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatInt(tt.input)
			if result != tt.expected {
				t.Errorf("formatInt(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
