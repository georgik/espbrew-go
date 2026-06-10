//go:build js
// +build js

package components

import (
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// Slider represents a range input slider with value display
type Slider struct {
	*dom.Element
	input *dom.Element
	value *dom.Element
}

// SliderConfig configures a slider component
type SliderConfig struct {
	ID       string
	Label    string
	Min      int32
	Max      int32
	Value    int32
	Step     int32
	Disabled bool
	OnChange func(value int32)
}

// NewSlider creates a new slider component
func NewSlider(config SliderConfig) *Slider {
	doc := dom.GlobalDocument()

	slider := &Slider{
		Element: doc.CreateElement("div"),
	}

	slider.SetClass("slider-container")
	if config.ID != "" {
		slider.SetID(config.ID)
	}

	// Create label
	label := doc.CreateElement("div")
	label.SetClass("slider-label")
	label.SetTextContent(config.Label)
	slider.Append(label)

	// Create input wrapper for flex layout
	inputWrapper := doc.CreateElement("div")
	inputWrapper.SetStyle("flex", "1")
	inputWrapper.SetStyle("display", "flex")
	inputWrapper.SetStyle("align-items", "center")

	// Create range input
	slider.input = doc.CreateElement("input")
	slider.input.SetAttribute("type", "range")
	slider.input.SetAttribute("min", formatInt32(config.Min))
	slider.input.SetAttribute("max", formatInt32(config.Max))
	slider.input.SetAttribute("value", formatInt32(config.Value))
	slider.input.SetAttribute("step", formatInt32(config.Step))
	slider.input.SetClass("slider-input")

	if config.Disabled {
		slider.input.SetAttribute("disabled", "true")
	}

	inputWrapper.Append(slider.input)
	slider.Append(inputWrapper)

	// Create value display
	slider.value = doc.CreateElement("div")
	slider.value.SetClass("slider-value")
	slider.value.SetTextContent(formatInt32(config.Value))
	slider.Append(slider.value)

	// Set up event listener
	if config.OnChange != nil && !config.Disabled {
		onChange := config.OnChange
		slider.input.AddEventListener("input", func(_ *dom.Event) {
			newValue := int32(slider.input.Value().Int())
			slider.value.SetTextContent(formatInt32(newValue))
			onChange(newValue)
		})
	}

	return slider
}

// SetValue updates the slider value
func (s *Slider) SetValue(value int32) {
	if s.input != nil {
		s.input.SetAttribute("value", formatInt32(value))
	}
	if s.value != nil {
		s.value.SetTextContent(formatInt32(value))
	}
}

// GetValue returns the current slider value
func (s *Slider) GetValue() int32 {
	if s.input == nil {
		return 0
	}
	return int32(s.input.Value().Int())
}

// SetDisabled enables or disables the slider
func (s *Slider) SetDisabled(disabled bool) {
	if s.input != nil {
		if disabled {
			s.input.SetAttribute("disabled", "true")
		} else {
			s.input.RemoveAttribute("disabled")
		}
	}
}

// formatInt32 converts an int32 to a string
func formatInt32(n int32) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	if n < 100 {
		tens := n / 10
		ones := n % 10
		return string(rune('0'+tens)) + string(rune('0'+ones))
	}
	result := ""
	for n > 0 {
		digit := n % 10
		result = string(rune('0'+digit)) + result
		n /= 10
	}
	if result == "" {
		return "0"
	}
	return result
}
