//go:build js
// +build js

package dom

import (
	"syscall/js"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDocument tests Document operations
func TestDocument(t *testing.T) {
	// Skip if not in browser/WASM environment
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	require.NotNil(t, doc, "GlobalDocument should return document")

	// Test GetTitle
	title := doc.GetTitle()
	assert.NotEmpty(t, title, "Document should have a title")

	// Test SetTitle
	newTitle := "Test Title"
	doc.SetTitle(newTitle)
	assert.Equal(t, newTitle, doc.GetTitle())

	// Restore original title
	doc.SetTitle(title)
}

// TestDocumentReady tests document ready state
func TestDocumentReady(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	require.NotNil(t, doc)

	// Document should be ready in test environment
	assert.True(t, doc.Ready(), "Document should be ready")
}

// TestDocumentCreateElement tests element creation
func TestDocumentCreateElement(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	require.NotNil(t, doc)

	// Test CreateElement
	div := doc.CreateElement("div")
	require.NotNil(t, div, "Should create div element")
	assert.Equal(t, "DIV", div.GetTagName())

	// Test CreateTextNode
	text := doc.CreateTextNode("test text")
	require.NotNil(t, text, "Should create text node")
}

// TestElementAttributes tests attribute operations
func TestElementAttributes(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	elem := doc.CreateElement("div")
	require.NotNil(t, elem)

	// Test SetAttribute/GetAttribute
	elem.SetAttribute("id", "test-id")
	assert.Equal(t, "test-id", elem.GetAttribute("id"))
	assert.True(t, elem.HasAttribute("id"))

	// Test RemoveAttribute
	elem.RemoveAttribute("id")
	assert.Empty(t, elem.GetAttribute("id"))
	assert.False(t, elem.HasAttribute("id"))
}

// TestElementClasses tests class operations
func TestElementClasses(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	elem := doc.CreateElement("div")
	require.NotNil(t, elem)

	// Test AddClass/HasClass
	elem.AddClass("test-class")
	assert.True(t, elem.HasClass("test-class"))

	// Test SetClass
	elem.SetClass("another-class")
	assert.False(t, elem.HasClass("test-class"))
	assert.True(t, elem.HasClass("another-class"))

	// Test ToggleClass
	result := elem.ToggleClass("toggle-class")
	assert.True(t, result, "First toggle should add class")
	assert.True(t, elem.HasClass("toggle-class"))

	result = elem.ToggleClass("toggle-class")
	assert.False(t, result, "Second toggle should remove class")
	assert.False(t, elem.HasClass("toggle-class"))

	// Test RemoveClass
	elem.RemoveClass("another-class")
	assert.False(t, elem.HasClass("another-class"))
}

// TestElementContent tests content operations
func TestElementContent(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	elem := doc.CreateElement("div")
	require.NotNil(t, elem)

	// Test SetTextContent/GetTextContent
	elem.SetTextContent("test content")
	assert.Equal(t, "test content", elem.GetTextContent())

	// Test SetInnerHTML/GetInnerHTML
	elem.SetInnerHTML("<span>inner</span>")
	assert.Contains(t, elem.GetInnerHTML(), "span")
}

// TestElementStructure tests structure operations
func TestElementStructure(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	parent := doc.CreateElement("div")
	child1 := doc.CreateElement("span")
	child2 := doc.CreateElement("span")

	require.NotNil(t, parent)
	require.NotNil(t, child1)
	require.NotNil(t, child2)

	// Test Append
	parent.Append(child1)
	assert.Equal(t, 1, parent.GetChildCount())
	assert.Equal(t, child1, parent.GetFirstChild())

	// Test Prepend
	parent.Prepend(child2)
	assert.Equal(t, 2, parent.GetChildCount())
	assert.Equal(t, child2, parent.GetFirstChild())

	// Test GetChildren
	children := parent.GetChildren()
	assert.Len(t, children, 2)

	// Test Remove
	child1.Remove()
	assert.Equal(t, 1, parent.GetChildCount())

	// Test RemoveChildren
	parent.RemoveChildren()
	assert.Equal(t, 0, parent.GetChildCount())
}

// TestElementQuery tests query operations
func TestElementQuery(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	parent := doc.CreateElement("div")
	child1 := doc.CreateElement("span")
	child2 := doc.CreateElement("div")

	child1.SetClass("test-class")
	parent.Append(child1)
	parent.Append(child2)

	require.NotNil(t, parent)

	// Test QuerySelector
	found := parent.QuerySelector(".test-class")
	assert.NotNil(t, found)
	assert.Equal(t, child1, found)

	// Test QuerySelectorAll
	all := parent.QuerySelectorAll("div")
	assert.Len(t, all, 1)
	assert.Equal(t, child2, all[0])
}

// TestElementEvents tests event operations
func TestElementEvents(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	elem := doc.CreateElement("button")
	require.NotNil(t, elem)

	clicked := false
	elem.AddEventListener(EventClick, func(e *Event) {
		clicked = true
	})

	// Trigger click (simulate event)
	// Note: In real WASM environment, you'd trigger actual DOM event
	// For unit tests, we might need a different approach
}

// TestElementStyle tests style operations
func TestElementStyle(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	elem := doc.CreateElement("div")
	require.NotNil(t, elem)

	// Test SetStyle/GetStyle
	elem.SetStyle("color", "red")
	assert.Equal(t, "red", elem.GetStyle("color"))

	// Test Show/Hide
	elem.Hide()
	assert.Equal(t, "none", elem.GetStyle("display"))

	elem.Show()
	// After show, display should be empty (default)
	assert.Empty(t, elem.GetStyle("display") != "none")
}

// TestElementForm tests form element operations
func TestElementForm(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()

	// Test input
	input := doc.CreateElement("input")
	require.NotNil(t, input)

	input.SetValue("test value")
	assert.Equal(t, "test value", input.GetValue())

	// Test checkbox
	checkbox := doc.CreateElement("input")
	checkbox.SetAttribute("type", "checkbox")

	checkbox.SetChecked(true)
	assert.True(t, checkbox.GetChecked())

	// Test disabled state
	input.SetDisabled(true)
	assert.True(t, input.GetDisabled())
}

// TestSelector tests selector operations
func TestSelector(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	sel := NewSelector(doc)

	// Create test elements
	div1 := doc.CreateElement("div")
	div1.SetID("test-id")
	div1.AddClass("test-class")

	div2 := doc.CreateElement("div")
	div2.AddClass("test-class")

	// Append to body for testing
	body := doc.GetBody()
	body.Append(div1)
	body.Append(div2)

	// Test ID
	found := sel.ID("test-id")
	assert.NotNil(t, found)
	assert.Equal(t, div1, found)

	// Test Class
	classElems := sel.ClassAll("test-class")
	assert.Len(t, classElems, 2)

	// Test Tag
	divs := sel.TagAll("div")
	assert.GreaterOrEqual(t, len(divs), 2)

	// Cleanup
	body.RemoveChildren()
}

// TestElementMatches tests selector matching
func TestElementMatches(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()
	elem := doc.CreateElement("div")
	elem.SetClass("test-class")

	// Test Matches
	assert.True(t, elem.Matches(".test-class"))
	assert.True(t, elem.Matches("div"))
	assert.False(t, elem.Matches("span"))
}

// TestHelpers tests helper functions
func TestHelpers(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()

	elem1 := doc.CreateElement("div")
	elem2 := doc.CreateElement("span")
	elem3 := doc.CreateElement("div")

	elems := []*Element{elem1, elem2, elem3}

	// Test First
	assert.Equal(t, elem1, First(elems))

	// Test Last
	assert.Equal(t, elem3, Last(elems))

	// Test Eq
	assert.Equal(t, elem2, Eq(elems, 1))
	assert.Nil(t, Eq(elems, 10))

	// Test Slice
	sliced := Slice(elems, 1, 2)
	assert.Len(t, sliced, 1)
	assert.Equal(t, elem2, sliced[0])

	// Test Filter
	elem1.AddClass("filter-me")
	elem3.AddClass("filter-me")
	filtered := Filter(elems, ".filter-me")
	assert.Len(t, filtered, 2)

	// Test Not
	notFiltered := Not(elems, ".filter-me")
	assert.Len(t, notFiltered, 1)

	// Test Index
	assert.Equal(t, 0, Index(elems, elem1))
	assert.Equal(t, 1, Index(elems, elem2))
	assert.Equal(t, -1, Index(elems, doc.CreateElement("p")))

	// Test Is
	assert.True(t, Is(elems, elem1))
	assert.False(t, Is(elems, doc.CreateElement("p")))

	// Test Add
	newElem := doc.CreateElement("a")
	updated := Add(elems, newElem)
	assert.Len(t, updated, 4)

	// Test NotIndex
	removed := NotIndex(updated, 2)
	assert.Len(t, removed, 3)
}

// TestEachAndMap tests Each and Map functions
func TestEachAndMap(t *testing.T) {
	if !js.Global().Truthy() {
		t.Skip("Not in JS environment")
	}

	doc := GlobalDocument()

	elem1 := doc.CreateElement("div")
	elem2 := doc.CreateElement("span")
	elem3 := doc.CreateElement("div")

	elems := []*Element{elem1, elem2, elem3}

	// Test Each
	sum := 0
	Each(elems, func(i int, elem *Element) {
		sum += i
	})
	assert.Equal(t, 3, sum) // 0 + 1 + 2 = 3

	// Test Map
	tags := Map(elems, func(i int, elem *Element) interface{} {
		return elem.GetTagName()
	})
	assert.Len(t, tags, 3)
	assert.Equal(t, "DIV", tags[0].(string))
	assert.Equal(t, "SPAN", tags[1].(string))
}
