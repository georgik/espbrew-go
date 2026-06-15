//go:build js
// +build js

package pages

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/components"
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
	"codeberg.org/georgik/espbrew-go/internal/ui/layout"
)

// Settings renders the settings page
func Settings(app *layout.App) {
	app.SetTitle("Settings")
	app.SetMainContentFunc(renderSettingsContent)
}

func renderSettingsContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Settings")
	container.Append(header)

	// Connection settings card
	connCard := createConnectionSettings()
	container.Append(connCard)

	// Display settings card
	displayCard := createDisplaySettings()
	container.Append(displayCard)

	// Camera properties card
	cameraCard := createCameraPropertiesCard()
	container.Append(cameraCard)

	// About card
	aboutCard := createAboutCard()
	container.Append(aboutCard)

	return container
}

func createConnectionSettings() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Connection Settings",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// API endpoint info
	endpointInfo := doc.CreateElement("div")
	endpointInfo.SetStyle("padding", "12px")
	endpointInfo.SetStyle("background-color", "rgba(255,255,255,0.05)")
	endpointInfo.SetStyle("border-radius", "6px")
	endpointInfo.SetStyle("font-size", "13px")

	endpointLabel := doc.CreateElement("div")
	endpointLabel.SetStyle("font-weight", "500")
	endpointLabel.SetStyle("margin-bottom", "4px")
	endpointLabel.SetTextContent("API Endpoint")
	endpointInfo.Append(endpointLabel)

	endpointValue := doc.CreateElement("div")
	endpointValue.SetStyle("color", "#aaa")
	endpointValue.SetTextContent("/api/v1")
	endpointInfo.Append(endpointValue)

	content.Append(endpointInfo)

	// Connection status
	statusInfo := doc.CreateElement("div")
	statusInfo.SetStyle("padding", "12px")
	statusInfo.SetStyle("background-color", "rgba(255,255,255,0.05)")
	statusInfo.SetStyle("border-radius", "6px")
	statusInfo.SetStyle("font-size", "13px")

	statusLabel := doc.CreateElement("div")
	statusLabel.SetStyle("font-weight", "500")
	statusLabel.SetStyle("margin-bottom", "4px")
	statusLabel.SetTextContent("Connection Status")
	statusInfo.Append(statusLabel)

	statusValue := doc.CreateElement("div")
	statusValue.SetID("settings-connection-status")
	statusValue.SetStyle("color", "#aaa")
	statusValue.SetTextContent("Checking...")
	statusInfo.Append(statusValue)

	content.Append(statusInfo)

	card.SetContent(content)
	return card.Element
}

func createDisplaySettings() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Display Settings",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// Theme info
	themeInfo := doc.CreateElement("div")
	themeInfo.SetStyle("padding", "12px")
	themeInfo.SetStyle("background-color", "rgba(255,255,255,0.05)")
	themeInfo.SetStyle("border-radius", "6px")
	themeInfo.SetStyle("font-size", "13px")

	themeLabel := doc.CreateElement("div")
	themeLabel.SetStyle("font-weight", "500")
	themeLabel.SetStyle("margin-bottom", "4px")
	themeLabel.SetTextContent("Theme")
	themeInfo.Append(themeLabel)

	themeValue := doc.CreateElement("div")
	themeValue.SetStyle("color", "#aaa")
	themeValue.SetTextContent("Dark (default)")
	themeInfo.Append(themeValue)

	content.Append(themeInfo)

	card.SetContent(content)
	return card.Element
}

func createAboutCard() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "About",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "8px")
	content.SetStyle("font-size", "13px")
	content.SetStyle("color", "#aaa")

	version := doc.CreateElement("div")
	version.SetTextContent("ESPBrew V2 WASM UI")
	content.Append(version)

	architecture := doc.CreateElement("div")
	architecture.SetTextContent("Pure Go WebAssembly")
	content.Append(architecture)

	notes := doc.CreateElement("div")
	notes.SetStyle("margin-top", "8px")
	notes.SetTextContent("No npm dependencies. Built with syscall/js.")
	content.Append(notes)

	card.SetContent(content)
	return card.Element
}

func createCameraPropertiesCard() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Camera Properties",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "16px")

	// Info text
	info := doc.CreateElement("div")
	info.SetStyle("padding", "12px")
	info.SetStyle("background-color", "rgba(255,255,255,0.05)")
	info.SetStyle("border-radius", "6px")
	info.SetStyle("font-size", "13px")
	info.SetStyle("color", "#aaa")
	info.SetTextContent("Camera properties can be configured on the Cameras page. Navigate to Cameras in the sidebar for full controls.")
	content.Append(info)

	// Quick link to cameras page
	linkBtn := components.NewButton(components.ButtonConfig{
		Text:  "Go to Cameras",
		Class: "btn-primary",
		OnClick: func(_ *dom.Event) {
			// Navigate to cameras page
			js.Global().Get("window").Get("location").Set("hash", "#cameras")
		},
	})
	content.Append(linkBtn.Element)

	card.SetContent(content)
	return card.Element
}
