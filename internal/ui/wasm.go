//go:build js
// +build js

package ui

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/api"
	"codeberg.org/georgik/espbrew-go/internal/ui/components"
	"codeberg.org/georgik/espbrew-go/internal/ui/layout"
	"codeberg.org/georgik/espbrew-go/internal/ui/pages"
)

var (
	app *layout.App
)

// Main is the WASM entry point called from JavaScript
func Main() {
	doc := js.Global().Get("document")
	if doc.IsUndefined() || doc.IsNull() {
		println("Error: document not available")
		return
	}

	// Check DOM ready state
	readyState := doc.Get("readyState").String()
	if readyState == "loading" {
		// Wait for DOMContentLoaded
		doc.Call("addEventListener", "DOMContentLoaded", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			initialize()
			return nil
		}))
	} else {
		// DOM already ready
		initialize()
	}
}

func initialize() {
	// Initialize demo mode detection FIRST (before any API calls)
	api.InitDemoMode()

	// Show demo banner if demo mode is active
	if api.DemoModeEnabled() {
		components.ShowDemoBanner()
		println("Demo mode enabled - using mock data")
	}

	// Create app shell
	app = layout.NewApp()

	// Initialize pages with app reference
	pages.Init(app)

	// Export navigation function for tab clicks
	exportAPI()

	// Set up hash-based routing
	setupHashRouting()

	// Check connection status (skip in demo mode)
	if !api.DemoModeEnabled() {
		checkConnection()
	} else {
		// In demo mode, show as connected
		app.UpdateConnectionStatus(true)
	}

	// Load initial page from hash or default to dashboard
	loadPageFromHash()

	mode := "live"
	if api.DemoModeEnabled() {
		mode = "demo"
	}
	println("ESPBrew V2 WASM UI initialized (" + mode + " mode)")
}

// exportAPI exports functions for JavaScript interop
func exportAPI() {
	exports := js.Global().Get("espbrewUI")
	if exports.IsUndefined() || exports.IsNull() {
		exports = js.Global().Get("Object").Call("create")
		js.Global().Set("espbrewUI", exports)
	}

	exports.Set("navigateTo", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			pageID := args[0].String()
			pages.NavigateTo(pageID)
		}
		return nil
	}))
}

// setupHashRouting sets up hash-based routing for deeplinking
func setupHashRouting() {
	window := js.Global().Get("window")
	document := js.Global().Get("document")

	// Listen for hash changes (back/forward button)
	window.Call("addEventListener", "hashchange", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		loadPageFromHash()
		return nil
	}))

	// Intercept clicks on links with hash anchors
	document.Call("addEventListener", "click", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			event := args[0]
			target := event.Get("target")

			// Check if clicked element or its parent is an anchor with hash
			for {
				if target.IsUndefined() || target.IsNull() {
					break
				}
				tagName := target.Get("tagName").String()
				if tagName == "A" {
					href := target.Get("href").String()
					if len(href) > 0 && href[0] == '#' {
						// Let the hashchange handler deal with it
						return nil
					}
					break
				}
				parent := target.Get("parentNode")
				if parent.IsUndefined() || parent.Equal(document) {
					break
				}
				target = parent
			}
		}
		return nil
	}))
}

// loadPageFromHash loads the page based on the current URL hash
func loadPageFromHash() {
	hash := js.Global().Get("window").Get("location").Get("hash").String()

	// Parse hash - should be #/pageName
	var pageID string
	if len(hash) > 2 && hash[0:2] == "#/" {
		pageID = hash[2:]
	} else {
		pageID = "dashboard"
	}

	// Validate page ID exists
	validPages := map[string]bool{
		"dashboard": true,
		"capture":   true,
		"gallery":   true,
		"cameras":   true,
		"mapping":   true,
		"devices":   true,
		"flash":     true,
		"monitor":   true,
		"settings":  true,
	}

	if !validPages[pageID] {
		pageID = "dashboard"
	}

	// Load the page
	pages.Load(pageID)

	// Update tab bar without triggering another navigation
	if tabbar := app.GetTabBar(); tabbar != nil {
		tabbar.ActivateTab(pageID)
	}
}

// checkConnection verifies API connection and updates status
func checkConnection() {
	api.GetCameras(func(cameras []api.Camera, err error) {
		if err == nil {
			app.UpdateConnectionStatus(true)
		} else {
			app.UpdateConnectionStatus(false)
		}
	})
}
