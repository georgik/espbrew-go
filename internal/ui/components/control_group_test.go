//go:build js
// +build js

package components

import (
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

func TestNewControlGroup(t *testing.T) {
	doc := dom.GlobalDocument()

	config := ControlGroupConfig{
		ID:    "test-group",
		Title: "Image Controls",
	}

	group := NewControlGroup(config)
	if group == nil {
		t.Fatal("NewControlGroup returned nil")
	}

	if group.Element == nil {
		t.Fatal("ControlGroup element is nil")
	}

	if group.GetID() != "test-group" {
		t.Errorf("Expected ID 'test-group', got '%s'", group.GetID())
	}

	if !group.HasClass("control-group") {
		t.Error("ControlGroup missing control-group class")
	}

	if group.header == nil {
		t.Fatal("ControlGroup header is nil")
	}

	if group.body == nil {
		t.Fatal("ControlGroup body is nil")
	}
}

func TestControlGroupSetTitle(t *testing.T) {
	config := ControlGroupConfig{
		Title: "Original Title",
	}

	group := NewControlGroup(config)
	group.SetTitle("New Title")

	textContent := group.header.GetTextContent()
	if textContent != "New Title" {
		t.Errorf("Expected title 'New Title', got '%s'", textContent)
	}
}

func TestControlGroupAddControl(t *testing.T) {
	doc := dom.GlobalDocument()

	config := ControlGroupConfig{
		Title: "Test Group",
	}

	group := NewControlGroup(config)

	control := doc.CreateElement("div")
	control.SetTextContent("Test Control")

	initialChildCount := group.body.Get("childElementCount").Int()
	group.AddControl(control)

	newChildCount := group.body.Get("childElementCount").Int()
	if newChildCount != initialChildCount+1 {
		t.Errorf("Expected child count %d, got %d", initialChildCount+1, newChildCount)
	}
}

func TestControlGroupShow(t *testing.T) {
	config := ControlGroupConfig{
		Title: "Test Group",
	}

	group := NewControlGroup(config)
	group.Hide()

	group.Show()

	displayStyle := group.GetStyle("display")
	if displayStyle != "block" {
		t.Errorf("Expected display 'block', got '%s'", displayStyle)
	}
}

func TestControlGroupHide(t *testing.T) {
	config := ControlGroupConfig{
		Title: "Test Group",
	}

	group := NewControlGroup(config)

	group.Hide()

	displayStyle := group.GetStyle("display")
	if displayStyle != "none" {
		t.Errorf("Expected display 'none', got '%s'", displayStyle)
	}
}

func TestControlGroupRemove(t *testing.T) {
	config := ControlGroupConfig{
		Title: "Test Group",
	}

	group := NewControlGroup(config)

	parent := group.GetParentElement()
	if parent == nil {
		parent = dom.GlobalDocument().GetBody()
		parent.Append(group.Element)
	}

	group.Remove()

	if group.GetParentElement() != nil {
		t.Error("Removed control group should have no parent")
	}
}

func TestControlGroupWithControls(t *testing.T) {
	doc := dom.GlobalDocument()

	control1 := doc.CreateElement("div")
	control1.SetTextContent("Control 1")

	control2 := doc.CreateElement("div")
	control2.SetTextContent("Control 2")

	config := ControlGroupConfig{
		Title:    "Test Group",
		Controls: []*dom.Element{control1, control2},
	}

	group := NewControlGroup(config)

	childCount := group.body.Get("childElementCount").Int()
	if childCount != 2 {
		t.Errorf("Expected 2 controls, got %d", childCount)
	}
}
