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

var (
	currentGalleryCapture *api.Capture
	currentDeviceCaptures []api.DeviceCaptureInfo
)

// Gallery renders the capture gallery page
func Gallery(app *layout.App) {
	app.SetTitle("Capture Gallery")
	app.SetMainContentFunc(renderGalleryContent)
}

func renderGalleryContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Capture Gallery")
	container.Append(header)

	// Gallery filters card
	filterCard := createGalleryFilters()
	container.Append(filterCard)

	// Gallery grid
	galleryGrid := doc.CreateElement("div")
	galleryGrid.SetID("gallery-grid")
	galleryGrid.SetStyle("display", "grid")
	galleryGrid.SetStyle("grid-template-columns", "repeat(auto-fill, minmax(280px, 1fr))")
	galleryGrid.SetStyle("gap", "16px")
	galleryGrid.SetStyle("margin-top", "16px")
	container.Append(galleryGrid)

	// Loading state
	loading := doc.CreateElement("div")
	loading.SetID("gallery-loading")
	loading.SetClass("loading")
	loading.SetTextContent("Loading captures...")
	loading.SetStyle("text-align", "center")
	loading.SetStyle("padding", "40px")
	container.Append(loading)

	// Empty state
	emptyState := doc.CreateElement("div")
	emptyState.SetID("gallery-empty")
	emptyState.SetClass("empty-state")
	emptyState.SetTextContent("No captures yet. Use the Capture page to take photos.")
	emptyState.SetStyle("display", "none")
	container.Append(emptyState)

	// Load captures
	loadGalleryCaptures()

	return container
}

func createGalleryFilters() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Filters",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("gap", "12px")
	content.SetStyle("flex-wrap", "wrap")

	// View mode toggle
	viewModeLabel := doc.CreateElement("label")
	viewModeLabel.SetTextContent("View:")
	viewModeLabel.SetStyle("font-size", "13px")
	viewModeLabel.SetStyle("font-weight", "500")
	content.Append(viewModeLabel)

	// All captures button
	allBtn := components.NewButton(components.ButtonConfig{
		Text:    "All Captures",
		Class:   "btn-primary",
		OnClick: func(_ *dom.Event) { setGalleryView("all") },
	})
	allBtn.Element.SetID("gallery-view-all")
	allBtn.Element.SetStyle("font-size", "13px")
	content.Append(allBtn.Element)

	// Device-specific button
	deviceBtn := components.NewButton(components.ButtonConfig{
		Text:    "Device Gallery",
		Class:   "btn-secondary",
		OnClick: func(_ *dom.Event) { setGalleryView("device") },
	})
	deviceBtn.Element.SetID("gallery-view-device")
	deviceBtn.Element.SetStyle("font-size", "13px")
	content.Append(deviceBtn.Element)

	// Device selector (initially hidden)
	deviceSelect := doc.CreateElement("select")
	deviceSelect.SetID("gallery-device-select")
	deviceSelect.SetStyle("padding", "6px 12px")
	deviceSelect.SetStyle("border-radius", "4px")
	deviceSelect.SetStyle("background-color", "#161634")
	deviceSelect.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	deviceSelect.SetStyle("color", "#eee")
	deviceSelect.SetStyle("font-size", "13px")
	deviceSelect.SetStyle("display", "none")
	deviceSelect.AddEventListener("change", func(_ *dom.Event) {
		loadDeviceGalleryCaptures()
	})
	content.Append(deviceSelect)

	// Refresh button
	refreshBtn := components.NewButton(components.ButtonConfig{
		Text:    "Refresh",
		Class:   "btn-secondary",
		OnClick: func(_ *dom.Event) { loadGalleryCaptures() },
	})
	refreshBtn.Element.SetStyle("font-size", "13px")
	content.Append(refreshBtn.Element)

	card.SetContent(content)
	return card.Element
}

func loadGalleryCaptures() {
	doc := dom.GlobalDocument()
	loading := doc.GetElementByID("gallery-loading")
	emptyState := doc.GetElementByID("gallery-empty")
	galleryGrid := doc.GetElementByID("gallery-grid")

	if loading != nil {
		loading.SetStyle("display", "block")
	}
	if emptyState != nil {
		emptyState.SetStyle("display", "none")
	}
	if galleryGrid != nil {
		galleryGrid.SetStyle("display", "none")
	}

	api.GetCaptures(func(captures []api.Capture, err error) {
		if loading != nil {
			loading.SetStyle("display", "none")
		}

		if err != nil {
			showGalleryError("Failed to load captures: " + err.Error())
			return
		}

		if galleryGrid == nil {
			return
		}

		galleryGrid.RemoveChildren()
		galleryGrid.SetStyle("display", "grid")

		if len(captures) == 0 {
			if emptyState != nil {
				emptyState.SetStyle("display", "block")
			}
			return
		}

		// Populate device selector if we have devices
		populateDeviceSelector()

		for _, capture := range captures {
			card := createCaptureCard(capture)
			galleryGrid.Append(card)
		}
	})
}

func populateDeviceSelector() {
	doc := dom.GlobalDocument()
	deviceSelect := doc.GetElementByID("gallery-device-select")
	if deviceSelect == nil {
		return
	}

	// Get devices list
	api.GetDevices(func(devices []api.Device, err error) {
		if err != nil || len(devices) == 0 {
			return
		}

		deviceSelect.RemoveChildren()

		// Add default option
		defaultOption := doc.CreateElement("option")
		defaultOption.SetTextContent("-- Select Device --")
		defaultOption.SetAttribute("value", "")
		deviceSelect.Append(defaultOption)

		for _, dev := range devices {
			option := doc.CreateElement("option")
			option.SetAttribute("value", dev.DeviceID)
			displayName := dev.DeviceID
			if len(dev.Aliases) > 0 {
				displayName = dev.Aliases[0]
			}
			option.SetTextContent(displayName)
			deviceSelect.Append(option)
		}
	})
}

func createCaptureCard(capture api.Capture) *dom.Element {
	doc := dom.GlobalDocument()
	card := doc.CreateElement("div")
	card.SetStyle("background-color", "#161634")
	card.SetStyle("border-radius", "8px")
	card.SetStyle("overflow", "hidden")
	card.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	card.SetStyle("cursor", "pointer")
	card.AddEventListener("click", func(_ *dom.Event) {
		showCaptureDetail(capture)
	})

	// Thumbnail
	thumbnail := doc.CreateElement("img")
	thumbnail.SetAttribute("src", capture.Path)
	thumbnail.SetStyle("width", "100%")
	thumbnail.SetStyle("height", "180px")
	thumbnail.SetStyle("object-fit", "cover")
	thumbnail.SetStyle("background-color", "#0a0a1a")
	card.Append(thumbnail)

	// Info
	info := doc.CreateElement("div")
	info.SetStyle("padding", "12px")

	filename := doc.CreateElement("div")
	filename.SetStyle("font-weight", "500")
	filename.SetStyle("font-size", "14px")
	filename.SetStyle("margin-bottom", "4px")
	filename.SetStyle("overflow", "hidden")
	filename.SetStyle("text-overflow", "ellipsis")
	filename.SetStyle("white-space", "nowrap")
	filename.SetTextContent(capture.Filename)
	info.Append(filename)

	cameraInfo := doc.CreateElement("div")
	cameraInfo.SetStyle("font-size", "12px")
	cameraInfo.SetStyle("color", "#aaa")
	cameraInfo.SetTextContent(capture.CameraName)
	info.Append(cameraInfo)

	card.Append(info)

	return card
}

func showCaptureDetail(capture api.Capture) {
	currentGalleryCapture = &capture
	doc := dom.GlobalDocument()

	// Create modal
	modal := components.NewModal(components.ModalConfig{
		ID:       "capture-detail-modal",
		Closable: true,
		OnClose: func() {
			currentGalleryCapture = nil
			currentDeviceCaptures = nil
		},
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "16px")
	content.SetStyle("max-width", "800px")
	content.SetStyle("max-height", "90vh")
	content.SetStyle("overflow-y", "auto")

	// Capture image
	image := doc.CreateElement("img")
	image.SetAttribute("src", capture.Path)
	image.SetStyle("max-width", "100%")
	image.SetStyle("max-height", "60vh")
	image.SetStyle("object-fit", "contain")
	image.SetStyle("border-radius", "4px")
	content.Append(image)

	// Info section
	info := doc.CreateElement("div")
	info.SetStyle("display", "flex")
	info.SetStyle("flex-direction", "column")
	info.SetStyle("gap", "8px")

	filename := doc.CreateElement("div")
	filename.SetStyle("font-weight", "500")
	filename.SetTextContent(capture.Filename)
	info.Append(filename)

	meta := doc.CreateElement("div")
	meta.SetStyle("font-size", "13px")
	meta.SetStyle("color", "#aaa")
	meta.SetTextContent("Camera: " + capture.CameraName + " | Size: " + formatFileSize(capture.Size))
	info.Append(meta)

	// Device captures section
	deviceSection := doc.CreateElement("div")
	deviceSection.SetID("capture-device-section")
	deviceSection.SetStyle("margin-top", "12px")

	deviceHeader := doc.CreateElement("div")
	deviceHeader.SetStyle("font-weight", "500")
	deviceHeader.SetStyle("margin-bottom", "8px")
	deviceHeader.SetTextContent("Device Captures")
	deviceSection.Append(deviceHeader)

	deviceLoading := doc.CreateElement("div")
	deviceLoading.SetID("capture-device-loading")
	deviceLoading.SetClass("loading")
	deviceLoading.SetTextContent("Loading device captures...")
	deviceLoading.SetStyle("font-size", "13px")
	deviceSection.Append(deviceLoading)

	deviceGrid := doc.CreateElement("div")
	deviceGrid.SetID("capture-device-grid")
	deviceGrid.SetStyle("display", "grid")
	deviceGrid.SetStyle("grid-template-columns", "repeat(auto-fill, minmax(120px, 1fr))")
	deviceGrid.SetStyle("gap", "8px")
	deviceSection.Append(deviceGrid)

	content.Append(deviceSection)

	modal.SetContent(content)
	doc.GetBody().Append(modal.Element)
	modal.Show()

	// Load device captures for this capture
	loadCaptureDeviceCaptures(capture)
}

func loadCaptureDeviceCaptures(capture api.Capture) {
	doc := dom.GlobalDocument()
	loading := doc.GetElementByID("capture-device-loading")
	deviceGrid := doc.GetElementByID("capture-device-grid")

	if loading != nil {
		loading.SetStyle("display", "block")
	}
	if deviceGrid != nil {
		deviceGrid.SetStyle("display", "none")
	}

	// Get relative path from full path
	// Path format: "/captures/YYYY-MM-DD/cam-abcd-20260601-153400.jpg"
	// We need to extract: "YYYY-MM-DD/cam-abcd-20260601-153400.jpg"
	capturePath := capture.Path
	if len(capturePath) > 10 && capturePath[:10] == "/captures/" {
		capturePath = capturePath[10:]
	}

	api.GetCaptureDeviceCaptures(capturePath, func(deviceCaptures []api.DeviceCaptureInfo, err error) {
		if loading != nil {
			loading.SetStyle("display", "none")
		}
		if deviceGrid == nil {
			return
		}
		deviceGrid.SetStyle("display", "grid")
		deviceGrid.RemoveChildren()

		currentDeviceCaptures = deviceCaptures

		if err != nil || len(deviceCaptures) == 0 {
			noDevices := doc.CreateElement("div")
			noDevices.SetStyle("font-size", "13px")
			noDevices.SetStyle("color", "#aaa")
			noDevices.SetTextContent("No device captures found. Create device mappings first.")
			deviceGrid.Append(noDevices)
			return
		}

		for _, dc := range deviceCaptures {
			card := createDeviceCaptureCard(dc)
			deviceGrid.Append(card)
		}
	})
}

func createDeviceCaptureCard(dc api.DeviceCaptureInfo) *dom.Element {
	doc := dom.GlobalDocument()
	card := doc.CreateElement("div")
	card.SetStyle("background-color", "#0a0a1a")
	card.SetStyle("border-radius", "4px")
	card.SetStyle("overflow", "hidden")
	card.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	card.SetStyle("cursor", "pointer")
	card.AddEventListener("click", func(_ *dom.Event) {
		showDeviceCaptureDetail(dc)
	})

	// Thumbnail
	thumbnail := doc.CreateElement("img")
	thumbnail.SetAttribute("src", "/captures/"+dc.Subimage)
	thumbnail.SetStyle("width", "100%")
	thumbnail.SetStyle("height", "100px")
	thumbnail.SetStyle("object-fit", "cover")
	thumbnail.SetStyle("background-color", "#0a0a1a")
	card.Append(thumbnail)

	// Device ID
	label := doc.CreateElement("div")
	label.SetStyle("padding", "6px")
	label.SetStyle("font-size", "11px")
	label.SetStyle("text-align", "center")
	label.SetStyle("overflow", "hidden")
	label.SetStyle("text-overflow", "ellipsis")
	label.SetStyle("white-space", "nowrap")
	displayName := dc.DeviceID
	if len(displayName) > 15 {
		displayName = displayName[:15]
	}
	label.SetTextContent(displayName)
	card.Append(label)

	return card
}

func showDeviceCaptureDetail(dc api.DeviceCaptureInfo) {
	doc := dom.GlobalDocument()

	modal := components.NewModal(components.ModalConfig{
		ID:       "device-capture-modal",
		Closable: true,
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// Image
	image := doc.CreateElement("img")
	image.SetAttribute("src", "/captures/"+dc.Subimage)
	image.SetStyle("max-width", "100%")
	image.SetStyle("max-height", "70vh")
	image.SetStyle("object-fit", "contain")
	image.SetStyle("border-radius", "4px")
	content.Append(image)

	// Info
	info := doc.CreateElement("div")
	info.SetStyle("font-size", "13px")

	deviceID := doc.CreateElement("div")
	deviceID.SetStyle("font-weight", "500")
	deviceID.SetTextContent("Device: " + dc.DeviceID)
	info.Append(deviceID)

	bounds := doc.CreateElement("div")
	bounds.SetStyle("color", "#aaa")
	bounds.SetTextContent(formatBounds(dc.Bounds))
	info.Append(bounds)

	if !dc.Adjustment.IsZero() {
		adj := doc.CreateElement("div")
		adj.SetStyle("color", "#aaa")
		adj.SetTextContent("Adjustments: " + formatAdjustment(dc.Adjustment))
		info.Append(adj)
	}

	content.Append(info)

	modal.SetContent(content)
	doc.GetBody().Append(modal.Element)
	modal.Show()
}

func setGalleryView(view string) {
	doc := dom.GlobalDocument()
	allBtn := doc.GetElementByID("gallery-view-all")
	deviceBtn := doc.GetElementByID("gallery-view-device")
	deviceSelect := doc.GetElementByID("gallery-device-select")

	if view == "device" {
		if allBtn != nil {
			allBtn.SetAttribute("class", "btn-secondary")
		}
		if deviceBtn != nil {
			deviceBtn.SetAttribute("class", "btn-primary")
		}
		if deviceSelect != nil {
			deviceSelect.SetStyle("display", "inline-block")
		}
	} else {
		if allBtn != nil {
			allBtn.SetAttribute("class", "btn-primary")
		}
		if deviceBtn != nil {
			deviceBtn.SetAttribute("class", "btn-secondary")
		}
		if deviceSelect != nil {
			deviceSelect.SetStyle("display", "none")
		}
		loadGalleryCaptures()
	}
}

func loadDeviceGalleryCaptures() {
	doc := dom.GlobalDocument()
	deviceSelect := doc.GetElementByID("gallery-device-select")
	if deviceSelect == nil {
		return
	}

	deviceID := deviceSelect.GetValue()
	if deviceID == "" {
		loadGalleryCaptures()
		return
	}

	// Load device-specific captures
	api.GetDeviceCaptures(deviceID, func(captures []api.Capture, err error) {
		if err != nil {
			showGalleryError("Failed to load device captures: " + err.Error())
			return
		}

		galleryGrid := doc.GetElementByID("gallery-grid")
		emptyState := doc.GetElementByID("gallery-empty")
		loading := doc.GetElementByID("gallery-loading")

		if loading != nil {
			loading.SetStyle("display", "none")
		}
		if galleryGrid == nil {
			return
		}

		galleryGrid.RemoveChildren()
		galleryGrid.SetStyle("display", "grid")

		if len(captures) == 0 {
			if emptyState != nil {
				emptyState.SetTextContent("No captures found for this device.")
				emptyState.SetStyle("display", "block")
			}
			return
		}

		if emptyState != nil {
			emptyState.SetStyle("display", "none")
		}

		for _, capture := range captures {
			card := createCaptureCard(capture)
			galleryGrid.Append(card)
		}
	})
}

func showGalleryError(message string) {
	doc := dom.GlobalDocument()

	errorDiv := doc.GetElementByID("gallery-error")
	if errorDiv != nil {
		errorDiv.Remove()
	}

	errorDiv = doc.CreateElement("div")
	errorDiv.SetID("gallery-error")
	errorDiv.SetClass("error-message")
	errorDiv.SetTextContent(message)

	// Append to main content area
	mainContent := doc.QuerySelector(".main-content")
	if mainContent != nil {
		mainContent.Append(errorDiv)
	}

	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if errorDiv != nil {
			errorDiv.Remove()
		}
		return nil
	}), 5000)
}

// initGalleryPage initializes the gallery page (called on first load)
func initGalleryPage() {
	// Gallery is self-loading on render, no additional init needed
}
