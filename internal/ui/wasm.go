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

	// Check connection status (skip in demo mode)
	if !api.DemoModeEnabled() {
		checkConnection()
	} else {
		// In demo mode, show as connected
		app.UpdateConnectionStatus(true)
	}

	// Load initial page (dashboard)
	pages.Load("dashboard")

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
