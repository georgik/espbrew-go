//go:build js
// +build js

package dom

import (
	"syscall/js"
)

// Element wraps js.Value for DOM element operations
type Element struct {
	value js.Value
}

// Value returns the underlying js.Value
func (e *Element) Value() js.Value {
	if e == nil {
		return js.Undefined()
	}
	return e.value
}

// IsNil returns true if the element is nil or undefined
func (e *Element) IsNil() bool {
	if e == nil {
		return true
	}
	return e.value.IsUndefined() || e.value.IsNull()
}

// === Attributes ===

// SetAttribute sets an attribute on the element
func (e *Element) SetAttribute(name, value string) {
	if e == nil {
		return
	}
	e.value.Call("setAttribute", name, value)
}

// GetAttribute returns the value of an attribute
func (e *Element) GetAttribute(name string) string {
	if e == nil {
		return ""
	}
	return e.value.Call("getAttribute", name).String()
}

// RemoveAttribute removes an attribute
func (e *Element) RemoveAttribute(name string) {
	if e == nil {
		return
	}
	e.value.Call("removeAttribute", name)
}

// HasAttribute returns true if the element has the attribute
func (e *Element) HasAttribute(name string) bool {
	if e == nil {
		return false
	}
	return e.value.Call("hasAttribute", name).Bool()
}

// === ID and Class ===

// SetID sets the id attribute
func (e *Element) SetID(id string) {
	e.SetAttribute("id", id)
}

// GetID returns the id attribute
func (e *Element) GetID() string {
	return e.GetAttribute("id")
}

// SetClass sets the class attribute (replaces all classes)
func (e *Element) SetClass(class string) {
	e.SetAttribute("class", class)
}

// GetClass returns the class attribute
func (e *Element) GetClass() string {
	return e.GetAttribute("class")
}

// AddClass adds a class to the element
func (e *Element) AddClass(class string) {
	if e == nil {
		return
	}
	classes := e.value.Get("classList")
	classes.Call("add", class)
}

// RemoveClass removes a class from the element
func (e *Element) RemoveClass(class string) {
	if e == nil {
		return
	}
	classes := e.value.Get("classList")
	classes.Call("remove", class)
}

// ToggleClass toggles a class on the element
func (e *Element) ToggleClass(class string) bool {
	if e == nil {
		return false
	}
	classes := e.value.Get("classList")
	return classes.Call("toggle", class).Bool()
}

// HasClass returns true if the element has the class
func (e *Element) HasClass(class string) bool {
	if e == nil {
		return false
	}
	classes := e.value.Get("classList")
	return classes.Call("contains", class).Bool()
}

// === Content ===

// SetInnerHTML sets the inner HTML content
func (e *Element) SetInnerHTML(html string) {
	if e == nil {
		return
	}
	e.value.Set("innerHTML", html)
}

// GetInnerHTML returns the inner HTML content
func (e *Element) GetInnerHTML() string {
	if e == nil {
		return ""
	}
	return e.value.Get("innerHTML").String()
}

// SetTextContent sets the text content
func (e *Element) SetTextContent(text string) {
	if e == nil {
		return
	}
	e.value.Set("textContent", text)
}

// GetTextContent returns the text content
func (e *Element) GetTextContent() string {
	if e == nil {
		return ""
	}
	return e.value.Get("textContent").String()
}

// GetInnerText returns the inner text (rendered text)
func (e *Element) GetInnerText() string {
	if e == nil {
		return ""
	}
	return e.value.Get("innerText").String()
}

// === Structure ===

// Append appends a child element
func (e *Element) Append(child *Element) {
	if e == nil || child == nil {
		return
	}
	e.value.Call("appendChild", child.value)
}

// Prepend prepends a child element
func (e *Element) Prepend(child *Element) {
	if e == nil || child == nil {
		return
	}
	e.value.Call("prepend", child.value)
}

// InsertBefore inserts a new child before a reference child
func (e *Element) InsertBefore(newChild, refChild *Element) {
	if e == nil || newChild == nil || refChild == nil {
		return
	}
	e.value.Call("insertBefore", newChild.value, refChild.value)
}

// Remove removes the element from its parent
func (e *Element) Remove() {
	if e == nil {
		return
	}
	e.value.Call("remove")
}

// RemoveChildren removes all child elements
func (e *Element) RemoveChildren() {
	if e == nil {
		return
	}
	firstChild := e.value.Get("firstChild")
	for !firstChild.IsNull() && !firstChild.IsUndefined() {
		firstChild.Call("remove")
		firstChild = e.value.Get("firstChild")
	}
}

// GetChildren returns all child elements
func (e *Element) GetChildren() []*Element {
	if e == nil {
		return nil
	}
	children := e.value.Get("children")
	if children.IsUndefined() || children.IsNull() {
		return nil
	}

	length := children.Get("length").Int()
	elems := make([]*Element, length)
	for i := 0; i < length; i++ {
		child := children.Index(i)
		if !child.IsNull() && !child.IsUndefined() {
			elems[i] = &Element{value: child}
		}
	}
	return elems
}

// GetChildCount returns the number of child elements
func (e *Element) GetChildCount() int {
	if e == nil {
		return 0
	}
	children := e.value.Get("children")
	if children.IsUndefined() || children.IsNull() {
		return 0
	}
	return children.Get("length").Int()
}

// GetParent returns the parent element
func (e *Element) GetParent() *Element {
	if e == nil {
		return nil
	}
	parent := e.value.Get("parentElement")
	if parent.IsUndefined() || parent.IsNull() {
		return nil
	}
	return &Element{value: parent}
}

// GetFirstChild returns the first child element
func (e *Element) GetFirstChild() *Element {
	if e == nil {
		return nil
	}
	child := e.value.Get("firstElementChild")
	if child.IsUndefined() || child.IsNull() {
		return nil
	}
	return &Element{value: child}
}

// GetLastChild returns the last child element
func (e *Element) GetLastChild() *Element {
	if e == nil {
		return nil
	}
	child := e.value.Get("lastElementChild")
	if child.IsUndefined() || child.IsNull() {
		return nil
	}
	return &Element{value: child}
}

// GetNextSibling returns the next sibling element
func (e *Element) GetNextSibling() *Element {
	if e == nil {
		return nil
	}
	sibling := e.value.Get("nextElementSibling")
	if sibling.IsUndefined() || sibling.IsNull() {
		return nil
	}
	return &Element{value: sibling}
}

// GetPreviousSibling returns the previous sibling element
func (e *Element) GetPreviousSibling() *Element {
	if e == nil {
		return nil
	}
	sibling := e.value.Get("previousElementSibling")
	if sibling.IsUndefined() || sibling.IsNull() {
		return nil
	}
	return &Element{value: sibling}
}

// === Query within element ===

// QuerySelector returns the first descendant element matching the selector
func (e *Element) QuerySelector(selector string) *Element {
	if e == nil {
		return nil
	}
	v := e.value.Call("querySelector", selector)
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// QuerySelectorAll returns all descendant elements matching the selector
func (e *Element) QuerySelectorAll(selector string) []*Element {
	if e == nil {
		return nil
	}
	values := e.value.Call("querySelectorAll", selector)
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

// === Event Handling ===

// AddEventListener adds an event listener
func (e *Element) AddEventListener(event string, handler func(*Event)) {
	if e == nil {
		return
	}

	callback := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			handler(&Event{value: args[0]})
		} else {
			handler(&Event{value: js.Value{}})
		}
		return nil
	})

	e.value.Call("addEventListener", event, callback)
}

// RemoveEventListener removes an event listener
func (e *Element) RemoveEventListener(event string, handler js.Func) {
	if e == nil {
		return
	}
	e.value.Call("removeEventListener", event, handler)
}

// Click triggers a click event on the element
func (e *Element) Click() {
	if e == nil {
		return
	}
	e.value.Call("click")
}

// Focus focuses the element
func (e *Element) Focus() {
	if e == nil {
		return
	}
	e.value.Call("focus")
}

// Blur removes focus from the element
func (e *Element) Blur() {
	if e == nil {
		return
	}
	e.value.Call("blur")
}

// === Style ===

// SetStyle sets a CSS style property
func (e *Element) SetStyle(property, value string) {
	if e == nil {
		return
	}
	e.value.Get("style").Set(property, value)
}

// GetStyle returns the value of a CSS style property
func (e *Element) GetStyle(property string) string {
	if e == nil {
		return ""
	}
	style := e.value.Get("style")
	return style.Get(property).String()
}

// Show removes display:none style
func (e *Element) Show() {
	e.SetStyle("display", "")
}

// Hide sets display:none style
func (e *Element) Hide() {
	e.SetStyle("display", "none")
}

// === Form Elements ===

// SetValue sets the value (for inputs, textareas, selects)
func (e *Element) SetValue(value string) {
	if e == nil {
		return
	}
	e.value.Set("value", value)
}

// GetValue returns the value
func (e *Element) GetValue() string {
	if e == nil {
		return ""
	}
	return e.value.Get("value").String()
}

// SetChecked sets the checked state (for checkboxes, radios)
func (e *Element) SetChecked(checked bool) {
	if e == nil {
		return
	}
	e.value.Set("checked", checked)
}

// GetChecked returns the checked state
func (e *Element) GetChecked() bool {
	if e == nil {
		return false
	}
	return e.value.Get("checked").Bool()
}

// SetDisabled sets the disabled state
func (e *Element) SetDisabled(disabled bool) {
	if e == nil {
		return
	}
	e.value.Set("disabled", disabled)
}

// GetDisabled returns the disabled state
func (e *Element) GetDisabled() bool {
	if e == nil {
		return false
	}
	return e.value.Get("disabled").Bool()
}

// SetReadOnly sets the readonly state
func (e *Element) SetReadOnly(readonly bool) {
	if e == nil {
		return
	}
	e.value.Set("readOnly", readonly)
}

// === Properties ===

// GetTagName returns the tag name
func (e *Element) GetTagName() string {
	if e == nil {
		return ""
	}
	return e.value.Get("tagName").String()
}

// GetScrollTop returns the scroll top position
func (e *Element) GetScrollTop() int {
	if e == nil {
		return 0
	}
	return e.value.Get("scrollTop").Int()
}

// SetScrollTop sets the scroll top position
func (e *Element) SetScrollTop(value int) {
	if e == nil {
		return
	}
	e.value.Set("scrollTop", value)
}

// GetClientWidth returns the client width
func (e *Element) GetClientWidth() int {
	if e == nil {
		return 0
	}
	return e.value.Get("clientWidth").Int()
}

// GetClientHeight returns the client height
func (e *Element) GetClientHeight() int {
	if e == nil {
		return 0
	}
	return e.value.Get("clientHeight").Int()
}

// GetOffsetWidth returns the offset width
func (e *Element) GetOffsetWidth() int {
	if e == nil {
		return 0
	}
	return e.value.Get("offsetWidth").Int()
}

// GetOffsetHeight returns the offset height
func (e *Element) GetOffsetHeight() int {
	if e == nil {
		return 0
	}
	return e.value.Get("offsetHeight").Int()
}
