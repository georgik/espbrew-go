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

// CameraProperties renders the camera properties configuration page
func CameraProperties(app *layout.App) {
	app.SetTitle("Cameras")
	app.SetMainContentFunc(renderCameraPropertiesContent)
}

func renderCameraPropertiesContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Camera Properties")
	container.Append(header)

	// Create two-column layout: preview on left, controls on right
	layoutWrapper := doc.CreateElement("div")
	layoutWrapper.SetStyle("display", "grid")
	layoutWrapper.SetStyle("grid-template-columns", "1fr 1fr")
	layoutWrapper.SetStyle("gap", "20px")
	layoutWrapper.SetStyle("align-items", "start")

	// Left column: Camera selector and preview
	leftCol := doc.CreateElement("div")
	leftCol.SetStyle("display", "flex")
	leftCol.SetStyle("flex-direction", "column")
	leftCol.SetStyle("gap", "16px")

	// Camera selector card
	selectorCard := createCameraSelectorCard()
	leftCol.Append(selectorCard)

	// Preview card
	previewCard := createPreviewCard()
	leftCol.Append(previewCard)

	// Right column: Camera controls
	rightCol := doc.CreateElement("div")
	rightCol.SetID("camera-controls-column")
	rightCol.SetStyle("display", "flex")
	rightCol.SetStyle("flex-direction", "column")
	rightCol.SetStyle("gap", "16px")

	// Platform info
	platformInfo := doc.CreateElement("div")
	platformInfo.SetID("camera-platform-info")
	platformInfo.SetStyle("padding", "12px")
	platformInfo.SetStyle("background-color", "rgba(255,255,255,0.05)")
	platformInfo.SetStyle("border-radius", "6px")
	platformInfo.SetStyle("text-align", "center")
	platformInfo.SetStyle("color", "#aaa")
	platformInfo.SetTextContent("Select a camera to view controls")
	rightCol.Append(platformInfo)

	// Controls container
	controlsContainer := doc.CreateElement("div")
	controlsContainer.SetID("camera-controls-container")
	controlsContainer.SetStyle("display", "none")
	rightCol.Append(controlsContainer)

	// Action buttons
	actions := createCameraActions()
	rightCol.Append(actions)

	layoutWrapper.Append(leftCol)
	layoutWrapper.Append(rightCol)
	container.Append(layoutWrapper)

	// Initialize page
	initCameraPropertiesPage()

	return container
}

// createCameraSelectorCard creates the camera selection card
func createCameraSelectorCard() *dom.Element {
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

	cameraSelect := doc.CreateElement("select")
	cameraSelect.SetID("camera-properties-select")
	cameraSelect.SetStyle("width", "100%")
	cameraSelect.SetStyle("padding", "8px 12px")
	cameraSelect.SetStyle("border-radius", "6px")
	cameraSelect.SetStyle("background-color", "#161634")
	cameraSelect.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	cameraSelect.SetStyle("color", "#eee")
	cameraSelect.SetStyle("font-size", "14px")

	cameraSelect.AddEventListener("change", func(_ *dom.Event) {
		cameraID := cameraSelect.GetValue()
		if cameraID != "" {
			loadCameraProperties(cameraID)
		}
	})

	content.Append(cameraSelect)

	// Loading indicator
	loading := doc.CreateElement("div")
	loading.SetID("camera-loading")
	loading.SetClass("loading")
	loading.SetTextContent("Loading cameras...")
	loading.SetStyle("display", "none")
	content.Append(loading)

	card.SetContent(content)
	return card.Element
}

// createPreviewCard creates the camera preview card
func createPreviewCard() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Preview",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// Preview container
	previewContainer := doc.CreateElement("div")
	previewContainer.SetID("camera-preview-container")
	previewContainer.SetStyle("aspect-ratio", "16/9")
	previewContainer.SetStyle("background-color", "#000")
	previewContainer.SetStyle("border-radius", "8px")
	previewContainer.SetStyle("overflow", "hidden")
	previewContainer.SetStyle("display", "flex")
	previewContainer.SetStyle("align-items", "center")
	previewContainer.SetStyle("justify-content", "center")

	// Placeholder text
	placeholder := doc.CreateElement("div")
	placeholder.SetID("camera-preview-placeholder")
	placeholder.SetStyle("color", "#666")
	placeholder.SetTextContent("No camera selected")
	previewContainer.Append(placeholder)

	// Preview image (hidden initially)
	previewImage := doc.CreateElement("img")
	previewImage.SetID("camera-preview-image")
	previewImage.SetStyle("width", "100%")
	previewImage.SetStyle("height", "100%")
	previewImage.SetStyle("object-fit", "contain")
	previewImage.SetStyle("display", "none")
	previewContainer.Append(previewImage)

	// Loading overlay
	loadingOverlay := doc.CreateElement("div")
	loadingOverlay.SetID("camera-preview-loading")
	loadingOverlay.SetStyle("position", "absolute")
	loadingOverlay.SetStyle("display", "none")
	loadingOverlay.SetStyle("color", "#6c5ce7")
	loadingOverlay.SetTextContent("Capturing preview...")
	previewContainer.Append(loadingOverlay)

	content.Append(previewContainer)

	// Refresh preview button
	refreshBtn := components.NewButton(components.ButtonConfig{
		Text:    "Refresh Preview",
		Class:   "btn-secondary",
		OnClick: handleRefreshPreview,
	})
	refreshBtn.Element.SetID("refresh-preview-button")
	refreshBtn.SetDisabled(true)
	content.Append(refreshBtn.Element)

	card.SetContent(content)
	return card.Element
}

// createCameraActions creates action buttons
func createCameraActions() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetStyle("display", "flex")
	container.SetStyle("gap", "8px")
	container.SetStyle("flex-wrap", "wrap")

	// Apply button
	applyBtn := components.NewButton(components.ButtonConfig{
		Text:    "Apply Settings",
		Class:   "btn-primary",
		OnClick: handleApplyCameraSettings,
	})
	applyBtn.Element.SetID("apply-camera-settings")
	applyBtn.SetDisabled(true)
	container.Append(applyBtn.Element)

	// Refresh button
	refreshBtn := components.NewButton(components.ButtonConfig{
		Text:    "Refresh Controls",
		Class:   "btn-secondary",
		OnClick: handleRefreshCameraControls,
	})
	container.Append(refreshBtn.Element)

	// Save Profile button
	saveBtn := components.NewButton(components.ButtonConfig{
		Text:    "Save Profile",
		Class:   "btn-secondary",
		OnClick: handleSaveProfile,
	})
	saveBtn.Element.SetID("save-profile-button")
	saveBtn.SetDisabled(true)
	container.Append(saveBtn.Element)

	return container
}

// initCameraPropertiesPage initializes the page when loaded
func initCameraPropertiesPage() {
	// Load cameras into dropdown
	api.GetCameras(func(cameras []api.Camera, err error) {
		if err != nil {
			return
		}

		doc := dom.GlobalDocument()
		cameraSelect := doc.GetElementByID("camera-properties-select")
		if cameraSelect == nil {
			return
		}

		// Clear existing options
		cameraSelect.RemoveChildren()

		// Add default option
		defaultOption := doc.CreateElement("option")
		defaultOption.SetAttribute("value", "")
		defaultOption.SetTextContent("-- Select Camera --")
		cameraSelect.Append(defaultOption)

		// Add camera options
		for _, camera := range cameras {
			option := doc.CreateElement("option")
			option.SetAttribute("value", camera.ID)
			option.SetTextContent(camera.Name)
			cameraSelect.Append(option)
		}
	})
}

// loadCameraProperties loads camera properties and preview
func loadCameraProperties(cameraID string) {
	doc := dom.GlobalDocument()

	// Show loading
	platformInfo := doc.GetElementByID("camera-platform-info")
	controlsContainer := doc.GetElementByID("camera-controls-container")

	if platformInfo != nil {
		platformInfo.SetTextContent("Loading camera controls...")
	}
	if controlsContainer != nil {
		controlsContainer.SetStyle("display", "none")
	}

	// Load camera controls
	api.GetCameraControls(cameraID, func(resp *api.CameraControlsResponse, err error) {
		if platformInfo != nil {
			if err != nil {
				platformInfo.SetTextContent("Failed to load camera controls")
				return
			}

			if !resp.Available {
				platformInfo.SetTextContent("Camera controls not available on " + resp.Platform)
				return
			}

			platformInfo.SetStyle("display", "none")
		}

		if controlsContainer == nil {
			return
		}

		controlsContainer.RemoveChildren()
		controlsContainer.SetStyle("display", "flex")
		controlsContainer.SetStyle("flex-direction", "column")
		controlsContainer.SetStyle("gap", "16px")

		// Create control groups
		displayGroup := createImageControls(resp)
		controlsContainer.Append(displayGroup)

		advancedGroup := createAdvancedControls(resp)
		controlsContainer.Append(advancedGroup)

		autoGroup := createAutoControls(resp)
		controlsContainer.Append(autoGroup)

		presetGroup := createPresetControls(resp)
		controlsContainer.Append(presetGroup)

		// Enable apply button
		applyBtn := doc.GetElementByID("apply-camera-settings")
		if applyBtn != nil {
			applyBtn.SetDisabled(false)
		}

		// Enable save profile button
		saveBtn := doc.GetElementByID("save-profile-button")
		if saveBtn != nil {
			saveBtn.SetDisabled(false)
		}

		// Enable and trigger preview refresh
		refreshBtn := doc.GetElementByID("refresh-preview-button")
		if refreshBtn != nil {
			refreshBtn.SetDisabled(false)
		}

		// Capture initial preview
		capturePreview(cameraID)
	})
}

// capturePreview captures a preview image from the camera
func capturePreview(cameraID string) {
	doc := dom.GlobalDocument()

	placeholder := doc.GetElementByID("camera-preview-placeholder")
	previewImage := doc.GetElementByID("camera-preview-image")
	loadingOverlay := doc.GetElementByID("camera-preview-loading")

	// Show loading
	if loadingOverlay != nil {
		loadingOverlay.SetStyle("display", "block")
	}

	// Capture with preview=true
	req := api.CaptureRequest{
		CameraID: cameraID,
		Width:    1280,
		Height:   720,
		Quality:  85,
		Format:   "jpg",
		Preview:  true,
	}

	api.CapturePreview(req, func(imageURL string, err error) {
		if loadingOverlay != nil {
			loadingOverlay.SetStyle("display", "none")
		}

		if err != nil {
			if placeholder != nil {
				placeholder.SetStyle("display", "block")
				placeholder.SetTextContent("Capture failed: " + err.Error())
			}
			if previewImage != nil {
				previewImage.SetStyle("display", "none")
			}
			showSettingsError("Failed to capture preview")
			return
		}

		// Update preview image with blob URL
		if previewImage != nil {
			// Revoke old URL if exists
			oldSrc := previewImage.GetAttribute("src")
			if oldSrc != "" {
				js.Global().Get("URL").Call("revokeObjectURL", oldSrc)
			}
			previewImage.SetAttribute("src", imageURL)
			previewImage.SetStyle("display", "block")
		}
		if placeholder != nil {
			placeholder.SetStyle("display", "none")
		}
	})
}

// handleRefreshPreview refreshes the camera preview
func handleRefreshPreview(_ *dom.Event) {
	doc := dom.GlobalDocument()
	cameraSelect := doc.GetElementByID("camera-properties-select")
	if cameraSelect == nil {
		return
	}

	cameraID := cameraSelect.GetValue()
	if cameraID == "" {
		showSettingsError("No camera selected")
		return
	}

	capturePreview(cameraID)
}

// handleRefreshCameraControls reloads camera controls
func handleRefreshCameraControls(_ *dom.Event) {
	doc := dom.GlobalDocument()
	cameraSelect := doc.GetElementByID("camera-properties-select")
	if cameraSelect == nil {
		return
	}

	cameraID := cameraSelect.GetValue()
	if cameraID != "" {
		loadCameraProperties(cameraID)
	}
}

// handleApplyCameraSettings applies the current settings to the camera
func handleApplyCameraSettings(_ *dom.Event) {
	doc := dom.GlobalDocument()
	cameraSelect := doc.GetElementByID("camera-properties-select")
	if cameraSelect == nil {
		return
	}

	cameraID := cameraSelect.GetValue()
	if cameraID == "" {
		showSettingsError("No camera selected")
		return
	}

	// Gather current control values
	settings := gatherCurrentSettings()

	// Disable apply button during operation
	applyBtn := doc.GetElementByID("apply-camera-settings")
	if applyBtn != nil {
		applyBtn.SetDisabled(true)
		applyBtn.SetTextContent("Applying...")
	}

	api.ApplyCameraSettings(cameraID, settings, func(success bool, err error) {
		if applyBtn != nil {
			applyBtn.SetDisabled(false)
			applyBtn.SetTextContent("Apply Settings")
		}

		if err != nil {
			showSettingsError("Failed to apply settings: " + err.Error())
			return
		}

		if success {
			showSettingsSuccess("Settings applied successfully")
			// Refresh preview after applying settings
			capturePreview(cameraID)
		} else {
			showSettingsError("Failed to apply settings")
		}
	})
}

// handleSaveProfile saves the current settings as a profile
func handleSaveProfile(_ *dom.Event) {
	doc := dom.GlobalDocument()
	cameraSelect := doc.GetElementByID("camera-properties-select")
	if cameraSelect == nil {
		return
	}

	cameraID := cameraSelect.GetValue()
	if cameraID == "" {
		showSettingsError("No camera selected")
		return
	}

	// Prompt for profile name using a simple modal
	profileName := promptForProfileName()
	if profileName == "" {
		return // User cancelled
	}

	// Gather current settings
	settings := gatherCurrentSettings()
	settings.Name = profileName

	// Disable save button during operation
	saveBtn := doc.GetElementByID("save-profile-button")
	if saveBtn != nil {
		saveBtn.SetDisabled(true)
		saveBtn.SetTextContent("Saving...")
	}

	api.SaveCameraSettings(cameraID, settings, func(err error) {
		if saveBtn != nil {
			saveBtn.SetDisabled(false)
			saveBtn.SetTextContent("Save Profile")
		}

		if err != nil {
			showSettingsError("Failed to save profile: " + err.Error())
			return
		}

		showSettingsSuccess("Profile '" + profileName + "' saved successfully")
	})
}

// promptForProfileName prompts user for a profile name using browser prompt
func promptForProfileName() string {
	jsGlobal := js.Global()
	result := jsGlobal.Call("prompt", "Enter profile name:", "My Profile")
	if result.IsUndefined() || result.IsNull() {
		return ""
	}
	return result.String()
}

// gatherCurrentSettings collects values from all camera controls
func gatherCurrentSettings() *api.CameraSettingsRequest {
	settings := &api.CameraSettingsRequest{
		Name: "Manual Settings", // Required by backend
	}

	// Get slider values
	if brightness := getSliderValue("brightness-slider"); brightness != nil {
		settings.Brightness = *brightness
	}
	if contrast := getSliderValue("contrast-slider"); contrast != nil {
		settings.Contrast = *contrast
	}
	if saturation := getSliderValue("saturation-slider"); saturation != nil {
		settings.Saturation = *saturation
	}
	if sharpness := getSliderValue("sharpness-slider"); sharpness != nil {
		settings.Sharpness = *sharpness
	}
	if gain := getSliderValue("gain-slider"); gain != nil {
		settings.Gain = *gain
	}
	if focus := getSliderValue("focus-slider"); focus != nil {
		settings.Focus = *focus
	}
	if exposure := getSliderValue("exposure-slider"); exposure != nil {
		settings.Exposure = *exposure
	}
	if whiteBalance := getSliderValue("whitebalance-slider"); whiteBalance != nil {
		settings.WhiteBalance = *whiteBalance
	}

	// Get toggle values
	settings.AutoExposure = getToggleValue("autoexposure-toggle")
	settings.AutoFocus = getToggleValue("autofocus-toggle")
	settings.AutoWhiteBalance = getToggleValue("autowhitebalance-toggle")

	return settings
}

// getSliderValue retrieves value from a slider component
func getSliderValue(id string) *int32 {
	doc := dom.GlobalDocument()
	elem := doc.GetElementByID(id)
	if elem == nil {
		return nil
	}

	input := elem.QuerySelector("input[type=range]")
	if input != nil {
		valueStr := input.GetValue()
		var val int32
		for _, c := range valueStr {
			if c >= '0' && c <= '9' {
				val = val*10 + int32(c-'0')
			}
		}
		return &val
	}

	return nil
}

// getToggleValue retrieves value from a toggle component
func getToggleValue(id string) bool {
	doc := dom.GlobalDocument()
	elem := doc.GetElementByID(id)
	if elem == nil {
		return false
	}

	switchElem := elem.QuerySelector(".toggle-switch")
	return switchElem != nil && switchElem.HasClass("active")
}

// createImageControls creates image adjustment controls
func createImageControls(resp *api.CameraControlsResponse) *dom.Element {
	controls := []*dom.Element{}

	// Brightness slider
	if ctrlRange, ok := resp.Ranges["brightness"]; ok {
		slider := components.NewSlider(components.SliderConfig{
			ID:    "brightness-slider",
			Label: "Brightness",
			Min:   ctrlRange.Min,
			Max:   ctrlRange.Max,
			Value: ctrlRange.Current,
			Step:  1,
		})
		controls = append(controls, slider.Element)
	}

	// Contrast slider
	if ctrlRange, ok := resp.Ranges["contrast"]; ok {
		slider := components.NewSlider(components.SliderConfig{
			ID:    "contrast-slider",
			Label: "Contrast",
			Min:   ctrlRange.Min,
			Max:   ctrlRange.Max,
			Value: ctrlRange.Current,
			Step:  1,
		})
		controls = append(controls, slider.Element)
	}

	// Saturation slider
	if ctrlRange, ok := resp.Ranges["saturation"]; ok {
		slider := components.NewSlider(components.SliderConfig{
			ID:    "saturation-slider",
			Label: "Saturation",
			Min:   ctrlRange.Min,
			Max:   ctrlRange.Max,
			Value: ctrlRange.Current,
			Step:  1,
		})
		controls = append(controls, slider.Element)
	}

	// Sharpness slider
	if ctrlRange, ok := resp.Ranges["sharpness"]; ok {
		slider := components.NewSlider(components.SliderConfig{
			ID:    "sharpness-slider",
			Label: "Sharpness",
			Min:   ctrlRange.Min,
			Max:   ctrlRange.Max,
			Value: ctrlRange.Current,
			Step:  1,
		})
		controls = append(controls, slider.Element)
	}

	return components.NewControlGroup(components.ControlGroupConfig{
		Title:    "Image Controls",
		Controls: controls,
	}).Element
}

// createAdvancedControls creates advanced camera controls
func createAdvancedControls(resp *api.CameraControlsResponse) *dom.Element {
	controls := []*dom.Element{}

	// Gain slider
	if ctrlRange, ok := resp.Ranges["gain"]; ok {
		slider := components.NewSlider(components.SliderConfig{
			ID:    "gain-slider",
			Label: "Gain",
			Min:   ctrlRange.Min,
			Max:   ctrlRange.Max,
			Value: ctrlRange.Current,
			Step:  1,
		})
		controls = append(controls, slider.Element)
	}

	// Exposure slider
	if ctrlRange, ok := resp.Ranges["exposure_absolute"]; ok {
		slider := components.NewSlider(components.SliderConfig{
			ID:    "exposure-slider",
			Label: "Exposure",
			Min:   ctrlRange.Min,
			Max:   ctrlRange.Max,
			Value: ctrlRange.Current,
			Step:  1,
		})
		controls = append(controls, slider.Element)
	}

	// White Balance slider
	if ctrlRange, ok := resp.Ranges["white_balance"]; ok {
		slider := components.NewSlider(components.SliderConfig{
			ID:    "whitebalance-slider",
			Label: "White Balance",
			Min:   ctrlRange.Min,
			Max:   ctrlRange.Max,
			Value: ctrlRange.Current,
			Step:  1,
		})
		controls = append(controls, slider.Element)
	}

	return components.NewControlGroup(components.ControlGroupConfig{
		Title:    "Advanced Controls",
		Controls: controls,
	}).Element
}

// createAutoControls creates automatic control toggles
func createAutoControls(resp *api.CameraControlsResponse) *dom.Element {
	controls := []*dom.Element{}

	// Auto Exposure toggle
	autoExp := components.NewToggle(components.ToggleConfig{
		ID:      "autoexposure-toggle",
		Label:   "Auto Exposure",
		Checked: false,
	})
	controls = append(controls, autoExp.Element)

	// Auto Focus toggle
	autoFocus := components.NewToggle(components.ToggleConfig{
		ID:      "autofocus-toggle",
		Label:   "Auto Focus",
		Checked: false,
	})
	controls = append(controls, autoFocus.Element)

	// Auto White Balance toggle
	autoWB := components.NewToggle(components.ToggleConfig{
		ID:      "autowhitebalance-toggle",
		Label:   "Auto White Balance",
		Checked: false,
	})
	controls = append(controls, autoWB.Element)

	return components.NewControlGroup(components.ControlGroupConfig{
		Title:    "Automatic Controls",
		Controls: controls,
	}).Element
}

// createPresetControls creates preset buttons
func createPresetControls(resp *api.CameraControlsResponse) *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetStyle("display", "flex")
	container.SetStyle("flex-direction", "column")
	container.SetStyle("gap", "12px")

	// Preset header
	header := doc.CreateElement("div")
	header.SetClass("control-group-header")
	header.SetTextContent("Presets")
	container.Append(header)

	// Preset buttons
	buttonContainer := doc.CreateElement("div")
	buttonContainer.SetClass("preset-buttons")

	// Display Preset button
	displayBtn := doc.CreateElement("button")
	displayBtn.SetClass("preset-btn")
	displayBtn.SetTextContent("Display Preset")
	displayBtn.AddEventListener("click", func(_ *dom.Event) {
		applyDisplayPreset(resp)
	})
	buttonContainer.Append(displayBtn)

	// Focus Preset buttons
	if len(resp.FocusPresets) > 0 {
		focusLabel := doc.CreateElement("span")
		focusLabel.SetStyle("font-size", "12px")
		focusLabel.SetStyle("color", "#aaa")
		focusLabel.SetTextContent("Focus:")
		buttonContainer.Append(focusLabel)

		for presetName := range resp.FocusPresets {
			presetBtn := doc.CreateElement("button")
			presetBtn.SetClass("preset-btn")
			presetBtn.SetTextContent(presetName)
			presetBtn.AddEventListener("click", func(name string, value int32) func(_ *dom.Event) {
				return func(_ *dom.Event) {
					applyFocusPreset(name, value)
				}
			}(presetName, resp.FocusPresets[presetName]))
			buttonContainer.Append(presetBtn)
		}
	}

	container.Append(buttonContainer)

	return container
}

// applyDisplayPreset applies the display preset values
func applyDisplayPreset(resp *api.CameraControlsResponse) {
	doc := dom.GlobalDocument()

	// Apply display preset values to sliders
	for key, value := range resp.DisplayPreset {
		sliderID := key + "-slider"
		sliderElem := doc.GetElementByID(sliderID)
		if sliderElem != nil {
			input := sliderElem.QuerySelector("input[type=range]")
			if input != nil {
				input.SetAttribute("value", formatInt32(value))
				// Trigger input event to update value display
				input.Value().Call("dispatchEvent", js.Global().Get("Event").New("input"))
			}
		}
	}
}

// applyFocusPreset applies a focus preset value
func applyFocusPreset(name string, value int32) {
	doc := dom.GlobalDocument()

	focusSlider := doc.GetElementByID("focus-slider")
	if focusSlider != nil {
		input := focusSlider.QuerySelector("input[type=range]")
		if input != nil {
			input.SetAttribute("value", formatInt32(value))
			input.Value().Call("dispatchEvent", js.Global().Get("Event").New("input"))
		}
	}

	showSettingsSuccess("Focus preset '" + name + "' applied")
}

// showSettingsError displays an error message
func showSettingsError(message string) {
	doc := dom.GlobalDocument()

	errorDiv := doc.GetElementByID("camera-error")
	if errorDiv != nil {
		errorDiv.Remove()
	}

	errorDiv = doc.CreateElement("div")
	errorDiv.SetID("camera-error")
	errorDiv.SetStyle("position", "fixed")
	errorDiv.SetStyle("top", "20px")
	errorDiv.SetStyle("right", "20px")
	errorDiv.SetStyle("background-color", "rgba(231, 76, 60, 0.9)")
	errorDiv.SetStyle("color", "#fff")
	errorDiv.SetStyle("padding", "12px 20px")
	errorDiv.SetStyle("border-radius", "8px")
	errorDiv.SetStyle("box-shadow", "0 4px 12px rgba(0,0,0,0.3)")
	errorDiv.SetStyle("z-index", "1000")
	errorDiv.SetStyle("max-width", "300px")
	errorDiv.SetTextContent(message)

	doc.GetBody().Append(errorDiv)

	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if errorDiv != nil {
			errorDiv.Remove()
		}
		return nil
	}), 5000)
}

// showSettingsSuccess displays a success message
func showSettingsSuccess(message string) {
	doc := dom.GlobalDocument()

	successDiv := doc.GetElementByID("camera-success")
	if successDiv != nil {
		successDiv.Remove()
	}

	successDiv = doc.CreateElement("div")
	successDiv.SetID("camera-success")
	successDiv.SetStyle("position", "fixed")
	successDiv.SetStyle("top", "20px")
	successDiv.SetStyle("right", "20px")
	successDiv.SetStyle("background-color", "rgba(76, 209, 135, 0.9)")
	successDiv.SetStyle("color", "#fff")
	successDiv.SetStyle("padding", "12px 20px")
	successDiv.SetStyle("border-radius", "8px")
	successDiv.SetStyle("box-shadow", "0 4px 12px rgba(0,0,0,0.3)")
	successDiv.SetStyle("z-index", "1000")
	successDiv.SetStyle("max-width", "300px")
	successDiv.SetTextContent(message)

	doc.GetBody().Append(successDiv)

	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if successDiv != nil {
			successDiv.Remove()
		}
		return nil
	}), 3000)
}

// formatInt32 formats an int32 as a string
func formatInt32(n int32) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	if n < 100 {
		tens := n / 10
		ones := n % 10
		return string(rune('0'+tens)) + string(rune('0'+ones))
	}
	result := ""
	for n > 0 {
		digit := n % 10
		result = string(rune('0'+digit)) + result
		n /= 10
	}
	if result == "" {
		return "0"
	}
	return result
}
