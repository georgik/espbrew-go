//go:build js
// +build js

package components

import (
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// ControlGroup represents a group of related controls
type ControlGroup struct {
	*dom.Element
	header *dom.Element
	body   *dom.Element
}

// ControlGroupConfig configures a control group
type ControlGroupConfig struct {
	ID       string
	Title    string
	Controls []*dom.Element
}

// NewControlGroup creates a new control group
func NewControlGroup(config ControlGroupConfig) *ControlGroup {
	doc := dom.GlobalDocument()

	group := &ControlGroup{
		Element: doc.CreateElement("div"),
	}

	group.SetClass("control-group")
	if config.ID != "" {
		group.SetID(config.ID)
	}

	// Create header
	group.header = doc.CreateElement("div")
	group.header.SetClass("control-group-header")
	group.header.SetTextContent(config.Title)
	group.Append(group.header)

	// Create body
	group.body = doc.CreateElement("div")
	group.body.SetStyle("display", "flex")
	group.body.SetStyle("flex-direction", "column")
	group.body.SetStyle("gap", "12px")
	group.body.SetStyle("margin-top", "8px")

	// Add controls
	for _, control := range config.Controls {
		if control != nil {
			group.body.Append(control)
		}
	}

	group.Append(group.body)

	return group
}

// AddControl adds a control to the group
func (g *ControlGroup) AddControl(control *dom.Element) {
	if g.body != nil && control != nil {
		g.body.Append(control)
	}
}

// SetTitle updates the group title
func (g *ControlGroup) SetTitle(title string) {
	if g.header != nil {
		g.header.SetTextContent(title)
	}
}

// Remove removes the group from the DOM
func (g *ControlGroup) Remove() {
	if g.Element != nil {
		g.Element.Remove()
	}
}

// Show displays the control group
func (g *ControlGroup) Show() {
	if g.Element != nil {
		g.Element.SetStyle("display", "block")
	}
}

// Hide hides the control group
func (g *ControlGroup) Hide() {
	if g.Element != nil {
		g.Element.SetStyle("display", "none")
	}
}
