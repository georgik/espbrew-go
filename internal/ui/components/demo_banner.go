//go:build js
// +build js

package components

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/api"
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// DemoBanner displays a banner when demo mode is active.
// The banner shows a purple gradient with "DEMO MODE" badge,
// informs users that mock data is being displayed,
// and provides a link to exit demo mode.
type DemoBanner struct {
	*dom.Element
}

// NewDemoBanner creates a demo mode banner if demo mode is enabled.
// Returns nil if demo mode is not active.
func NewDemoBanner() *DemoBanner {
	if !api.DemoModeEnabled() {
		return nil
	}

	doc := dom.GlobalDocument()
	banner := doc.CreateElement("div")
	banner.SetClass("demo-banner")

	content := doc.CreateElement("div")
	content.SetClass("demo-banner-content")

	// Demo badge
	badge := doc.CreateElement("span")
	badge.SetClass("demo-banner-icon")
	badge.SetTextContent("DEMO MODE")
	content.Append(badge)

	// Info text
	info := doc.CreateElement("span")
	info.SetTextContent("Showing mock data - no backend connected")
	content.Append(info)

	// Exit link
	exitLink := doc.CreateElement("a")
	exitLink.SetClass("demo-banner-exit")
	exitLink.SetAttribute("href", "./")
	exitLink.SetTextContent("Exit Demo")
	content.Append(exitLink)

	banner.Append(content)

	return &DemoBanner{
		Element: banner,
	}
}

// ShowDemoBanner adds the demo banner to the page if demo mode is active.
// The banner is prepended to the document body so it appears at the top.
func ShowDemoBanner() {
	banner := NewDemoBanner()
	if banner == nil {
		return
	}

	doc := dom.GlobalDocument()
	body := doc.GetBody()
	if body != nil {
		// Prepend to body so it appears at the top
		body.Value().Call("insertBefore", banner.Value(), body.Value().Get("firstChild"))
	}
}

// HideDemoBanner removes the demo banner from the page.
func HideDemoBanner() {
	doc := dom.GlobalDocument()
	banner := doc.QuerySelector(".demo-banner")
	if banner != nil {
		banner.Value().Call("remove")
	}
}

// Export demo banner functions for JavaScript interop
func init() {
	js.Global().Set("espbrewDemoBanner", js.ValueOf(map[string]interface{}{
		"show": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			ShowDemoBanner()
			return nil
		}),
		"hide": js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			HideDemoBanner()
			return nil
		}),
	}))
}
