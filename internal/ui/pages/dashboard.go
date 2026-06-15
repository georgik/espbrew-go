//go:build js
// +build js

package pages

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/api"
	"codeberg.org/georgik/espbrew-go/internal/ui/components"
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
	"codeberg.org/georgik/espbrew-go/internal/ui/layout"
)

// Dashboard renders the main dashboard page
func Dashboard(app *layout.App) {
	app.SetTitle("Dashboard")
	app.SetMainContentFunc(renderDashboardContent)
}

func renderDashboardContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	// Header
	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Dashboard")
	container.Append(header)

	// Stats cards
	statsRow := renderStatsCards()
	container.Append(statsRow)

	// Quick actions
	actionsRow := renderQuickActions()
	container.Append(actionsRow)

	// Devices list
	devicesCard := renderDevicesList()
	container.Append(devicesCard)

	// Recent activity
	activityCard := renderRecentActivity()
	container.Append(activityCard)

	return container
}

func renderStatsCards() *dom.Element {
	doc := dom.GlobalDocument()
	row := doc.CreateElement("div")
	row.SetStyle("display", "grid")
	row.SetStyle("grid-template-columns", "repeat(auto-fit, minmax(200px, 1fr))")
	row.SetStyle("gap", "16px")
	row.SetStyle("margin-bottom", "16px")

	// Cameras card
	camStat := renderCameraStat()
	camerasCard := components.NewCard(components.CardConfig{
		Title:   "Cameras",
		Content: camStat,
	})
	row.Append(camerasCard.Element)

	// Devices card
	devStat := renderDeviceStat()
	devicesCard := components.NewCard(components.CardConfig{
		Title:   "Devices",
		Content: devStat,
	})
	row.Append(devicesCard.Element)

	// Captures card
	capStat := renderCaptureStat()
	capturesCard := components.NewCard(components.CardConfig{
		Title:   "Recent Captures",
		Content: capStat,
	})
	row.Append(capturesCard.Element)

	return row
}

func renderCameraStat() *dom.Element {
	doc := dom.GlobalDocument()
	div := doc.CreateElement("div")
	div.SetClass("stat-content")

	count := doc.CreateElement("div")
	count.SetClass("stat-count")
	count.SetTextContent("-")
	div.Append(count)

	// Fetch camera count asynchronously
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		api.GetCameras(func(cameras []api.Camera, err error) {
			if err == nil {
				count.SetTextContent(formatInt(len(cameras)))
			} else {
				count.SetTextContent("Error")
			}
		})
		return nil
	}), 0)

	return div
}

func renderDeviceStat() *dom.Element {
	doc := dom.GlobalDocument()
	div := doc.CreateElement("div")
	div.SetClass("stat-content")

	count := doc.CreateElement("div")
	count.SetClass("stat-count")
	count.SetTextContent("-")
	div.Append(count)

	// Fetch device count asynchronously
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		api.GetDevices(func(devices []api.Device, err error) {
			if err == nil {
				count.SetTextContent(formatInt(len(devices)))
			} else {
				count.SetTextContent("Error")
			}
		})
		return nil
	}), 0)

	return div
}

func renderCaptureStat() *dom.Element {
	doc := dom.GlobalDocument()
	div := doc.CreateElement("div")
	div.SetClass("stat-content")

	count := doc.CreateElement("div")
	count.SetClass("stat-count")
	count.SetTextContent("-")
	div.Append(count)

	// Fetch recent capture count asynchronously
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		api.GetCaptures(func(captures []api.Capture, err error) {
			if err == nil {
				// Show count from last 24 hours or total
				count.SetTextContent(formatInt(len(captures)))
			} else {
				count.SetTextContent("Error")
			}
		})
		return nil
	}), 0)

	return div
}

func renderQuickActions() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("gap", "12px")
	content.SetStyle("flex-wrap", "wrap")

	// Capture button
	captureBtn := components.NewButton(components.ButtonConfig{
		Text:    "Capture",
		Class:   "btn-primary",
		OnClick: func(_ *dom.Event) { NavigateTo("capture") },
	})
	content.Append(captureBtn.Element)

	// Mapping button
	mappingBtn := components.NewButton(components.ButtonConfig{
		Text:    "Device Mapping",
		Class:   "btn-secondary",
		OnClick: func(_ *dom.Event) { NavigateTo("mapping") },
	})
	content.Append(mappingBtn.Element)

	// Flash button
	flashBtn := components.NewButton(components.ButtonConfig{
		Text:    "Flash Device",
		Class:   "btn-secondary",
		OnClick: func(_ *dom.Event) { NavigateTo("flash") },
	})
	content.Append(flashBtn.Element)

	card := components.NewCard(components.CardConfig{
		Title:   "Quick Actions",
		Content: content,
	})
	return card.Element
}

func renderDevicesList() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetID("dashboard-devices")

	// Loading state
	loading := doc.CreateElement("div")
	loading.SetClass("loading")
	loading.SetTextContent("Loading devices...")
	content.Append(loading)

	// Fetch devices asynchronously
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		api.GetDevices(func(devices []api.Device, err error) {
			loading.Remove()
			if err == nil {
				renderDashboardDeviceList(content, devices)
			} else {
				content.SetTextContent("Error loading devices")
			}
		})
		return nil
	}), 0)

	card := components.NewCard(components.CardConfig{
		Title:   "Connected Devices",
		Content: content,
	})
	return card.Element
}

func renderDashboardDeviceList(container *dom.Element, devices []api.Device) {
	container.RemoveChildren()

	if len(devices) == 0 {
		noDevices := dom.GlobalDocument().CreateElement("div")
		noDevices.SetStyle("color", "#aaa")
		noDevices.SetTextContent("No devices connected")
		container.Append(noDevices)
		return
	}

	// Show up to 5 devices
	max := 5
	if len(devices) < max {
		max = len(devices)
	}

	list := dom.GlobalDocument().CreateElement("div")
	list.SetStyle("display", "flex")
	list.SetStyle("flex-direction", "column")
	list.SetStyle("gap", "8px")

	for i := 0; i < max; i++ {
		dev := devices[i]
		deviceItem := createDashboardDeviceItem(dev)
		list.Append(deviceItem)
	}

	container.Append(list)

	// "View All" button if more than 5 devices
	if len(devices) > 5 {
		viewAllBtn := components.NewButton(components.ButtonConfig{
			Text:    "View All Devices",
			Class:   "btn-secondary",
			OnClick: func(_ *dom.Event) { NavigateTo("devices") },
		})
		viewAllBtn.Element.SetStyle("margin-top", "12px")
		container.Append(viewAllBtn.Element)
	}
}

func createDashboardDeviceItem(dev api.Device) *dom.Element {
	doc := dom.GlobalDocument()
	item := doc.CreateElement("div")
	item.SetStyle("display", "flex")
	item.SetStyle("justify-content", "space-between")
	item.SetStyle("align-items", "center")
	item.SetStyle("padding", "8px 12px")
	item.SetStyle("background-color", "rgba(255,255,255,0.03)")
	item.SetStyle("border-radius", "4px")

	info := doc.CreateElement("div")
	info.SetStyle("flex", "1")

	path := doc.CreateElement("div")
	path.SetStyle("font-family", "monospace")
	path.SetStyle("font-size", "13px")
	path.SetTextContent(dev.Path)
	info.Append(path)

	chip := doc.CreateElement("div")
	chip.SetStyle("font-size", "12px")
	chip.SetStyle("color", "#aaa")
	chip.SetTextContent(dev.ChipType)
	info.Append(chip)

	item.Append(info)

	// Status badge
	status := doc.CreateElement("div")
	status.SetStyle("font-size", "11px")
	status.SetStyle("padding", "2px 6px")
	status.SetStyle("border-radius", "4px")

	if dev.Status == "available" {
		status.SetStyle("background-color", "rgba(76, 209, 135, 0.2)")
		status.SetStyle("color", "#4cd137")
		status.SetTextContent("Available")
	} else {
		status.SetStyle("background-color", "rgba(255, 165, 2, 0.2)")
		status.SetStyle("color", "#ffa502")
		status.SetTextContent(dev.Status)
	}

	item.Append(status)

	return item
}

func renderRecentActivity() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetClass("activity-list")

	loading := doc.CreateElement("div")
	loading.SetClass("loading")
	loading.SetTextContent("Loading recent captures...")
	content.Append(loading)

	// Fetch recent captures asynchronously
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		api.GetCaptures(func(captures []api.Capture, err error) {
			if err == nil && len(captures) > 0 {
				loading.Remove()
				renderCaptureList(content, captures)
			} else if err != nil {
				loading.SetTextContent("Error loading captures")
			} else {
				loading.SetTextContent("No captures yet")
			}
		})
		return nil
	}), 0)

	card := components.NewCard(components.CardConfig{
		Title:   "Recent Activity",
		Content: content,
	})
	return card.Element
}

func renderCaptureList(container *dom.Element, captures []api.Capture) {
	container.RemoveChildren()

	doc := dom.GlobalDocument()
	list := doc.CreateElement("div")
	list.SetClass("capture-list")
	list.SetStyle("display", "grid")
	list.SetStyle("grid-template-columns", "repeat(auto-fill, minmax(150px, 1fr))")
	list.SetStyle("gap", "12px")

	// Show up to 8 most recent captures
	max := 8
	if len(captures) < max {
		max = len(captures)
	}

	for i := 0; i < max; i++ {
		cap := captures[i]
		item := renderCaptureItem(cap)
		list.Append(item)
	}

	container.Append(list)
}

func renderCaptureItem(cap api.Capture) *dom.Element {
	doc := dom.GlobalDocument()
	item := doc.CreateElement("div")
	item.SetClass("capture-item")
	item.SetStyle("background-color", "#161634")
	item.SetStyle("border-radius", "6px")
	item.SetStyle("overflow", "hidden")
	item.SetStyle("cursor", "pointer")

	thumbnail := doc.CreateElement("img")
	thumbnail.SetAttribute("src", cap.Path)
	thumbnail.SetAttribute("alt", cap.Filename)
	thumbnail.SetStyle("width", "100%")
	thumbnail.SetStyle("height", "100px")
	thumbnail.SetStyle("object-fit", "cover")
	item.Append(thumbnail)

	info := doc.CreateElement("div")
	info.SetStyle("padding", "8px")
	info.SetStyle("font-size", "12px")

	camera := doc.CreateElement("div")
	camera.SetTextContent(cap.CameraID)
	info.Append(camera)

	timestamp := doc.CreateElement("div")
	timestamp.SetStyle("color", "#aaa")
	timestamp.SetTextContent(formatTimestamp(cap.Timestamp))
	info.Append(timestamp)

	item.Append(info)

	return item
}

// Helper functions
func formatInt(n int) string {
	// Proper integer to string conversion
	if n < 0 {
		return "0"
	}
	if n == 0 {
		return "0"
	}

	// Convert integer to string
	result := ""
	for n > 0 {
		digit := n % 10
		result = string(rune('0'+digit)) + result
		n = n / 10
	}
	return result
}

func formatTimestamp(ts int64) string {
	// Simple timestamp formatting
	if ts == 0 {
		return "Unknown"
	}
	// In full implementation, use proper date formatting
	return "Recently"
}
