//go:build js
// +build js

package pages

import (
	"sort"
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/api"
	"codeberg.org/georgik/espbrew-go/internal/ui/components"
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
	"codeberg.org/georgik/espbrew-go/internal/ui/layout"
)

// Capture renders the image capture page
func Capture(app *layout.App) {
	app.SetTitle("Capture")
	app.SetMainContentFunc(renderCaptureContent)
}

func renderCaptureContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Image Capture")
	container.Append(header)

	// Camera selection card
	cameraCard := createCameraSelector()
	container.Append(cameraCard)

	// Capture button card
	actionCard := createCaptureActions()
	container.Append(actionCard)

	// Recent captures gallery
	galleryCard := createCaptureGallery()
	container.Append(galleryCard)

	return container
}

func createCameraSelector() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Camera Selection",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// Camera dropdown
	label := doc.CreateElement("label")
	label.SetTextContent("Select Camera:")
	label.SetStyle("font-weight", "500")
	content.Append(label)

	selectWrapper := doc.CreateElement("div")
	selectWrapper.SetStyle("position", "relative")

	cameraSelect := doc.CreateElement("select")
	cameraSelect.SetID("camera-select")
	cameraSelect.SetStyle("width", "100%")
	cameraSelect.SetStyle("padding", "8px 12px")
	cameraSelect.SetStyle("border-radius", "6px")
	cameraSelect.SetStyle("background-color", "#161634")
	cameraSelect.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	cameraSelect.SetStyle("color", "#eee")
	cameraSelect.SetStyle("font-size", "14px")

	selectWrapper.Append(cameraSelect)

	// Loading state
	loading := doc.CreateElement("div")
	loading.SetID("camera-loading")
	loading.SetClass("loading")
	loading.SetTextContent("Loading cameras...")
	loading.SetStyle("display", "none")
	selectWrapper.Append(loading)

	content.Append(selectWrapper)

	// Camera info display
	cameraInfo := doc.CreateElement("div")
	cameraInfo.SetID("camera-info")
	cameraInfo.SetStyle("padding", "12px")
	cameraInfo.SetStyle("background-color", "rgba(255,255,255,0.05)")
	cameraInfo.SetStyle("border-radius", "6px")
	cameraInfo.SetStyle("font-size", "13px")
	cameraInfo.SetStyle("display", "none")

	infoName := doc.CreateElement("div")
	infoName.SetID("camera-name")
	infoName.SetStyle("font-weight", "500")
	infoName.SetStyle("margin-bottom", "4px")
	cameraInfo.Append(infoName)

	infoPath := doc.CreateElement("div")
	infoPath.SetID("camera-path")
	infoPath.SetStyle("color", "#aaa")
	cameraInfo.Append(infoPath)

	infoBackend := doc.CreateElement("div")
	infoBackend.SetID("camera-backend")
	infoBackend.SetStyle("color", "#aaa")
	cameraInfo.Append(infoBackend)

	content.Append(cameraInfo)

	// Device mappings display
	deviceMappings := doc.CreateElement("div")
	deviceMappings.SetID("device-mappings")
	deviceMappings.SetStyle("padding", "12px")
	deviceMappings.SetStyle("background-color", "rgba(255,255,255,0.05)")
	deviceMappings.SetStyle("border-radius", "6px")
	deviceMappings.SetStyle("font-size", "13px")
	deviceMappings.SetStyle("display", "none")

	mappingsLabel := doc.CreateElement("div")
	mappingsLabel.SetStyle("font-weight", "500")
	mappingsLabel.SetStyle("margin-bottom", "8px")
	mappingsLabel.SetTextContent("Mapped Devices")
	deviceMappings.Append(mappingsLabel)

	mappingsList := doc.CreateElement("div")
	mappingsList.SetID("device-mappings-list")
	mappingsList.SetStyle("display", "flex")
	mappingsList.SetStyle("flex-direction", "column")
	mappingsList.SetStyle("gap", "6px")
	deviceMappings.Append(mappingsList)

	content.Append(deviceMappings)

	card.SetContent(content)
	return card.Element
}

func createCaptureActions() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("gap", "12px")
	content.SetStyle("flex-wrap", "wrap")

	// Capture button
	captureBtn := components.NewButton(components.ButtonConfig{
		Text:    "Capture Image",
		Class:   "btn-primary",
		OnClick: handleCapture,
	})
	captureBtn.Element.SetID("capture-button")
	captureBtn.SetDisabled(true)
	content.Append(captureBtn.Element)

	// Refresh cameras button
	refreshBtn := components.NewButton(components.ButtonConfig{
		Text:    "Refresh Cameras",
		Class:   "btn-secondary",
		OnClick: handleRefreshCameras,
	})
	content.Append(refreshBtn.Element)

	card := components.NewCard(components.CardConfig{
		Title:   "Actions",
		Content: content,
	})
	return card.Element
}

func createCaptureGallery() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetID("capture-gallery")
	content.SetClass("gallery")

	// Loading state
	loading := doc.CreateElement("div")
	loading.SetID("gallery-loading")
	loading.SetClass("loading")
	loading.SetTextContent("Loading recent captures...")
	content.Append(loading)

	card := components.NewCard(components.CardConfig{
		Title:   "Recent Captures",
		Content: content,
	})
	return card.Element
}

func handleRefreshCameras(_ *dom.Event) {
	loadCameras()
}

func handleCapture(_ *dom.Event) {
	doc := dom.GlobalDocument()
	cameraSelect := doc.GetElementByID("camera-select")
	if cameraSelect == nil {
		return
	}

	cameraID := cameraSelect.GetValue()
	if cameraID == "" {
		return
	}

	// Disable capture button during capture
	captureBtn := doc.GetElementByID("capture-button")
	if captureBtn != nil {
		captureBtn.SetDisabled(true)
		captureBtn.SetTextContent("Capturing...")
	}

	// Create capture request
	req := api.CaptureRequest{
		CameraID: cameraID,
	}

	// Call capture API
	api.CaptureImage(req, func(resp *api.CaptureResponse, err error) {
		if captureBtn != nil {
			captureBtn.SetDisabled(false)
			captureBtn.SetTextContent("Capture Image")
		}

		if err != nil {
			showCaptureError(err.Error())
			return
		}

		if resp.Status == "success" {
			showCaptureSuccess(resp.Path)
			loadCaptures() // Refresh gallery
		} else {
			showCaptureError("Capture failed: " + resp.Status)
		}
	})
}

func showCaptureError(message string) {
	doc := dom.GlobalDocument()

	errorDiv := doc.GetElementByID("capture-error")
	if errorDiv != nil {
		errorDiv.Remove()
	}

	errorDiv = doc.CreateElement("div")
	errorDiv.SetID("capture-error")
	errorDiv.SetClass("error-message")
	errorDiv.SetTextContent(message)

	// Insert after actions card
	actionsCard := doc.QuerySelector(".card:nth-child(2)")
	if actionsCard != nil && actionsCard.GetParent() != nil {
		actionsCard.GetParent().InsertBefore(errorDiv, actionsCard.GetNextSibling())
	}
}

func showCaptureSuccess(path string) {
	doc := dom.GlobalDocument()

	successDiv := doc.GetElementByID("capture-success")
	if successDiv != nil {
		successDiv.Remove()
	}

	successDiv = doc.CreateElement("div")
	successDiv.SetID("capture-success")
	successDiv.SetStyle("background-color", "rgba(76, 209, 135, 0.2)")
	successDiv.SetStyle("color", "#4cd137")
	successDiv.SetStyle("padding", "12px")
	successDiv.SetStyle("border-radius", "6px")
	successDiv.SetStyle("margin-bottom", "16px")
	successDiv.SetTextContent("Image captured: " + path)

	// Insert after actions card
	actionsCard := doc.QuerySelector(".card:nth-child(2)")
	if actionsCard != nil && actionsCard.GetParent() != nil {
		actionsCard.GetParent().InsertBefore(successDiv, actionsCard.GetNextSibling())
	}

	// Auto-hide after 3 seconds
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if successDiv != nil {
			successDiv.Remove()
		}
		return nil
	}), 3000)
}

func loadCameras() {
	doc := dom.GlobalDocument()
	loading := doc.GetElementByID("camera-loading")
	if loading != nil {
		loading.SetStyle("display", "block")
	}

	api.GetCameras(func(cameras []api.Camera, err error) {
		if loading != nil {
			loading.SetStyle("display", "none")
		}

		if err != nil {
			showCaptureError("Failed to load cameras: " + err.Error())
			return
		}

		cameraSelect := doc.GetElementByID("camera-select")
		if cameraSelect == nil {
			return
		}

		// Clear existing options
		cameraSelect.RemoveChildren()

		// Add default option
		defaultOption := doc.CreateElement("option")
		defaultOption.SetTextContent("-- Select Camera --")
		defaultOption.SetAttribute("value", "")
		cameraSelect.Append(defaultOption)

		// Sort cameras by name
		sort.Slice(cameras, func(i, j int) bool {
			return cameras[i].Name < cameras[j].Name
		})

		// Add camera options
		for _, cam := range cameras {
			option := doc.CreateElement("option")
			option.SetAttribute("value", cam.ID)
			option.SetTextContent(cam.Name)
			cameraSelect.Append(option)
		}

		// Enable capture button if cameras available
		captureBtn := doc.GetElementByID("capture-button")
		if captureBtn != nil && len(cameras) > 0 {
			captureBtn.SetDisabled(false)
		}

		// Add change listener for camera info display
		cameraSelect.AddEventListener("change", func(evt *dom.Event) {
			selectedID := cameraSelect.GetValue()
			displayCameraInfo(selectedID, cameras)
		})
	})
}

func displayCameraInfo(cameraID string, cameras []api.Camera) {
	doc := dom.GlobalDocument()
	cameraInfo := doc.GetElementByID("camera-info")

	if cameraID == "" {
		if cameraInfo != nil {
			cameraInfo.SetStyle("display", "none")
		}
		return
	}

	// Find camera
	var selectedCam *api.Camera
	for i := range cameras {
		if cameras[i].ID == cameraID {
			selectedCam = &cameras[i]
			break
		}
	}

	if selectedCam == nil || cameraInfo == nil {
		return
	}

	// Update info display
	nameEl := doc.GetElementByID("camera-name")
	pathEl := doc.GetElementByID("camera-path")
	backendEl := doc.GetElementByID("camera-backend")

	if nameEl != nil {
		nameEl.SetTextContent("Name: " + selectedCam.Name)
	}
	if pathEl != nil {
		pathEl.SetTextContent("Path: " + selectedCam.Path)
	}
	if backendEl != nil {
		backendEl.SetTextContent("Backend: " + selectedCam.Backend)
	}

	cameraInfo.SetStyle("display", "block")

	// Load device mappings for this camera
	loadDeviceMappings(cameraID)
}

func loadCaptures() {
	doc := dom.GlobalDocument()
	loading := doc.GetElementByID("gallery-loading")
	gallery := doc.GetElementByID("capture-gallery")

	api.GetCaptures(func(captures []api.Capture, err error) {
		if loading != nil {
			loading.Remove()
		}

		if err != nil {
			if gallery != nil {
				gallery.SetTextContent("Error loading captures")
			}
			return
		}

		if gallery == nil {
			return
		}

		if len(captures) == 0 {
			gallery.SetTextContent("No captures yet")
			return
		}

		// Clear loading state
		gallery.RemoveChildren()

		// Show recent captures (last 12)
		max := 12
		if len(captures) < max {
			max = len(captures)
		}

		for i := 0; i < max; i++ {
			cap := captures[i]
			item := createCaptureItem(cap)
			gallery.Append(item)
		}
	})
}

func createCaptureItem(cap api.Capture) *dom.Element {
	doc := dom.GlobalDocument()
	item := doc.CreateElement("div")
	item.SetClass("gallery-item")
	item.SetStyle("cursor", "pointer")
	item.SetAttribute("data-path", cap.Path)

	img := doc.CreateElement("img")
	img.SetAttribute("src", cap.Path)
	img.SetAttribute("alt", cap.Filename)
	img.SetStyle("width", "100%")
	img.SetStyle("height", "150px")
	img.SetStyle("object-fit", "cover")
	item.Append(img)

	info := doc.CreateElement("div")
	info.SetClass("gallery-item-info")

	filename := doc.CreateElement("div")
	filename.SetTextContent(cap.Filename)
	info.Append(filename)

	camera := doc.CreateElement("div")
	camera.SetStyle("color", "#aaa")
	camera.SetTextContent(cap.CameraName)
	info.Append(camera)

	item.Append(info)

	// Click to view
	item.AddEventListener("click", func(_ *dom.Event) {
		viewCapture(cap.Path)
	})

	return item
}

func viewCapture(path string) {
	doc := dom.GlobalDocument()

	// Remove existing modal if present
	existingModal := doc.GetElementByID("capture-viewer")
	if existingModal != nil {
		existingModal.Remove()
	}

	// Create modal for viewing
	modal := components.NewModal(components.ModalConfig{
		ID:       "capture-viewer",
		Closable: true,
	})

	img := doc.CreateElement("img")
	img.SetAttribute("src", path)
	img.SetStyle("max-width", "100%")
	img.SetStyle("max-height", "70vh")
	img.SetStyle("display", "block")
	img.SetStyle("margin", "0 auto")

	modal.SetContent(img)

	// Append modal to body
	doc.GetBody().Append(modal.Element)
	modal.Show()
}

// loadDeviceMappings loads and displays device mappings for a camera
func loadDeviceMappings(cameraID string) {
	doc := dom.GlobalDocument()
	deviceMappings := doc.GetElementByID("device-mappings")
	mappingsList := doc.GetElementByID("device-mappings-list")

	if deviceMappings == nil || mappingsList == nil {
		return
	}

	// Hide mappings section initially
	deviceMappings.SetStyle("display", "none")
	mappingsList.RemoveChildren()

	api.GetCameraMappings(cameraID, func(resp *api.CameraMappingsResponse, err error) {
		if err != nil || resp == nil {
			return
		}

		// Show mappings section if there are any
		if len(resp.Mappings) == 0 {
			noMappings := doc.CreateElement("div")
			noMappings.SetStyle("color", "#aaa")
			noMappings.SetStyle("font-style", "italic")
			noMappings.SetTextContent("No devices mapped to this camera")
			mappingsList.Append(noMappings)
			deviceMappings.SetStyle("display", "block")
			return
		}

		// Display each mapped device
		for _, mapping := range resp.Mappings {
			deviceCard := createDeviceMappingCard(mapping)
			mappingsList.Append(deviceCard)
		}

		deviceMappings.SetStyle("display", "block")
	})
}

// createDeviceMappingCard creates a card for a mapped device
func createDeviceMappingCard(mapping api.DeviceMappingWithDevice) *dom.Element {
	doc := dom.GlobalDocument()

	container := doc.CreateElement("div")
	container.SetStyle("display", "flex")
	container.SetStyle("justify-content", "space-between")
	container.SetStyle("align-items", "center")
	container.SetStyle("padding", "8px 12px")
	container.SetStyle("background-color", "rgba(255,255,255,0.03)")
	container.SetStyle("border-radius", "4px")
	container.SetStyle("border", "1px solid rgba(255,255,255,0.05)")

	// Device info
	info := doc.CreateElement("div")
	info.SetStyle("flex", "1")

	deviceID := doc.CreateElement("div")
	deviceID.SetStyle("font-weight", "500")
	deviceID.SetStyle("font-size", "13px")
	deviceID.SetTextContent(mapping.DeviceID)
	info.Append(deviceID)

	if mapping.Device != nil {
		chipType := doc.CreateElement("div")
		chipType.SetStyle("font-size", "12px")
		chipType.SetStyle("color", "#aaa")
		chipType.SetTextContent(mapping.Device.ChipType)
		info.Append(chipType)
	} else {
		chipType := doc.CreateElement("div")
		chipType.SetStyle("font-size", "12px")
		chipType.SetStyle("color", "#aaa")
		chipType.SetTextContent("Device")
		info.Append(chipType)
	}

	container.Append(info)

	// Calibration badge
	if mapping.CalibrationVersion > 0 {
		calibBadge := doc.CreateElement("div")
		calibBadge.SetStyle("font-size", "11px")
		calibBadge.SetStyle("padding", "2px 6px")
		calibBadge.SetStyle("background-color", "rgba(108, 92, 231, 0.2)")
		calibBadge.SetStyle("color", "#6c5ce7")
		calibBadge.SetStyle("border-radius", "4px")
		calibBadge.SetTextContent("v" + formatInt(mapping.CalibrationVersion))
		container.Append(calibBadge)
	}

	return container
}

// Initialize cameras when capture page loads
func initCapturePage() {
	loadCameras()
	loadCaptures()
}
