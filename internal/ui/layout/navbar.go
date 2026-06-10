//go:build js
// +build js

package layout

import (
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// Navbar is the top navigation bar
type Navbar struct {
	*dom.Element
	status *dom.Element
}

// NewNavbar creates a navbar
func NewNavbar() *Navbar {
	nav := &Navbar{
		Element: dom.GlobalDocument().CreateElement("nav"),
	}

	nav.SetClass("navbar")

	// Logo/title
	title := dom.GlobalDocument().CreateElement("div")
	title.SetClass("navbar-title")
	title.SetTextContent("ESPBrew V2")
	nav.Append(title)

	// Connection status
	nav.status = dom.GlobalDocument().CreateElement("div")
	nav.status.SetID("connection-status")
	nav.status.SetClass("status-badge offline")
	nav.status.SetTextContent("Connecting...")
	nav.Append(nav.status)

	return nav
}

// UpdateConnectionStatus updates the connection status display
func (n *Navbar) UpdateConnectionStatus(connected bool) {
	if n.status == nil {
		return
	}

	n.status.RemoveClass("online")
	n.status.RemoveClass("offline")
	n.status.RemoveClass("connecting")

	if connected {
		n.status.AddClass("online")
		n.status.SetTextContent("Connected")
	} else {
		n.status.AddClass("offline")
		n.status.SetTextContent("Disconnected")
	}
}

// SetTitle sets the navbar title
func (n *Navbar) SetTitle(title string) {
	titleElem := n.QuerySelector(".navbar-title")
	if titleElem != nil {
		titleElem.SetTextContent(title)
	}
}
