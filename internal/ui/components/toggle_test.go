//go:build js
// +build js

package components

import (
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

func TestNewToggle(t *testing.T) {
	doc := dom.GlobalDocument()

	config := ToggleConfig{
		ID:       "test-toggle",
		Label:    "Auto Exposure",
		Checked:  false,
		Disabled: false,
	}

	toggle := NewToggle(config)
	if toggle == nil {
		t.Fatal("NewToggle returned nil")
	}

	if toggle.Element == nil {
		t.Fatal("Toggle element is nil")
	}

	if toggle.GetID() != "test-toggle" {
		t.Errorf("Expected ID 'test-toggle', got '%s'", toggle.GetID())
	}

	if !toggle.HasClass("toggle-container") {
		t.Error("Toggle missing toggle-container class")
	}
}

func TestToggleSetChecked(t *testing.T) {
	config := ToggleConfig{
		Label:   "Test",
		Checked: false,
	}

	toggle := NewToggle(config)

	if toggle.IsChecked() {
		t.Error("New toggle should be unchecked")
	}

	toggle.SetChecked(true)

	if !toggle.IsChecked() {
		t.Error("Toggle should be checked after SetChecked(true)")
	}

	toggle.SetChecked(false)

	if toggle.IsChecked() {
		t.Error("Toggle should be unchecked after SetChecked(false)")
	}
}

func TestToggleIsChecked(t *testing.T) {
	config := ToggleConfig{
		Label:   "Test",
		Checked: true,
	}

	toggle := NewToggle(config)

	if !toggle.IsChecked() {
		t.Error("New toggle with Checked=true should be checked")
	}
}

func TestToggleSetDisabled(t *testing.T) {
	config := ToggleConfig{
		Label:    "Test",
		Checked:  false,
		Disabled: false,
	}

	toggle := NewToggle(config)

	toggle.SetDisabled(true)
	if toggle.switch == nil {
		t.Fatal("Toggle switch is nil")
	}

	opacity := toggle.switch.GetStyle("opacity")
	if opacity != "0.5" {
		t.Errorf("Expected opacity 0.5, got %s", opacity)
	}

	pointerEvents := toggle.switch.GetStyle("pointer-events")
	if pointerEvents != "none" {
		t.Errorf("Expected pointer-events none, got %s", pointerEvents)
	}

	toggle.SetDisabled(false)

	opacity = toggle.switch.GetStyle("opacity")
	if opacity != "1" {
		t.Errorf("Expected opacity 1, got %s", opacity)
	}

	pointerEvents = toggle.switch.GetStyle("pointer-events")
	if pointerEvents != "auto" {
		t.Errorf("Expected pointer-events auto, got %s", pointerEvents)
	}
}

func TestToggleInitialState(t *testing.T) {
	config := ToggleConfig{
		Label:   "Test",
		Checked: true,
	}

	toggle := NewToggle(config)

	if !toggle.IsChecked() {
		t.Error("Toggle with Checked=true should start checked")
	}

	if !toggle.switch.HasClass("active") {
		t.Error("Checked toggle should have active class")
	}
}
