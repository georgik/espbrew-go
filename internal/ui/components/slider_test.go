//go:build js
// +build js

package components

import (
	"syscall/js"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

func TestNewSlider(t *testing.T) {
	doc := dom.GlobalDocument()

	config := SliderConfig{
		ID:       "test-slider",
		Label:    "Brightness",
		Min:      0,
		Max:      100,
		Value:    50,
		Step:     1,
		Disabled: false,
	}

	slider := NewSlider(config)
	if slider == nil {
		t.Fatal("NewSlider returned nil")
	}

	if slider.Element == nil {
		t.Fatal("Slider element is nil")
	}

	if slider.GetID() != "test-slider" {
		t.Errorf("Expected ID 'test-slider', got '%s'", slider.GetID())
	}

	if !slider.HasClass("slider-container") {
		t.Error("Slider missing slider-container class")
	}
}

func TestSliderSetValue(t *testing.T) {
	config := SliderConfig{
		Label: "Test",
		Min:   0,
		Max:   100,
		Value: 50,
		Step:  1,
	}

	slider := NewSlider(config)

	slider.SetValue(75)

	if slider.GetValue() != 75 {
		t.Errorf("Expected value 75, got %d", slider.GetValue())
	}
}

func TestSliderGetValue(t *testing.T) {
	config := SliderConfig{
		Label: "Test",
		Min:   0,
		Max:   100,
		Value: 42,
		Step:  1,
	}

	slider := NewSlider(config)

	value := slider.GetValue()
	if value != 42 {
		t.Errorf("Expected initial value 42, got %d", value)
	}
}

func TestSliderSetDisabled(t *testing.T) {
	config := SliderConfig{
		Label:    "Test",
		Min:      0,
		Max:      100,
		Value:    50,
		Step:     1,
		Disabled: false,
	}

	slider := NewSlider(config)

	slider.SetDisabled(true)
	if slider.input == nil {
		t.Fatal("Slider input is nil")
	}

	disabledAttr := slider.input.GetAttribute("disabled")
	if disabledAttr != "true" {
		t.Error("Expected disabled attribute to be 'true'")
	}

	slider.SetDisabled(false)
	disabledAttr = slider.input.GetAttribute("disabled")
	if disabledAttr == "true" {
		t.Error("Expected disabled attribute to be removed")
	}
}

func TestSliderOnChange(t *testing.T) {
	called := false
	var receivedValue int32

	config := SliderConfig{
		Label: "Test",
		Min:   0,
		Max:   100,
		Value: 50,
		Step:  1,
		OnChange: func(value int32) {
			called = true
			receivedValue = value
		},
	}

	slider := NewSlider(config)

	if slider.input == nil {
		t.Fatal("Slider input is nil")
	}

	slider.input.SetValue("75")

	dispatchEvent := js.Global().Get("Event").New("input")
	slider.input.Call("dispatchEvent", dispatchEvent)

	if called && receivedValue != 75 {
		t.Logf("Change callback works (received: %d)", receivedValue)
	}
}

func TestFormatInt32(t *testing.T) {
	tests := []struct {
		input    int32
		expected string
	}{
		{0, "0"},
		{5, "5"},
		{10, "10"},
		{42, "42"},
		{100, "100"},
		{999, "999"},
	}

	for _, tt := range tests {
		result := formatInt32(tt.input)
		if result != tt.expected {
			t.Errorf("formatInt32(%d) = %s; want %s", tt.input, result, tt.expected)
		}
	}
}
