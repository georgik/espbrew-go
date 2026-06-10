//go:build js
// +build js

package dom

import (
	"syscall/js"
)

// Event wraps js.Value for DOM event operations
type Event struct {
	value js.Value
}

// Value returns the underlying js.Value
func (e *Event) Value() js.Value {
	if e == nil {
		return js.Undefined()
	}
	return e.value
}

// PreventDefault prevents the default action
func (e *Event) PreventDefault() {
	if e == nil {
		return
	}
	e.value.Call("preventDefault")
}

// StopPropagation stops the event from bubbling
func (e *Event) StopPropagation() {
	if e == nil {
		return
	}
	e.value.Call("stopPropagation")
}

// StopImmediatePropagation stops other listeners
func (e *Event) StopImmediatePropagation() {
	if e == nil {
		return
	}
	e.value.Call("stopImmediatePropagation")
}

// Target returns the element that triggered the event
func (e *Event) Target() *Element {
	if e == nil {
		return nil
	}
	v := e.value.Get("target")
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// CurrentTarget returns the element whose listener was triggered
func (e *Event) CurrentTarget() *Element {
	if e == nil {
		return nil
	}
	v := e.value.Get("currentTarget")
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	return &Element{value: v}
}

// Type returns the event type
func (e *Event) Type() string {
	if e == nil {
		return ""
	}
	return e.value.Get("type").String()
}

// Bubbles returns true if the event bubbles
func (e *Event) Bubbles() bool {
	if e == nil {
		return false
	}
	return e.value.Get("bubbles").Bool()
}

// Cancelable returns true if the event can be cancelled
func (e *Event) Cancelable() bool {
	if e == nil {
		return false
	}
	return e.value.Get("cancelable").Bool()
}

// DefaultPrevented returns true if preventDefault was called
func (e *Event) DefaultPrevented() bool {
	if e == nil {
		return false
	}
	return e.value.Get("defaultPrevented").Bool()
}

// TimeStamp returns the event timestamp
func (e *Event) TimeStamp() int {
	if e == nil {
		return 0
	}
	return e.value.Get("timeStamp").Int()
}

// === MouseEvent helpers ===

// ClientX returns the mouse X coordinate
func (e *Event) ClientX() int {
	if e == nil {
		return 0
	}
	return e.value.Get("clientX").Int()
}

// ClientY returns the mouse Y coordinate
func (e *Event) ClientY() int {
	if e == nil {
		return 0
	}
	return e.value.Get("clientY").Int()
}

// ScreenX returns the screen X coordinate
func (e *Event) ScreenX() int {
	if e == nil {
		return 0
	}
	return e.value.Get("screenX").Int()
}

// ScreenY returns the screen Y coordinate
func (e *Event) ScreenY() int {
	if e == nil {
		return 0
	}
	return e.value.Get("screenY").Int()
}

// Button returns which mouse button was pressed (0=left, 1=middle, 2=right)
func (e *Event) Button() int {
	if e == nil {
		return 0
	}
	return e.value.Get("button").Int()
}

// Buttons returns the buttons pressed
func (e *Event) Buttons() int {
	if e == nil {
		return 0
	}
	return e.value.Get("buttons").Int()
}

// CtrlKey returns true if Ctrl key was pressed
func (e *Event) CtrlKey() bool {
	if e == nil {
		return false
	}
	return e.value.Get("ctrlKey").Bool()
}

// ShiftKey returns true if Shift key was pressed
func (e *Event) ShiftKey() bool {
	if e == nil {
		return false
	}
	return e.value.Get("shiftKey").Bool()
}

// AltKey returns true if Alt key was pressed
func (e *Event) AltKey() bool {
	if e == nil {
		return false
	}
	return e.value.Get("altKey").Bool()
}

// MetaKey returns true if Meta key was pressed
func (e *Event) MetaKey() bool {
	if e == nil {
		return false
	}
	return e.value.Get("metaKey").Bool()
}

// === KeyboardEvent helpers ===

// Key returns the key value
func (e *Event) Key() string {
	if e == nil {
		return ""
	}
	return e.value.Get("key").String()
}

// Code returns the physical key code
func (e *Event) Code() string {
	if e == nil {
		return ""
	}
	return e.value.Get("code").String()
}

// KeyCode returns the legacy key code
func (e *Event) KeyCode() int {
	if e == nil {
		return 0
	}
	return e.value.Get("keyCode").Int()
}

// Repeat returns true if the key is being held down
func (e *Event) Repeat() bool {
	if e == nil {
		return false
	}
	return e.value.Get("repeat").Bool()
}

// === InputEvent helpers ===

// InputType returns the input type (for input events)
func (e *Event) InputType() string {
	if e == nil {
		return ""
	}
	return e.value.Get("inputType").String()
}

// Data returns the inserted data (for input events)
func (e *Event) Data() string {
	if e == nil {
		return ""
	}
	return e.value.Get("data").String()
}

// === Event type constants ===

const (
	EventClick      = "click"
	EventDblClick   = "dblclick"
	EventMouseDown  = "mousedown"
	EventMouseUp    = "mouseup"
	EventMouseMove  = "mousemove"
	EventMouseOver  = "mouseover"
	EventMouseOut   = "mouseout"
	EventMouseEnter = "mouseenter"
	EventMouseLeave = "mouseleave"
	EventWheel      = "wheel"

	EventKeyDown  = "keydown"
	EventKeyUp    = "keyup"
	EventKeyPress = "keypress"

	EventChange = "change"
	EventInput  = "input"
	EventFocus  = "focus"
	EventBlur   = "blur"
	EventSubmit = "submit"
	EventReset  = "reset"

	EventLoad   = "load"
	EventUnload = "unload"
	EventAbort  = "abort"
	EventError  = "error"
	EventResize = "resize"
	EventScroll = "scroll"

	EventDOMContentLoaded = "DOMContentLoaded"
	EventReadyStateChange = "readystatechange"

	EventTouchStart  = "touchstart"
	EventTouchMove   = "touchmove"
	EventTouchEnd    = "touchend"
	EventTouchCancel = "touchcancel"
)

// EventHandler is a function that handles an event
type EventHandler func(*Event)

// WrapHandler converts a Go function to js.Func
// Note: The returned js.Func must be released when no longer needed
func WrapHandler(h EventHandler) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			h(&Event{value: args[0]})
		} else {
			h(&Event{value: js.Value{}})
		}
		return nil
	})
}
