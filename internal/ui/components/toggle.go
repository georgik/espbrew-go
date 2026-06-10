//go:build js
// +build js

package components

import (
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// Toggle represents a boolean toggle switch
type Toggle struct {
	*dom.Element
	toggleSwitch *dom.Element
}

// ToggleConfig configures a toggle component
type ToggleConfig struct {
	ID       string
	Label    string
	Checked  bool
	Disabled bool
	OnChange func(checked bool)
}

// NewToggle creates a new toggle component
func NewToggle(config ToggleConfig) *Toggle {
	doc := dom.GlobalDocument()

	toggle := &Toggle{
		Element: doc.CreateElement("div"),
	}

	toggle.SetClass("toggle-container")
	if config.ID != "" {
		toggle.SetID(config.ID)
	}

	// Create label
	label := doc.CreateElement("div")
	label.SetClass("toggle-label")
	label.SetTextContent(config.Label)
	toggle.Append(label)

	// Create switch
	toggle.toggleSwitch = doc.CreateElement("div")
	toggle.toggleSwitch.SetClass("toggle-switch")
	if config.Checked {
		toggle.toggleSwitch.AddClass("active")
	}

	if config.Disabled {
		toggle.toggleSwitch.SetStyle("opacity", "0.5")
		toggle.toggleSwitch.SetStyle("pointer-events", "none")
	}

	toggle.Append(toggle.toggleSwitch)

	// Set up event listener
	if config.OnChange != nil && !config.Disabled {
		onChange := config.OnChange
		toggle.toggleSwitch.AddEventListener("click", func(_ *dom.Event) {
			isChecked := toggle.toggleSwitch.HasClass("active")
			if isChecked {
				toggle.toggleSwitch.RemoveClass("active")
				onChange(false)
			} else {
				toggle.toggleSwitch.AddClass("active")
				onChange(true)
			}
		})
	}

	return toggle
}

// SetChecked updates the toggle state
func (t *Toggle) SetChecked(checked bool) {
	if t.toggleSwitch == nil {
		return
	}
	if checked {
		t.toggleSwitch.AddClass("active")
	} else {
		t.toggleSwitch.RemoveClass("active")
	}
}

// IsChecked returns the current toggle state
func (t *Toggle) IsChecked() bool {
	if t.toggleSwitch == nil {
		return false
	}
	return t.toggleSwitch.HasClass("active")
}

// SetDisabled enables or disables the toggle
func (t *Toggle) SetDisabled(disabled bool) {
	if t.toggleSwitch == nil {
		return
	}
	if disabled {
		t.toggleSwitch.SetStyle("opacity", "0.5")
		t.toggleSwitch.SetStyle("pointer-events", "none")
	} else {
		t.toggleSwitch.SetStyle("opacity", "1")
		t.toggleSwitch.SetStyle("pointer-events", "auto")
	}
}
