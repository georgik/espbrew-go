//go:build js
// +build js

package components

import (
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// Button represents a button element
type Button struct {
	*dom.Element
	onClick func(*dom.Event)
}

// ButtonConfig configures a button
type ButtonConfig struct {
	Text     string
	Class    string
	Disabled bool
	OnClick  func(*dom.Event)
}

// NewButton creates a new button
func NewButton(config ButtonConfig) *Button {
	btn := &Button{
		Element: dom.GlobalDocument().CreateElement("button"),
		onClick: config.OnClick,
	}

	btn.SetTextContent(config.Text)
	btn.SetClass("btn " + config.Class)

	if config.Disabled {
		btn.SetAttribute("disabled", "true")
	}

	if config.OnClick != nil {
		btn.AddEventListener(dom.EventClick, config.OnClick)
	}

	return btn
}

// SetText updates the button text
func (b *Button) SetText(text string) {
	b.SetTextContent(text)
}

// SetDisabled updates the disabled state
func (b *Button) SetDisabled(disabled bool) {
	if disabled {
		b.SetAttribute("disabled", "true")
	} else {
		b.RemoveAttribute("disabled")
	}
}

// GetDisabled returns the disabled state
func (b *Button) GetDisabled() bool {
	return b.GetAttribute("disabled") == "true"
}

// Enable enables the button
func (b *Button) Enable() {
	b.SetDisabled(false)
}

// Disable disables the button
func (b *Button) Disable() {
	b.SetDisabled(true)
}
