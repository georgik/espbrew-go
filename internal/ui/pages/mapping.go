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

var (
	currentEditor *components.BoundingBoxEditor
	pendingBoxes  = make(map[string]*components.BoundingBox) // Store box bounds by ID
)

// Mapping renders the device mapping page
func Mapping(app *layout.App) {
	app.SetTitle("Device Mapping")
	app.SetMainContentFunc(renderMappingContent)
}

func renderMappingContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Device Mapping")
	container.Append(header)

	// Info card
	infoCard := createMappingInfo()
	container.Append(infoCard)

	// Camera selector card
	cameraCard := createMappingCameraSelector()
	container.Append(cameraCard)

	// Mode toggle card
	modeCard := createMappingModeToggle()
	container.Append(modeCard)

	// Editor container (shown in edit mode)
	editorContainer := doc.CreateElement("div")
	editorContainer.SetID("mapping-editor-container")
	editorContainer.SetStyle("display", "none")
	container.Append(editorContainer)

	// Mappings display card (shown in view mode)
	mappingsCard := createMappingsDisplay()
	mappingsCard.SetID("mappings-display-card")
	container.Append(mappingsCard)

	return container
}

func createMappingInfo() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetStyle("font-size", "14px")
	content.SetStyle("line-height", "1.6")
	content.SetStyle("color", "#bbb")

	para := doc.CreateElement("p")
	para.SetTextContent("Device mappings associate cameras with devices and define screen regions for monitoring. Select a camera to view its current mappings.")
	content.Append(para)

	card := components.NewCard(components.CardConfig{
		Title:   "About Device Mapping",
		Content: content,
	})
	return card.Element
}

func createMappingCameraSelector() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Select Camera",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// Camera dropdown
	label := doc.CreateElement("label")
	label.SetTextContent("Camera to view mappings:")
	label.SetStyle("font-weight", "500")
	content.Append(label)

	selectWrapper := doc.CreateElement("div")
	selectWrapper.SetStyle("position", "relative")

	cameraSelect := doc.CreateElement("select")
	cameraSelect.SetID("mapping-camera-select")
	cameraSelect.SetStyle("width", "100%")
	cameraSelect.SetStyle("max-width", "400px")
	cameraSelect.SetStyle("padding", "8px 12px")
	cameraSelect.SetStyle("border-radius", "6px")
	cameraSelect.SetStyle("background-color", "#161634")
	cameraSelect.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	cameraSelect.SetStyle("color", "#eee")
	cameraSelect.SetStyle("font-size", "14px")

	selectWrapper.Append(cameraSelect)

	// Loading state
	loading := doc.CreateElement("div")
	loading.SetID("mapping-camera-loading")
	loading.SetClass("loading")
	loading.SetTextContent("Loading cameras...")
	loading.SetStyle("display", "none")
	selectWrapper.Append(loading)

	content.Append(selectWrapper)

	// View mappings button
	viewBtn := components.NewButton(components.ButtonConfig{
		Text:    "View Mappings",
		Class:   "btn-primary",
		OnClick: handleViewMappings,
	})
	viewBtn.Element.SetID("view-mappings-button")
	viewBtn.SetDisabled(true)
	content.Append(viewBtn.Element)

	card.SetContent(content)
	return card.Element
}

func createMappingsDisplay() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetID("mappings-display")

	// Empty state
	emptyState := doc.CreateElement("div")
	emptyState.SetID("mappings-empty")
	emptyState.SetClass("empty-state")
	emptyState.SetTextContent("Select a camera to view its device mappings")
	content.Append(emptyState)

	// Loading state
	loading := doc.CreateElement("div")
	loading.SetID("mappings-loading")
	loading.SetClass("loading")
	loading.SetTextContent("Loading mappings...")
	loading.SetStyle("display", "none")
	content.Append(loading)

	// Mappings list
	mappingsList := doc.CreateElement("div")
	mappingsList.SetID("mappings-list")
	mappingsList.SetStyle("display", "none")
	content.Append(mappingsList)

	card := components.NewCard(components.CardConfig{
		Title:   "Device Mappings",
		Content: content,
	})
	return card.Element
}

func handleViewMappings(_ *dom.Event) {
	doc := dom.GlobalDocument()
	cameraSelect := doc.GetElementByID("mapping-camera-select")
	if cameraSelect == nil {
		return
	}

	cameraID := cameraSelect.GetValue()
	if cameraID == "" {
		return
	}

	loadMappings(cameraID)
}

func loadMappings(cameraID string) {
	doc := dom.GlobalDocument()

	// Show loading
	loading := doc.GetElementByID("mappings-loading")
	emptyState := doc.GetElementByID("mappings-empty")
	mappingsList := doc.GetElementByID("mappings-list")

	if loading != nil {
		loading.SetStyle("display", "block")
	}
	if emptyState != nil {
		emptyState.SetStyle("display", "none")
	}
	if mappingsList != nil {
		mappingsList.SetStyle("display", "none")
	}

	// Fetch mappings for camera
	api.GetCameraMappings(cameraID, func(resp *api.CameraMappingsResponse, err error) {
		if loading != nil {
			loading.SetStyle("display", "none")
		}

		if err != nil {
			showMappingsError("Failed to load mappings: " + err.Error())
			return
		}

		displayMappings(resp)

		// Show editor link
		editorLink := doc.GetElementByID("mapping-editor-link")
		if editorLink != nil {
			editorLink.SetStyle("display", "inline-block")
			editorLink.SetAttribute("href", "/?camera="+cameraID)
		}
	})
}

func displayMappings(resp *api.CameraMappingsResponse) {
	doc := dom.GlobalDocument()
	mappingsList := doc.GetElementByID("mappings-list")
	emptyState := doc.GetElementByID("mappings-empty")

	if mappingsList == nil {
		return
	}

	if len(resp.Mappings) == 0 {
		if emptyState != nil {
			emptyState.SetTextContent("No device mappings found for this camera. Use the Mapping Editor to create mappings.")
			emptyState.SetStyle("display", "block")
		}
		return
	}

	if emptyState != nil {
		emptyState.SetStyle("display", "none")
	}

	mappingsList.RemoveChildren()
	mappingsList.SetStyle("display", "block")

	// Calibration info
	if resp.Calibration != nil {
		calHeader := doc.CreateElement("div")
		calHeader.SetStyle("font-size", "14px")
		calHeader.SetStyle("font-weight", "500")
		calHeader.SetStyle("margin-bottom", "12px")
		calHeader.SetTextContent("Calibration Version " + formatInt(resp.Calibration.Version))
		if resp.Calibration.Description != "" {
			calHeader.SetTextContent(calHeader.GetTextContent() + ": " + resp.Calibration.Description)
		}
		mappingsList.Append(calHeader)
	}

	// Mappings count
	countHeader := doc.CreateElement("div")
	countHeader.SetStyle("font-size", "13px")
	countHeader.SetStyle("color", "#aaa")
	countHeader.SetStyle("margin-bottom", "12px")
	countHeader.SetTextContent("Found " + formatInt(len(resp.Mappings)) + " device mappings")
	mappingsList.Append(countHeader)

	// Mappings grid
	grid := doc.CreateElement("div")
	grid.SetStyle("display", "grid")
	grid.SetStyle("grid-template-columns", "repeat(auto-fill, minmax(250px, 1fr))")
	grid.SetStyle("gap", "12px")

	for _, mapping := range resp.Mappings {
		card := createMappingCard(mapping)
		grid.Append(card)
	}

	mappingsList.Append(grid)
}

func createMappingCard(mapping api.DeviceMappingWithDevice) *dom.Element {
	doc := dom.GlobalDocument()
	card := doc.CreateElement("div")
	card.SetStyle("background-color", "#161634")
	card.SetStyle("border-radius", "6px")
	card.SetStyle("padding", "12px")
	card.SetStyle("border", "1px solid rgba(255,255,255,0.1)")

	// Device info header
	header := doc.CreateElement("div")
	header.SetStyle("font-weight", "500")
	header.SetStyle("margin-bottom", "8px")

	deviceName := mapping.DeviceID
	if mapping.Device != nil && mapping.Device.Aliases != nil && len(mapping.Device.Aliases) > 0 {
		deviceName = mapping.Device.Aliases[0]
	}
	header.SetTextContent(deviceName)
	card.Append(header)

	// Camera info
	cameraInfo := doc.CreateElement("div")
	cameraInfo.SetStyle("font-size", "12px")
	cameraInfo.SetStyle("color", "#aaa")
	cameraInfo.SetStyle("margin-bottom", "8px")
	cameraInfo.SetTextContent("Camera: " + mapping.CameraName)
	card.Append(cameraInfo)

	// Bounds info
	boundsInfo := doc.CreateElement("div")
	boundsInfo.SetStyle("font-size", "12px")
	boundsInfo.SetStyle("margin-bottom", "8px")
	boundsInfo.SetTextContent("Region: " + formatBounds(mapping.Bounds))
	card.Append(boundsInfo)

	// Adjustment info (if any)
	if mapping.Adjustment.Brightness != 0 || mapping.Adjustment.Contrast != 0 || mapping.Adjustment.Saturation != 0 {
		adjInfo := doc.CreateElement("div")
		adjInfo.SetStyle("font-size", "12px")
		adjInfo.SetStyle("color", "#aaa")
		adjInfo.SetTextContent("Adjustments: " + formatAdjustment(mapping.Adjustment))
		card.Append(adjInfo)
	}

	// Status badge
	statusBadge := doc.CreateElement("div")
	statusBadge.SetStyle("display", "inline-block")
	statusBadge.SetStyle("padding", "4px 8px")
	statusBadge.SetStyle("border-radius", "4px")
	statusBadge.SetStyle("font-size", "11px")
	statusBadge.SetStyle("margin-top", "8px")
	statusBadge.SetStyle("background-color", "rgba(76, 209, 135, 0.2)")
	statusBadge.SetStyle("color", "#4cd137")
	statusBadge.SetTextContent("Active")
	card.Append(statusBadge)

	return card
}

func formatBounds(bounds api.BoundingBox) string {
	return "X:" + formatFloat(bounds.X) + " Y:" + formatFloat(bounds.Y) +
		" W:" + formatFloat(bounds.Width) + " H:" + formatFloat(bounds.Height)
}

func formatFloat(f float64) string {
	if f >= 100 {
		return "100%"
	}
	return formatInt(int(f*100)) + "%"
}

func formatAdjustment(adj api.ImageAdjustment) string {
	parts := []string{}
	if adj.Brightness != 0 {
		parts = append(parts, "B"+formatInt(adj.Brightness))
	}
	if adj.Contrast != 0 {
		parts = append(parts, "C"+formatInt(adj.Contrast))
	}
	if adj.Saturation != 0 {
		parts = append(parts, "S"+formatInt(adj.Saturation))
	}
	if len(parts) == 0 {
		return "None"
	}
	return parts[0]
}

func showMappingsError(message string) {
	doc := dom.GlobalDocument()

	errorDiv := doc.GetElementByID("mappings-error")
	if errorDiv != nil {
		errorDiv.Remove()
	}

	errorDiv = doc.CreateElement("div")
	errorDiv.SetID("mappings-error")
	errorDiv.SetClass("error-message")
	errorDiv.SetTextContent(message)

	mappingsDisplay := doc.GetElementByID("mappings-display")
	if mappingsDisplay != nil {
		mappingsDisplay.Append(errorDiv)
	}

	// Auto-hide after 5 seconds
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if errorDiv != nil {
			errorDiv.Remove()
		}
		return nil
	}), 5000)
}

func loadMappingCameras() {
	doc := dom.GlobalDocument()
	loading := doc.GetElementByID("mapping-camera-loading")
	cameraSelect := doc.GetElementByID("mapping-camera-select")

	if loading != nil {
		loading.SetStyle("display", "block")
	}

	api.GetCameras(func(cameras []api.Camera, err error) {
		if loading != nil {
			loading.SetStyle("display", "none")
		}

		if err != nil {
			showMappingsError("Failed to load cameras: " + err.Error())
			return
		}

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

		// Enable view button
		viewBtn := doc.GetElementByID("view-mappings-button")
		if viewBtn != nil && len(cameras) > 0 {
			viewBtn.SetDisabled(false)
		}

		// Restore shared camera selection if available
		if app != nil && app.HasSelectedCamera() {
			sharedCameraID := app.GetSelectedCameraID()
			// Verify the shared camera is still available
			cameraExists := false
			for _, cam := range cameras {
				if cam.ID == sharedCameraID {
					cameraExists = true
					break
				}
			}
			if cameraExists {
				cameraSelect.SetValue(sharedCameraID)
			}
		}

		// Add change listener to save to shared state
		cameraSelect.AddEventListener("change", func(_ *dom.Event) {
			selectedID := cameraSelect.GetValue()
			if app != nil {
				app.SetSelectedCameraID(selectedID)
			}
		})
	})
}

// Initialize mapping page when loaded
func initMappingPage() {
	loadMappingCamerasWithAutoLoad()
}

func loadMappingCamerasWithAutoLoad() {
	doc := dom.GlobalDocument()
	loading := doc.GetElementByID("mapping-camera-loading")
	cameraSelect := doc.GetElementByID("mapping-camera-select")

	if loading != nil {
		loading.SetStyle("display", "block")
	}

	api.GetCameras(func(cameras []api.Camera, err error) {
		if loading != nil {
			loading.SetStyle("display", "none")
		}

		if err != nil {
			showMappingsError("Failed to load cameras: " + err.Error())
			return
		}

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

		// Enable view button
		viewBtn := doc.GetElementByID("view-mappings-button")
		if viewBtn != nil && len(cameras) > 0 {
			viewBtn.SetDisabled(false)
		}

		// Auto-select first camera and load its mappings
		if len(cameras) > 0 {
			firstCameraID := cameras[0].ID
			cameraSelect.SetValue(firstCameraID)
			if app != nil {
				app.SetSelectedCameraID(firstCameraID)
			}
			loadMappings(firstCameraID)
		}

		// Add change listener to save to shared state
		cameraSelect.AddEventListener("change", func(_ *dom.Event) {
			selectedID := cameraSelect.GetValue()
			if app != nil {
				app.SetSelectedCameraID(selectedID)
			}
		})
	})
}

func createMappingModeToggle() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Edit Mode",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// Mode buttons
	modeButtons := doc.CreateElement("div")
	modeButtons.SetStyle("display", "flex")
	modeButtons.SetStyle("gap", "12px")

	viewBtn := components.NewButton(components.ButtonConfig{
		Text:    "View Mappings",
		Class:   "btn-secondary",
		OnClick: func(_ *dom.Event) { setMappingMode("view") },
	})
	viewBtn.Element.SetID("mode-view-button")
	viewBtn.Element.SetStyle("display", "none")
	modeButtons.Append(viewBtn.Element)

	editBtn := components.NewButton(components.ButtonConfig{
		Text:    "Edit Mappings",
		Class:   "btn-primary",
		OnClick: func(_ *dom.Event) { setMappingMode("edit") },
	})
	editBtn.Element.SetID("mode-edit-button")
	modeButtons.Append(editBtn.Element)

	content.Append(modeButtons)

	// Editor info
	infoText := doc.CreateElement("div")
	infoText.SetID("editor-info")
	infoText.SetStyle("font-size", "13px")
	infoText.SetStyle("color", "#aaa")
	infoText.SetTextContent("Click 'Edit Mappings' to draw device regions on a capture image")
	content.Append(infoText)

	card.SetContent(content)
	return card.Element
}

func setMappingMode(mode string) {
	doc := dom.GlobalDocument()

	editorContainer := doc.GetElementByID("mapping-editor-container")
	mappingsCard := doc.GetElementByID("mappings-display-card")
	viewBtn := doc.GetElementByID("mode-view-button")
	editBtn := doc.GetElementByID("mode-edit-button")

	if mode == "edit" {
		// Show editor, hide mappings
		if editorContainer != nil {
			editorContainer.SetStyle("display", "block")
		}
		if mappingsCard != nil {
			mappingsCard.SetStyle("display", "none")
		}
		if viewBtn != nil {
			viewBtn.SetStyle("display", "inline-block")
		}
		if editBtn != nil {
			editBtn.SetStyle("display", "none")
		}

		// Load editor with current camera
		cameraSelect := doc.GetElementByID("mapping-camera-select")
		if cameraSelect != nil {
			cameraID := cameraSelect.GetValue()
			if cameraID != "" {
				loadMappingEditor(cameraID)
			}
		}
	} else {
		// Show mappings, hide editor
		if editorContainer != nil {
			editorContainer.SetStyle("display", "none")
		}
		if mappingsCard != nil {
			mappingsCard.SetStyle("display", "block")
		}
		if viewBtn != nil {
			viewBtn.SetStyle("display", "none")
		}
		if editBtn != nil {
			editBtn.SetStyle("display", "inline-block")
		}
	}
}

func loadMappingEditor(cameraID string) {
	doc := dom.GlobalDocument()
	container := doc.GetElementByID("mapping-editor-container")
	if container == nil {
		return
	}

	container.RemoveChildren()

	// First, fetch devices for the dropdown
	api.GetDevices(func(devices []api.Device, err error) {
		if err != nil {
			showMappingsError("Failed to load devices: " + err.Error())
			// Continue anyway with empty device list
		}

		// Get recent capture for this camera
		api.GetCaptures(func(captures []api.Capture, err error) {
			if err != nil {
				showMappingsError("Failed to load captures: " + err.Error())
				return
			}

			// Find first capture for this camera
			var selectedCapture *api.Capture
			for i := range captures {
				if captures[i].CameraID == cameraID {
					selectedCapture = &captures[i]
					break
				}
			}

			if selectedCapture == nil {
				// No capture found, show message
				noCapture := doc.CreateElement("div")
				noCapture.SetStyle("padding", "20px")
				noCapture.SetStyle("text-align", "center")
				noCapture.SetStyle("color", "#aaa")
				noCapture.SetTextContent("No captures found for this camera. Capture an image first.")
				container.Append(noCapture)
				return
			}

			// Create capture selector
			captureSelector := createCaptureSelector(captures, cameraID, selectedCapture.Path)
			container.Append(captureSelector)

			// Create canvas editor with devices
			editor := components.NewBoundingBoxEditor(components.EditorConfig{
				CameraID: cameraID,
				ImageSrc: selectedCapture.Path,
				Devices:  devices,
				OnBoxCreate: func(box *components.BoundingBox, boxID string) {
					handleBoxCreate(box, boxID, cameraID, selectedCapture.Path)
				},
			})

			// Store editor reference for device assignment
			currentEditor = editor

			container.Append(editor.Element)

			// Load existing mappings
			api.GetCameraMappings(cameraID, func(resp *api.CameraMappingsResponse, err error) {
				if err == nil {
					editor.LoadMappings(resp.Mappings)
				}
			})
		})
	})
}

func createCaptureSelector(captures []api.Capture, cameraID, selectedPath string) *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetStyle("display", "flex")
	container.SetStyle("align-items", "center")
	container.SetStyle("gap", "12px")
	container.SetStyle("margin-bottom", "12px")

	label := doc.CreateElement("label")
	label.SetTextContent("Capture to edit:")
	label.SetStyle("font-weight", "500")
	container.Append(label)

	captureSelect := doc.CreateElement("select")
	captureSelect.SetID("editor-capture-select")
	captureSelect.SetStyle("padding", "6px 12px")
	captureSelect.SetStyle("border-radius", "4px")
	captureSelect.SetStyle("background-color", "#161634")
	captureSelect.SetStyle("border", "1px solid rgba(255,255,255,0.1")
	captureSelect.SetStyle("color", "#eee")

	for _, cap := range captures {
		if cap.CameraID == cameraID {
			option := doc.CreateElement("option")
			option.SetAttribute("value", cap.Path)
			option.SetTextContent(cap.Filename)
			if cap.Path == selectedPath {
				option.SetAttribute("selected", "true")
			}
			captureSelect.Append(option)
		}
	}

	captureSelect.AddEventListener("change", func(_ *dom.Event) {
		newPath := captureSelect.GetValue()
		// Reload editor with new capture
		cameraSelect := dom.GlobalDocument().GetElementByID("mapping-camera-select")
		if cameraSelect != nil {
			cameraID := cameraSelect.GetValue()

			// Get devices for the new editor
			api.GetDevices(func(devices []api.Device, err error) {
				if err != nil {
					// Continue with empty devices
					devices = []api.Device{}
				}

				editor := components.NewBoundingBoxEditor(components.EditorConfig{
					CameraID: cameraID,
					ImageSrc: newPath,
					Devices:  devices,
					OnBoxCreate: func(box *components.BoundingBox, boxID string) {
						handleBoxCreate(box, boxID, cameraID, newPath)
					},
				})

				// Store editor reference
				currentEditor = editor

				// Update editor in DOM
				editorContainer := dom.GlobalDocument().GetElementByID("mapping-editor-container")
				if editorContainer != nil {
					// Remove old editor and add new one
					oldEditor := editorContainer.QuerySelector(".bbox-editor")
					if oldEditor != nil {
						oldEditor.Remove()
					}
					editorContainer.Append(editor.Element)

					// Reload mappings
					api.GetCameraMappings(cameraID, func(resp *api.CameraMappingsResponse, err error) {
						if err == nil {
							editor.LoadMappings(resp.Mappings)
						}
					})
				}
			})
		}
	})

	container.Append(captureSelect)
	return container
}

func handleBoxCreate(box *components.BoundingBox, boxID, cameraID, capturePath string) {
	println("handleBoxCreate called with box:", boxID, "coords:", box.X, box.Y, box.Width, box.Height)

	// Store box bounds for later use when saving
	pendingBoxes[boxID] = box

	// Show device selection dialog
	showDeviceSelector(boxID, cameraID)
}

func showDeviceSelector(boxID, cameraID string) {
	doc := dom.GlobalDocument()

	println("showDeviceSelector called for box:", boxID, ", fetching devices...")

	// Get devices for selection
	api.GetDevices(func(devices []api.Device, err error) {
		if err != nil {
			println("Failed to load devices:", err.Error())
			showMappingsError("Failed to load devices: " + err.Error())
			return
		}

		println("Devices loaded, count:", len(devices))

		if len(devices) == 0 {
			// Show error if no devices available
			showMappingsError("No devices found. Connect devices first to create mappings.")
			return
		}

		// Create modal for device selection
		modal := components.NewModal(components.ModalConfig{
			ID:       "device-selector-modal",
			Closable: true,
		})

		content := doc.CreateElement("div")
		content.SetStyle("display", "flex")
		content.SetStyle("flex-direction", "column")
		content.SetStyle("gap", "12px")

		header := doc.CreateElement("div")
		header.SetStyle("font-weight", "500")
		header.SetTextContent("Select Device for this Region")
		content.Append(header)

		// Device list
		deviceList := doc.CreateElement("div")
		deviceList.SetStyle("display", "flex")
		deviceList.SetStyle("flex-direction", "column")
		deviceList.SetStyle("gap", "8px")
		deviceList.SetStyle("max-height", "400px")
		deviceList.SetStyle("overflow-y", "auto")

		for _, dev := range devices {
			deviceBtn := doc.CreateElement("button")
			deviceBtn.SetStyle("padding", "10px")
			deviceBtn.SetStyle("text-align", "left")
			deviceBtn.SetStyle("border-radius", "4px")
			deviceBtn.SetStyle("background-color", "#161634")
			deviceBtn.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
			deviceBtn.SetStyle("color", "#eee")
			deviceBtn.SetStyle("cursor", "pointer")

			displayName := dev.DeviceID
			if len(dev.Aliases) > 0 {
				displayName = dev.Aliases[0]
			}
			deviceBtn.SetTextContent(displayName + " (" + dev.ChipType + ")")

			deviceBtn.AddEventListener("click", func(_ *dom.Event) {
				// Assign device to the box on canvas
				if currentEditor != nil {
					currentEditor.AssignDevice(boxID, dev.DeviceID)
				}

				// Save mapping
				saveMappingForBox(boxID, cameraID, dev.DeviceID)

				modal.Close()
				modal.Element.Remove() // Clean up modal from DOM
			})

			deviceList.Append(deviceBtn)
		}

		content.Append(deviceList)

		modal.SetContent(content)

		// Append modal to body and show
		dom.GlobalDocument().GetBody().Append(modal.Element)
		modal.Show()

		println("Modal shown with", len(devices), "devices")
	})
}

func saveMappingForBox(boxID, cameraID, deviceID string) {
	// Get the stored box bounds
	box, ok := pendingBoxes[boxID]
	if !ok {
		showMappingsError("Box not found: " + boxID)
		return
	}

	bounds := api.BoundingBox{
		X:      box.X,
		Y:      box.Y,
		Width:  box.Width,
		Height: box.Height,
	}

	req := api.CreateMappingRequest{
		DeviceID: deviceID,
		CameraID: cameraID,
		Bounds:   bounds,
	}

	api.CreateMapping(req, func(resp *api.CreateMappingResponse, err error) {
		if err != nil {
			showMappingsError("Failed to save mapping: " + err.Error())
			return
		}

		showMappingsSuccess("Mapping saved successfully")

		// Clean up pending box
		delete(pendingBoxes, boxID)

		// Refresh mappings display
		cameraSelect := dom.GlobalDocument().GetElementByID("mapping-camera-select")
		if cameraSelect != nil {
			currentCameraID := cameraSelect.GetValue()
			if currentCameraID == cameraID {
				loadMappings(cameraID)
			}
		}
	})
}

func showMappingsSuccess(message string) {
	doc := dom.GlobalDocument()

	successDiv := doc.GetElementByID("mappings-success")
	if successDiv != nil {
		successDiv.Remove()
	}

	successDiv = doc.CreateElement("div")
	successDiv.SetID("mappings-success")
	successDiv.SetStyle("background-color", "rgba(76, 209, 135, 0.2)")
	successDiv.SetStyle("color", "#4cd137")
	successDiv.SetStyle("padding", "12px")
	successDiv.SetStyle("border-radius", "6px")
	successDiv.SetStyle("margin-bottom", "12px")
	successDiv.SetTextContent(message)

	mappingsDisplay := doc.GetElementByID("mappings-display")
	if mappingsDisplay != nil {
		mappingsDisplay.Append(successDiv)
	}

	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if successDiv != nil {
			successDiv.Remove()
		}
		return nil
	}), 3000)
}
