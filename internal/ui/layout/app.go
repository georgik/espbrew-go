//go:build js
// +build js

package layout

import (
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// ContentFunc is a function that renders page content
type ContentFunc func() *dom.Element

// App is the main application shell
type App struct {
	Container   *dom.Element
	Navbar      *Navbar
	TabBar      *TabBar
	Main        *dom.Element
	Tabs        []*dom.Element
	contentFunc ContentFunc
}

// NewApp creates the app shell
func NewApp() *App {
	container := dom.GlobalDocument().GetElementByID("app")
	if container == nil {
		container = dom.GlobalDocument().CreateElement("div")
		container.SetID("app")
		dom.GlobalDocument().GetBody().Append(container)
	}

	app := &App{
		Container: container,
	}

	app.createStructure()
	return app
}

func (a *App) createStructure() {
	a.Container.RemoveChildren()

	// Create header
	a.Navbar = NewNavbar()
	a.Container.Append(a.Navbar.Element)

	// Create tabs
	a.TabBar = NewTabBar()
	a.Tabs = a.TabBar.Tabs
	a.Container.Append(a.TabBar.Element)

	// Create main content area
	a.Main = dom.GlobalDocument().CreateElement("div")
	a.Main.SetClass("main-content")
	a.Container.Append(a.Main)
}

// Render renders the app (already rendered in createStructure)
func (a *App) Render() {
	// App is already rendered
}

// SetMainContent sets the main content area with an element
func (a *App) SetMainContent(content *dom.Element) {
	if a.Main == nil {
		return
	}
	a.Main.RemoveChildren()
	if content != nil {
		a.Main.Append(content)
	}
}

// SetMainContentFunc sets a function that renders the main content
func (a *App) SetMainContentFunc(fn ContentFunc) {
	a.contentFunc = fn
	a.RefreshMainContent()
}

// RefreshMainContent re-renders the main content using the content function
func (a *App) RefreshMainContent() {
	if a.contentFunc == nil {
		return
	}
	content := a.contentFunc()
	a.SetMainContent(content)
}

// GetMainContent returns the main content element
func (a *App) GetMainContent() *dom.Element {
	return a.Main
}

// ShowPage shows a page by setting main content
func (a *App) ShowPage(content *dom.Element) {
	a.SetMainContent(content)
}

// UpdateConnectionStatus updates the connection status badge
func (a *App) UpdateConnectionStatus(connected bool) {
	if a.Navbar == nil {
		return
	}
	a.Navbar.UpdateConnectionStatus(connected)
}

// SetTitle sets the page title (displayed in navbar)
func (a *App) SetTitle(title string) {
	if a.Navbar == nil {
		return
	}
	a.Navbar.SetTitle(title)
}

// GetTabBar returns the tab bar instance
func (a *App) GetTabBar() *TabBar {
	return a.TabBar
}

// NavigateTo navigates to a page by name
func (a *App) NavigateTo(pageID string) {
	if a.TabBar != nil {
		a.TabBar.ActivateTab(pageID)
	}
	// The page router will handle actual content loading
	// via HandleNavigation callback
}
