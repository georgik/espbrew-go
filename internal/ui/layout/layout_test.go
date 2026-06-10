//go:build js
// +build js

package layout

import (
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/ui/dom"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewApp tests app creation
func TestNewApp(t *testing.T) {
	// Create a container for testing
	container := dom.GlobalDocument().CreateElement("div")
	container.SetID("test-app")
	dom.GlobalDocument().GetBody().Append(container)
	defer container.Remove()

	app := NewApp()

	require.NotNil(t, app)
	assert.NotNil(t, app.Container)
	assert.NotNil(t, app.Navbar)
	assert.NotNil(t, app.TabBar)
	assert.NotNil(t, app.Main)
	assert.Len(t, app.Tabs, len(tabs))
}

// TestAppSetMainContent tests setting main content
func TestAppSetMainContent(t *testing.T) {
	container := dom.GlobalDocument().CreateElement("div")
	container.SetID("test-app-content")
	dom.GlobalDocument().GetBody().Append(container)
	defer container.Remove()

	app := NewApp()

	content := dom.GlobalDocument().CreateElement("div")
	content.SetTextContent("Test content")

	app.SetMainContent(content)

	mainContent := app.GetMainContent()
	require.NotNil(t, mainContent)
	assert.Contains(t, mainContent.GetInnerHTML(), "Test content")
}

// TestNavbar tests navbar creation
func TestNewNavbar(t *testing.T) {
	navbar := NewNavbar()

	require.NotNil(t, navbar)
	assert.True(t, navbar.HasClass("navbar"))

	title := navbar.QuerySelector(".navbar-title")
	require.NotNil(t, title)
	assert.Equal(t, "ESPBrew V2", title.GetTextContent())
}

// TestNavbarUpdateConnectionStatus tests connection status update
func TestNavbarUpdateConnectionStatus(t *testing.T) {
	navbar := NewNavbar()

	// Test connected state
	navbar.UpdateConnectionStatus(true)
	status := navbar.QuerySelector("#connection-status")
	require.NotNil(t, status)
	assert.True(t, status.HasClass("online"))
	assert.Equal(t, "Connected", status.GetTextContent())

	// Test disconnected state
	navbar.UpdateConnectionStatus(false)
	assert.True(t, status.HasClass("offline"))
	assert.Equal(t, "Disconnected", status.GetTextContent())
}

// TestTabBar tests tab bar creation
func TestNewTabBar(t *testing.T) {
	tabBar := NewTabBar()

	require.NotNil(t, tabBar)
	assert.True(t, tabBar.HasClass("tabbar"))
	assert.Len(t, tabBar.Tabs, len(tabs))

	// Verify each tab has correct attributes
	for i, tab := range tabBar.Tabs {
		assert.True(t, tab.HasClass("nav-tab"))
		assert.Equal(t, tabs[i].ID, tab.GetAttribute("data-page"))
		assert.Equal(t, tabs[i].Name, tab.GetTextContent())
	}
}

// TestTabBarActivateTab tests tab activation
func TestTabBarActivateTab(t *testing.T) {
	tabBar := NewTabBar()

	// Activate first tab
	tabBar.ActivateTab("dashboard")
	assert.Equal(t, "dashboard", tabBar.GetActiveTab())

	// Activate different tab
	tabBar.ActivateTab("capture")
	assert.Equal(t, "capture", tabBar.GetActiveTab())

	// Verify only one tab is active
	activeCount := 0
	for _, tab := range tabBar.Tabs {
		if tab.HasClass("active") {
			activeCount++
		}
	}
	assert.Equal(t, 1, activeCount)
}
