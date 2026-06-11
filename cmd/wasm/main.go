//go:build js
// +build js

package main

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui"
)

func main() {
	// Initialize the UI
	ui.Main()

	// Keep the Go program alive - don't let main() return
	// This allows async callbacks to execute
	select {}
}

// Export the main function for JavaScript access
func init() {
	// Create a JavaScript object to export
	obj := js.Global().Get("Object").Call("create", nil)
	obj.Set("main", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		ui.Main()
		return nil
	}))
	obj.Set("version", js.ValueOf("2.0.0-wasm"))

	// Export to global scope
	js.Global().Set("espbrewUI", obj)
}
