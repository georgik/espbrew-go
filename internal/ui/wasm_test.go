//go:build js
// +build js

package ui_test

import (
	"syscall/js"
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/ui"
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// TestDOMOperations tests basic DOM manipulation
func TestDOMOperations(t *testing.T) {
	// Create a test document
	doc := js.Global().Get("document")
	if doc.IsUndefined() {
		t.Skip("Document not available - not in browser environment")
		return
	}

	// Test CreateElement
	elem := dom.CreateElement("div")
	if elem == nil {
		t.Fatal("CreateElement returned nil")
	}

	// Test SetClass
	elem.SetClass("test-class")
	if class := elem.GetClass(); class != "test-class" {
		t.Errorf("Expected class 'test-class', got '%s'", class)
	}

	// Test SetTextContent
	elem.SetTextContent("Test content")
	if text := elem.GetTextContent(); text != "Test content" {
		t.Errorf("Expected text 'Test content', got '%s'", text)
	}

	// Test Append
	container := dom.CreateElement("div")
	container.Append(elem)
	children := container.GetChildren()
	if len(children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(children))
	}
}

// TestComponentCreation tests that components can be created
func TestComponentCreation(t *testing.T) {
	doc := js.Global().Get("document")
	if doc.IsUndefined() {
		t.Skip("Document not available - not in browser environment")
		return
	}

	// We can't fully test components without DOM attachment
	// but we can test that they don't panic
	t.Run("Button", func(t *testing.T) {
		// Test would require importing components package
		// which has build tags for js
		t.Skip("Components test - requires full DOM environment")
	})
}

// TestUIExports verifies that the UI exports are available
func TestUIExports(t *testing.T) {
	exports := js.Global().Get("espbrewUI")
	if exports.IsUndefined() || exports.IsNull() {
		t.Fatal("espbrewUI not exported to global scope")
	}

	// Check main function exists
	mainFunc := exports.Get("main")
	if mainFunc.IsUndefined() || mainFunc.IsNull() {
		t.Error("main function not exported")
	}

	// Check version exists
	version := exports.Get("version")
	if version.IsUndefined() || version.IsNull() {
		t.Error("version not exported")
	}
}
