//go:build js
// +build js

package layout

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// Tab defines a navigation tab
type Tab struct {
	ID   string
	Name string
}

// Default tabs for the application
var tabs = []Tab{
	{ID: "dashboard", Name: "Dashboard"},
	{ID: "capture", Name: "Capture"},
	{ID: "cameras", Name: "Cameras"},
	{ID: "devices", Name: "Devices"},
	{ID: "monitor", Name: "Monitor"},
	{ID: "gallery", Name: "Gallery"},
	{ID: "mapping", Name: "Device Mapping"},
	{ID: "flash", Name: "Flash"},
	{ID: "settings", Name: "Settings"},
}

// TabBar is the tab navigation bar
type TabBar struct {
	*dom.Element
	Tabs []*dom.Element
}

// NewTabBar creates a tab bar
func NewTabBar() *TabBar {
	bar := &TabBar{
		Element: dom.GlobalDocument().CreateElement("div"),
		Tabs:    make([]*dom.Element, 0, len(tabs)),
	}

	bar.SetClass("tabbar")

	for _, tab := range tabs {
		tabElem := dom.GlobalDocument().CreateElement("button")
		tabElem.SetClass("nav-tab")
		tabElem.SetAttribute("data-page", tab.ID)
		tabElem.SetTextContent(tab.Name)

		// Add click handler for navigation
		tabID := tab.ID
		tabElem.AddEventListener("click", func(_ *dom.Event) {
			handleTabClick(tabID)
		})

		bar.Append(tabElem)
		bar.Tabs = append(bar.Tabs, tabElem)
	}

	return bar
}

// handleTabClick handles tab click events
func handleTabClick(tabID string) {
	// Navigate to the page
	// This will be intercepted by the router's NavigateTo function
	js.Global().Get("espbrewUI").Call("navigateTo", tabID)
}

// ActivateTab highlights the specified tab
func (t *TabBar) ActivateTab(pageID string) {
	for _, tab := range t.Tabs {
		if tab.GetAttribute("data-page") == pageID {
			tab.AddClass("active")
		} else {
			tab.RemoveClass("active")
		}
	}
}

// GetActiveTab returns the currently active tab ID
func (t *TabBar) GetActiveTab() string {
	for _, tab := range t.Tabs {
		if tab.HasClass("active") {
			return tab.GetAttribute("data-page")
		}
	}
	return ""
}
