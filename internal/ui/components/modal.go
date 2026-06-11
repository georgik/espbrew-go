//go:build js
// +build js

package components

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// Modal represents a modal dialog
type Modal struct {
	*dom.Element
	overlay  *dom.Element
	content  *dom.Element
	closeBtn *dom.Element
	onClose  func()
}

// ModalConfig configures a modal
type ModalConfig struct {
	ID       string
	Class    string
	Closable bool
	OnClose  func()
}

// NewModal creates a new modal
func NewModal(config ModalConfig) *Modal {
	// Create overlay
	overlay := dom.GlobalDocument().CreateElement("div")
	overlay.SetClass("modal-overlay")
	if config.ID != "" {
		overlay.SetID(config.ID)
	}

	// Create modal content container
	content := dom.GlobalDocument().CreateElement("div")
	content.SetClass("modal-content " + config.Class)

	// Create close button if closable
	var closeBtn *dom.Element
	if config.Closable {
		closeBtn = dom.GlobalDocument().CreateElement("button")
		closeBtn.SetClass("modal-close")
		closeBtn.SetTextContent("×")
		content.Append(closeBtn)
	}

	overlay.Append(content)

	modal := &Modal{
		Element:  overlay,
		overlay:  overlay,
		content:  content,
		closeBtn: closeBtn,
		onClose:  config.OnClose,
	}

	// Set up event handlers
	if config.Closable {
		// Close on button click
		closeBtn.AddEventListener(dom.EventClick, func(evt *dom.Event) {
			evt.PreventDefault()
			evt.StopPropagation()
			modal.Close()
		})

		// Close on overlay click (outside content)
		overlay.AddEventListener(dom.EventClick, func(evt *dom.Event) {
			if evt.Target() == overlay {
				modal.Close()
			}
		})

		// Close on ESC key
		js.Global().Get("document").Call("addEventListener", "keydown", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if args[0].Get("key").String() == "Escape" {
				modal.Close()
			}
			return js.Undefined()
		}))
	}

	return modal
}

// Show displays the modal
func (m *Modal) Show() {
	m.AddClass("active")
}

// Hide hides the modal
func (m *Modal) Hide() {
	m.RemoveClass("active")
}

// Close closes the modal and calls the onClose callback
func (m *Modal) Close() {
	m.Hide()
	if m.onClose != nil {
		m.onClose()
	}
}

// IsVisible returns true if the modal is currently visible
func (m *Modal) IsVisible() bool {
	return m.HasClass("active")
}

// SetContent replaces the modal content (keeps close button)
func (m *Modal) SetContent(elem *dom.Element) {
	// Remove old content except close button
	for _, child := range m.content.GetChildren() {
		if child != m.closeBtn {
			child.Remove()
		}
	}

	// Add new content
	if elem != nil {
		m.content.Append(elem)
	}
}

// GetContent returns the content element
func (m *Modal) GetContent() *dom.Element {
	return m.content
}

// SetTitle sets a title in the modal header
func (m *Modal) SetTitle(title string) {
	// Check if title already exists
	titleElem := m.content.QuerySelector(".modal-title")
	if titleElem == nil {
		// Create title element
		titleElem = dom.GlobalDocument().CreateElement("div")
		titleElem.SetClass("modal-title")
		titleElem.SetTextContent(title)

		// Insert after close button or at start
		if m.closeBtn != nil {
			// Get next sibling and insert before it
			next := m.closeBtn.GetNextSibling()
			if next != nil {
				m.content.InsertBefore(titleElem, next)
			} else {
				m.content.Append(titleElem)
			}
		} else {
			m.content.Prepend(titleElem)
		}
	} else {
		titleElem.SetTextContent(title)
	}
}
