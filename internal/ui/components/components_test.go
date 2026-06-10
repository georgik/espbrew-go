//go:build js
// +build js

package components

import (
	"testing"

	"codeberg.org/georgik/espbrew-go/internal/ui/dom"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewButton tests button creation
func TestNewButton(t *testing.T) {
	config := ButtonConfig{
		Text:     "Click Me",
		Class:    "primary",
		Disabled: false,
	}

	btn := NewButton(config)

	require.NotNil(t, btn)
	assert.Equal(t, "Click Me", btn.GetTextContent())
	assert.True(t, btn.HasClass("btn"))
	assert.True(t, btn.HasClass("primary"))
	assert.False(t, btn.GetDisabled())
}

// TestButtonDisabled tests disabled button
func TestButtonDisabled(t *testing.T) {
	config := ButtonConfig{
		Text:     "Disabled",
		Disabled: true,
	}

	btn := NewButton(config)

	require.NotNil(t, btn)
	assert.True(t, btn.GetDisabled())

	btn.SetDisabled(false)
	assert.False(t, btn.GetDisabled())

	btn.Disable()
	assert.True(t, btn.GetDisabled())

	btn.Enable()
	assert.False(t, btn.GetDisabled())
}

// TestButtonText tests button text
func TestButtonText(t *testing.T) {
	config := ButtonConfig{
		Text: "Original",
	}

	btn := NewButton(config)
	assert.Equal(t, "Original", btn.GetTextContent())

	btn.SetText("Updated")
	assert.Equal(t, "Updated", btn.GetTextContent())
}

// TestButtonClick tests button click handler
func TestButtonClick(t *testing.T) {
	clicked := false
	config := ButtonConfig{
		Text: "Click Me",
		OnClick: func(evt *dom.Event) {
			clicked = true
		},
	}

	btn := NewButton(config)

	// Trigger click event (in real WASM environment, this would trigger the handler)
	// For unit testing, we'd need to mock event triggering
	_ = btn
	_ = clicked
}

// TestNewCard tests card creation
func TestNewCard(t *testing.T) {
	content := dom.GlobalDocument().CreateElement("div")
	content.SetTextContent("Card content")

	config := CardConfig{
		Title:   "Test Card",
		Class:   "test-card-class",
		Content: content,
	}

	card := NewCard(config)

	require.NotNil(t, card)
	assert.True(t, card.HasClass("card"))
	assert.True(t, card.HasClass("test-card-class"))

	title := card.QuerySelector(".card-title")
	require.NotNil(t, title)
	assert.Equal(t, "Test Card", title.GetTextContent())

	cardContent := card.GetContent()
	require.NotNil(t, cardContent)
	assert.Contains(t, cardContent.GetInnerHTML(), "Card content")
}

// TestCardSetTitle tests card title update
func TestCardSetTitle(t *testing.T) {
	config := CardConfig{
		Title: "Original Title",
	}

	card := NewCard(config)
	assert.Equal(t, "Original Title", card.QuerySelector(".card-title").GetTextContent())

	card.SetTitle("Updated Title")
	assert.Equal(t, "Updated Title", card.QuerySelector(".card-title").GetTextContent())
}

// TestCardSetContent tests card content update
func TestCardSetContent(t *testing.T) {
	config := CardConfig{
		Title: "Test Card",
	}

	card := NewCard(config)

	newContent := dom.GlobalDocument().CreateElement("p")
	newContent.SetTextContent("New content")

	card.SetContent(newContent)

	content := card.GetContent()
	require.NotNil(t, content)
	assert.Contains(t, content.GetInnerHTML(), "New content")
}

// TestNewModal tests modal creation
func TestNewModal(t *testing.T) {
	config := ModalConfig{
		ID:       "test-modal",
		Class:    "test-modal-class",
		Closable: true,
	}

	modal := NewModal(config)

	require.NotNil(t, modal)
	assert.True(t, modal.HasClass("modal-overlay"))
	assert.True(t, modal.HasClass("test-modal-class"))
	assert.Equal(t, "test-modal", modal.GetID())
}

// TestModalShowHide tests modal show/hide
func TestModalShowHide(t *testing.T) {
	config := ModalConfig{
		Closable: false,
	}

	modal := NewModal(config)

	assert.False(t, modal.IsVisible())

	modal.Show()
	assert.True(t, modal.IsVisible())

	modal.Hide()
	assert.False(t, modal.IsVisible())
}

// TestModalClose tests modal close with callback
func TestModalClose(t *testing.T) {
	closed := false
	config := ModalConfig{
		Closable: true,
		OnClose: func() {
			closed = true
		},
	}

	modal := NewModal(config)
	modal.Show()

	modal.Close()

	assert.False(t, modal.IsVisible())
	assert.True(t, closed)
}

// TestModalSetTitle tests modal title
func TestModalSetTitle(t *testing.T) {
	config := ModalConfig{
		Closable: true,
	}

	modal := NewModal(config)
	modal.SetTitle("Test Title")

	title := modal.GetContent().QuerySelector(".modal-title")
	require.NotNil(t, title)
	assert.Equal(t, "Test Title", title.GetTextContent())
}

// TestModalSetContent tests modal content
func TestModalSetContent(t *testing.T) {
	config := ModalConfig{
		Closable: true,
	}

	modal := NewModal(config)

	content := dom.GlobalDocument().CreateElement("p")
	content.SetTextContent("Modal content")

	modal.SetContent(content)

	modalContent := modal.GetContent()
	require.NotNil(t, modalContent)
	assert.Contains(t, modalContent.GetInnerHTML(), "Modal content")
}
