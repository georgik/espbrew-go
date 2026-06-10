//go:build js
// +build js

package pages

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/layout"
)

var (
	app *layout.App
)

// PageFunc defines a page renderer function
type PageFunc func(app *layout.App)

// PageInitFunc defines a page initialization function
type PageInitFunc func()

// getRoutes returns the routes map (lazy initialization to avoid cycle)
func getRoutes() map[string]PageFunc {
	return map[string]PageFunc{
		"dashboard": Dashboard,
		"capture":   Capture,
		"cameras":   CameraProperties,
		"devices":   Devices,
		"monitor":   Monitor,
		"gallery":   Gallery,
		"mapping":   Mapping,
		"flash":     Flash,
		"settings":  Settings,
	}
}

// getInitFuncs returns initialization functions for each page
func getInitFuncs() map[string]PageInitFunc {
	return map[string]PageInitFunc{
		"capture":  initCapturePage,
		"cameras":  initCameraPropertiesPage,
		"devices":  initDevicesPage,
		"monitor":  initMonitorPage,
		"mapping":  initMappingPage,
		"gallery":  initGalleryPage,
		"flash":    initFlashPage,
	}
}

// Init initializes the page router with the app instance
func Init(a *layout.App) {
	app = a
}

// Load loads a page by name
func Load(name string) {
	routes := getRoutes()
	if page, ok := routes[name]; ok {
		page(app)

		// Call page initialization if available
		if initFn, ok := getInitFuncs()[name]; ok {
			initFn()
		}
	} else {
		// Default to dashboard for unknown pages
		Dashboard(app)
	}
}

// NavigateTo changes the current page
func NavigateTo(name string) {
	// Update tab bar if available
	if tabbar := app.GetTabBar(); tabbar != nil {
		tabbar.ActivateTab(name)
	}
	// Load the page
	Load(name)
}

// Register is not implemented with lazy routes
// Use explicit routing instead
func Register(name string, page PageFunc) {
	// This would require a persistent routes map
	// For now, routes are defined in getRoutes()
}

// HandleNavigation handles navigation clicks from the tab bar
func HandleNavigation(tabName string) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		// Prevent default behavior
		if len(args) > 0 {
			event := args[0]
			event.Call("preventDefault")
		}
		// Navigate to the page
		NavigateTo(tabName)
		return nil
	})
}
