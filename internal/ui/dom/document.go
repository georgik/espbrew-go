// Package dom provides Go wrappers for DOM manipulation
// Designed for Go WASM (syscall/js) usage
//
//go:build js
// +build js

package dom

import (
	"syscall/js"
)

// Document wraps js.Value for document operations
type Document struct {
	value js.Value
}

// GlobalDocument returns the global document object
func GlobalDocument() *Document {
	doc := js.Global().Get("document")
	if doc.IsUndefined() || doc.IsNull() {
		return nil
	}
	return &Document{
		value: doc,
	}
}

// Value returns the underlying js.Value
func (d *Document) Value() js.Value {
	if d == nil {
		return js.Undefined()
	}
	return d.value
}

// GetElementByID returns an element by its ID
func (d *Document) GetElementByID(id string) *Element {
	if d == nil {
		return nil
	}
	v := d.value.Call("getElementById", id)
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// QuerySelector returns the first element matching the selector
func (d *Document) QuerySelector(selector string) *Element {
	if d == nil {
		return nil
	}
	v := d.value.Call("querySelector", selector)
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// QuerySelectorAll returns all elements matching the selector
func (d *Document) QuerySelectorAll(selector string) []*Element {
	if d == nil {
		return nil
	}
	values := d.value.Call("querySelectorAll", selector)
	if values.IsUndefined() || values.IsNull() {
		return nil
	}

	length := values.Get("length").Int()
	elems := make([]*Element, length)
	for i := 0; i < length; i++ {
		elems[i] = &Element{value: values.Index(i)}
	}
	return elems
}

// CreateElement creates a new element with the given tag name
func (d *Document) CreateElement(tag string) *Element {
	if d == nil {
		return nil
	}
	v := d.value.Call("createElement", tag)
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// CreateTextNode creates a new text node
func (d *Document) CreateTextNode(text string) *Element {
	if d == nil {
		return nil
	}
	v := d.value.Call("createTextNode", text)
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// GetBody returns the body element
func (d *Document) GetBody() *Element {
	if d == nil {
		return nil
	}
	v := d.value.Get("body")
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// GetHead returns the head element
func (d *Document) GetHead() *Element {
	if d == nil {
		return nil
	}
	v := d.value.Get("head")
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// GetTitle returns the document title
func (d *Document) GetTitle() string {
	if d == nil {
		return ""
	}
	return d.value.Get("title").String()
}

// SetTitle sets the document title
func (d *Document) SetTitle(title string) {
	if d == nil {
		return
	}
	d.value.Set("title", title)
}

// Ready returns true if the DOM is ready
func (d *Document) Ready() bool {
	if d == nil {
		return false
	}
	state := d.value.Get("readyState").String()
	return state == "complete" || state == "interactive"
}

// OnDOMContentLoaded registers a callback for DOMContentLoaded event
func (d *Document) OnDOMContentLoaded(callback func()) {
	if d == nil {
		return
	}

	if d.Ready() {
		// Already ready, call immediately
		callback()
		return
	}

	handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		callback()
		return nil
	})
	d.value.Call("addEventListener", "DOMContentLoaded", handler)
}
