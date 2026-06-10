//go:build js
// +build js

package components

import (
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// Card represents a card container
type Card struct {
	*dom.Element
	title   *dom.Element
	content *dom.Element
}

// CardConfig configures a card
type CardConfig struct {
	Title      string
	TitleClass string
	Class      string
	Content    *dom.Element
}

// NewCard creates a new card
func NewCard(config CardConfig) *Card {
	card := &Card{
		Element: dom.GlobalDocument().CreateElement("div"),
	}

	card.SetClass("card " + config.Class)

	if config.Title != "" {
		card.title = dom.GlobalDocument().CreateElement("div")
		card.title.SetClass("card-title " + config.TitleClass)
		card.title.SetTextContent(config.Title)
		card.Append(card.title)
	}

	if config.Content != nil {
		card.content = dom.GlobalDocument().CreateElement("div")
		card.content.SetClass("card-content")
		card.content.Append(config.Content)
		card.Append(card.content)
	} else if config.Title == "" {
		// If no title and no content, create empty content area
		card.content = dom.GlobalDocument().CreateElement("div")
		card.content.SetClass("card-content")
		card.Append(card.content)
	}

	return card
}

// SetTitle updates the card title
func (c *Card) SetTitle(title string) {
	if c.title != nil {
		c.title.SetTextContent(title)
	}
}

// SetContent replaces the card content
func (c *Card) SetContent(content *dom.Element) {
	if c.content == nil {
		c.content = dom.GlobalDocument().CreateElement("div")
		c.content.SetClass("card-content")
		c.Append(c.content)
	}

	c.content.RemoveChildren()
	if content != nil {
		c.content.Append(content)
	}
}

// GetContent returns the content element
func (c *Card) GetContent() *dom.Element {
	return c.content
}

// AddToContent appends an element to the content area
func (c *Card) AddToContent(elem *dom.Element) {
	if c.content != nil && elem != nil {
		c.content.Append(elem)
	}
}
