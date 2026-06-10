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
	currentFileID   string
	currentJobID    string
	flashInProgress bool
)

// Flash renders the flash device page
func Flash(app *layout.App) {
	app.SetTitle("Flash Device")
	app.SetMainContentFunc(renderFlashContent)
}

func renderFlashContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Flash Firmware")
	container.Append(header)

	// Firmware upload card
	uploadCard := createFirmwareUpload()
	container.Append(uploadCard)

	// Device selection card
	deviceCard := createFlashDeviceSelector()
	container.Append(deviceCard)

	// Flash actions card
	actionCard := createFlashActions()
	container.Append(actionCard)

	// Progress card (hidden initially)
	progressCard := createFlashProgress()
	container.Append(progressCard)

	// Status messages area
	statusDiv := doc.CreateElement("div")
	statusDiv.SetID("flash-status")
	container.Append(statusDiv)

	return container
}

func createFirmwareUpload() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Firmware File",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// File input wrapper
	fileWrapper := doc.CreateElement("div")
	fileWrapper.SetStyle("display", "flex")
	fileWrapper.SetStyle("flex-direction", "column")
	fileWrapper.SetStyle("gap", "8px")

	label := doc.CreateElement("label")
	label.SetTextContent("Select firmware file (.bin):")
	label.SetStyle("font-weight", "500")
	fileWrapper.Append(label)

	// Hidden file input
	fileInput := doc.CreateElement("input")
	fileInput.SetAttribute("type", "file")
	fileInput.SetID("firmware-file")
	fileInput.SetStyle("display", "none")
	fileInput.SetAttribute("accept", ".bin")
	fileInput.AddEventListener("change", handleFileSelect)
	fileWrapper.Append(fileInput)

	// Custom file button
	fileButton := doc.CreateElement("button")
	fileButton.SetID("file-select-button")
	fileButton.SetClass("btn-secondary")
	fileButton.SetTextContent("Choose File")
	fileButton.AddEventListener("click", func(_ *dom.Event) {
		fileInput.Value().Call("click")
	})
	fileWrapper.Append(fileButton)

	// Selected file info
	fileInfo := doc.CreateElement("div")
	fileInfo.SetID("file-info")
	fileInfo.SetStyle("font-size", "13px")
	fileInfo.SetStyle("color", "#aaa")
	fileInfo.SetTextContent("No file selected")
	fileWrapper.Append(fileInfo)

	content.Append(fileWrapper)

	// File size info after upload
	sizeInfo := doc.CreateElement("div")
	sizeInfo.SetID("upload-size-info")
	sizeInfo.SetStyle("display", "none")
	sizeInfo.SetStyle("padding", "8px")
	sizeInfo.SetStyle("background-color", "rgba(76, 209, 135, 0.1)")
	sizeInfo.SetStyle("border-radius", "4px")
	sizeInfo.SetStyle("font-size", "13px")
	content.Append(sizeInfo)

	card.SetContent(content)
	return card.Element
}

func createFlashDeviceSelector() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Target Device",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// Device dropdown
	label := doc.CreateElement("label")
	label.SetTextContent("Select device to flash:")
	label.SetStyle("font-weight", "500")
	content.Append(label)

	selectWrapper := doc.CreateElement("div")
	selectWrapper.SetStyle("position", "relative")

	deviceSelect := doc.CreateElement("select")
	deviceSelect.SetID("flash-device-select")
	deviceSelect.SetStyle("width", "100%")
	deviceSelect.SetStyle("padding", "8px 12px")
	deviceSelect.SetStyle("border-radius", "6px")
	deviceSelect.SetStyle("background-color", "#161634")
	deviceSelect.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	deviceSelect.SetStyle("color", "#eee")
	deviceSelect.SetStyle("font-size", "14px")

	selectWrapper.Append(deviceSelect)

	// Loading state
	loading := doc.CreateElement("div")
	loading.SetID("flash-device-loading")
	loading.SetClass("loading")
	loading.SetTextContent("Loading devices...")
	loading.SetStyle("display", "none")
	selectWrapper.Append(loading)

	content.Append(selectWrapper)

	// Device info display
	deviceInfo := doc.CreateElement("div")
	deviceInfo.SetID("flash-device-info")
	deviceInfo.SetStyle("padding", "12px")
	deviceInfo.SetStyle("background-color", "rgba(255,255,255,0.05)")
	deviceInfo.SetStyle("border-radius", "6px")
	deviceInfo.SetStyle("font-size", "13px")
	deviceInfo.SetStyle("display", "none")

	infoPath := doc.CreateElement("div")
	infoPath.SetID("device-path")
	infoPath.SetStyle("font-weight", "500")
	infoPath.SetStyle("margin-bottom", "4px")
	deviceInfo.Append(infoPath)

	infoChip := doc.CreateElement("div")
	infoChip.SetID("device-chip")
	infoChip.SetStyle("color", "#aaa")
	deviceInfo.Append(infoChip)

	content.Append(deviceInfo)

	card.SetContent(content)
	return card.Element
}

func createFlashActions() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("gap", "12px")
	content.SetStyle("flex-wrap", "wrap")

	// Flash button
	flashBtn := components.NewButton(components.ButtonConfig{
		Text:    "Flash Device",
		Class:   "btn-primary",
		OnClick: handleFlashStart,
	})
	flashBtn.Element.SetID("flash-button")
	flashBtn.SetDisabled(true)
	content.Append(flashBtn.Element)

	// Refresh devices button
	refreshBtn := components.NewButton(components.ButtonConfig{
		Text:    "Refresh Devices",
		Class:   "btn-secondary",
		OnClick: handleRefreshDevices,
	})
	content.Append(refreshBtn.Element)

	card := components.NewCard(components.CardConfig{
		Title:   "Actions",
		Content: content,
	})
	return card.Element
}

func createFlashProgress() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetID("flash-progress-container")
	content.SetStyle("display", "none")

	// Progress bar
	progressBar := doc.CreateElement("div")
	progressBar.SetClass("progress-bar")
	progressBar.SetStyle("width", "100%")
	progressBar.SetStyle("height", "8px")
	progressBar.SetStyle("background-color", "rgba(255,255,255,0.1)")
	progressBar.SetStyle("border-radius", "4px")
	progressBar.SetStyle("overflow", "hidden")

	progressFill := doc.CreateElement("div")
	progressFill.SetID("flash-progress-fill")
	progressFill.SetStyle("width", "0%")
	progressFill.SetStyle("height", "100%")
	progressFill.SetStyle("background-color", "#6c5ce7")
	progressFill.SetStyle("transition", "width 0.3s")
	progressBar.Append(progressFill)

	content.Append(progressBar)

	// Progress text
	progressText := doc.CreateElement("div")
	progressText.SetID("flash-progress-text")
	progressText.SetStyle("margin-top", "8px")
	progressText.SetStyle("font-size", "13px")
	progressText.SetStyle("text-align", "center")
	progressText.SetTextContent("Preparing...")
	content.Append(progressText)

	card := components.NewCard(components.CardConfig{
		Title:   "Flash Progress",
		Content: content,
	})
	return card.Element
}

func handleFileSelect(_ *dom.Event) {
	doc := dom.GlobalDocument()
	fileInput := doc.GetElementByID("firmware-file")
	if fileInput == nil {
		return
	}

	files := fileInput.Value().Get("files")
	if files.IsUndefined() || files.Get("length").Int() == 0 {
		return
	}

	selectedFile := files.Index(0)
	fileName := selectedFile.Get("name").String()
	fileSize := int64(selectedFile.Get("size").Int())

	// Update file info
	fileInfo := doc.GetElementByID("file-info")
	if fileInfo != nil {
		// Format file size
		sizeText := formatFileSize(fileSize)
		fileInfo.SetTextContent(fileName + " (" + sizeText + ")")
		fileInfo.SetStyle("color", "#4cd137")
	}

	// Upload file
	uploadFirmware(selectedFile)
}

func uploadFirmware(file js.Value) {
	doc := dom.GlobalDocument()
	fileButton := doc.GetElementByID("file-select-button")

	// Show uploading state
	if fileButton != nil {
		fileButton.SetTextContent("Uploading...")
		fileButton.SetDisabled(true)
	}

	api.UploadFirmware(file, func(resp *api.FlashUploadResponse, err error) {
		if fileButton != nil {
			fileButton.SetTextContent("Choose File")
			fileButton.SetDisabled(false)
		}

		if err != nil {
			showFlashError("Upload failed: " + err.Error())
			return
		}

		currentFileID = resp.FileID

		// Show upload success
		sizeInfo := doc.GetElementByID("upload-size-info")
		if sizeInfo != nil {
			sizeInfo.SetTextContent("Firmware uploaded successfully (" + formatFileSize(resp.Size) + ")")
			sizeInfo.SetStyle("display", "block")
		}

		updateFlashButton()
	})
}

func handleRefreshDevices(_ *dom.Event) {
	loadFlashDevices()
}

func handleFlashStart(_ *dom.Event) {
	if flashInProgress || currentFileID == "" {
		return
	}

	doc := dom.GlobalDocument()
	deviceSelect := doc.GetElementByID("flash-device-select")
	if deviceSelect == nil {
		return
	}

	selectedDevice := getSelectedDevice()
	if selectedDevice == nil {
		showFlashError("Please select a device")
		return
	}

	flashInProgress = true
	updateFlashButton()

	// Show progress container
	progressContainer := doc.GetElementByID("flash-progress-container")
	if progressContainer != nil {
		progressContainer.SetStyle("display", "block")
	}

	// Submit flash job
	req := &api.FlashJobRequest{
		DevicePath: selectedDevice.Path,
		FileID:     currentFileID,
	}

	api.SubmitFlashJob(req, func(resp *api.FlashJobResponse, err error) {
		if err != nil {
			flashInProgress = false
			updateFlashButton()
			showFlashError("Flash job submission failed: " + err.Error())
			return
		}

		currentJobID = resp.JobID
		startProgressWatch()
	})
}

func startProgressWatch() {
	if currentJobID == "" {
		return
	}

	api.WatchFlashProgress(currentJobID, func(progress *api.FlashProgress) {
		updateFlashProgress(progress)

		// Check if complete
		if progress.Status == "completed" {
			flashInProgress = false
			currentJobID = ""
			updateFlashButton()
			showFlashSuccess("Device flashed successfully!")
		} else if progress.Status == "error" || progress.Status == "failed" {
			flashInProgress = false
			currentJobID = ""
			updateFlashButton()
			showFlashError("Flash failed: " + progress.Error)
		}
	})
}

func updateFlashProgress(progress *api.FlashProgress) {
	doc := dom.GlobalDocument()

	progressFill := doc.GetElementByID("flash-progress-fill")
	progressText := doc.GetElementByID("flash-progress-text")

	if progressFill != nil {
		progressFill.SetStyle("width", formatInt(progress.Progress)+"%")
	}

	if progressText != nil {
		if progress.Message != "" {
			progressText.SetTextContent(progress.Message + " (" + formatInt(progress.Progress) + "%)")
		} else {
			progressText.SetTextContent(formatInt(progress.Progress) + "%")
		}
	}
}

func loadFlashDevices() {
	doc := dom.GlobalDocument()
	loading := doc.GetElementByID("flash-device-loading")
	deviceSelect := doc.GetElementByID("flash-device-select")

	if loading != nil {
		loading.SetStyle("display", "block")
	}

	api.GetDevices(func(devices []api.Device, err error) {
		if loading != nil {
			loading.SetStyle("display", "none")
		}

		if err != nil {
			showFlashError("Failed to load devices: " + err.Error())
			return
		}

		if deviceSelect == nil {
			return
		}

		// Clear existing options
		deviceSelect.RemoveChildren()

		// Add default option
		defaultOption := doc.CreateElement("option")
		defaultOption.SetTextContent("-- Select Device --")
		defaultOption.SetAttribute("value", "")
		deviceSelect.Append(defaultOption)

		// Add device options
		for _, dev := range devices {
			option := doc.CreateElement("option")
			option.SetAttribute("value", dev.DeviceID)
			displayName := dev.DeviceID
			if len(dev.Aliases) > 0 {
				displayName = dev.Aliases[0]
			}
			option.SetTextContent(displayName + " (" + dev.ChipType + ")")
			deviceSelect.Append(option)
		}

		// Add change listener
		deviceSelect.AddEventListener("change", func(evt *dom.Event) {
			selectedID := deviceSelect.GetValue()
			displayFlashDeviceInfo(selectedID, devices)
		})
	})
}

func displayFlashDeviceInfo(deviceID string, devices []api.Device) {
	doc := dom.GlobalDocument()
	deviceInfo := doc.GetElementByID("flash-device-info")

	if deviceID == "" {
		if deviceInfo != nil {
			deviceInfo.SetStyle("display", "none")
		}
		updateFlashButton()
		return
	}

	// Find device
	var selectedDev *api.Device
	for i := range devices {
		if devices[i].DeviceID == deviceID {
			selectedDev = &devices[i]
			break
		}
	}

	if selectedDev == nil || deviceInfo == nil {
		updateFlashButton()
		return
	}

	// Update info display
	pathEl := doc.GetElementByID("device-path")
	chipEl := doc.GetElementByID("device-chip")

	if pathEl != nil {
		pathEl.SetTextContent("Path: " + selectedDev.Path)
	}
	if chipEl != nil {
		chipEl.SetTextContent("Chip: " + selectedDev.ChipType)
	}

	deviceInfo.SetStyle("display", "block")
	updateFlashButton()
}

func getSelectedDevice() *api.Device {
	doc := dom.GlobalDocument()
	deviceSelect := doc.GetElementByID("flash-device-select")
	if deviceSelect == nil {
		return nil
	}

	selectedID := deviceSelect.GetValue()
	if selectedID == "" {
		return nil
	}

	// This would need the cached devices list
	// For now, return a placeholder
	return &api.Device{
		DeviceID: selectedID,
		Path:     selectedID, // Will be filled by proper cache
		ChipType: "ESP32",
	}
}

func updateFlashButton() {
	doc := dom.GlobalDocument()
	flashBtn := doc.GetElementByID("flash-button")
	if flashBtn == nil {
		return
	}

	shouldEnable := currentFileID != "" && !flashInProgress

	deviceSelect := doc.GetElementByID("flash-device-select")
	if deviceSelect != nil && deviceSelect.GetValue() == "" {
		shouldEnable = false
	}

	flashBtn.SetDisabled(!shouldEnable)

	if flashInProgress {
		flashBtn.SetTextContent("Flashing...")
	} else {
		flashBtn.SetTextContent("Flash Device")
	}
}

func showFlashError(message string) {
	doc := dom.GlobalDocument()

	errorDiv := doc.GetElementByID("flash-status-error")
	if errorDiv != nil {
		errorDiv.Remove()
	}

	errorDiv = doc.CreateElement("div")
	errorDiv.SetID("flash-status-error")
	errorDiv.SetClass("error-message")
	errorDiv.SetTextContent(message)

	statusDiv := doc.GetElementByID("flash-status")
	if statusDiv != nil {
		statusDiv.Append(errorDiv)
	}

	// Auto-hide after 5 seconds
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if errorDiv != nil {
			errorDiv.Remove()
		}
		return nil
	}), 5000)
}

func showFlashSuccess(message string) {
	doc := dom.GlobalDocument()

	successDiv := doc.GetElementByID("flash-status-success")
	if successDiv != nil {
		successDiv.Remove()
	}

	successDiv = doc.CreateElement("div")
	successDiv.SetID("flash-status-success")
	successDiv.SetStyle("background-color", "rgba(76, 209, 135, 0.2)")
	successDiv.SetStyle("color", "#4cd137")
	successDiv.SetStyle("padding", "12px")
	successDiv.SetStyle("border-radius", "6px")
	successDiv.SetStyle("margin-top", "12px")
	successDiv.SetTextContent(message)

	statusDiv := doc.GetElementByID("flash-status")
	if statusDiv != nil {
		statusDiv.Append(successDiv)
	}

	// Auto-hide after 5 seconds
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if successDiv != nil {
			successDiv.Remove()
		}
		return nil
	}), 5000)
}

func formatFileSize(bytes int64) string {
	if bytes < 1024 {
		return formatInt(int(bytes)) + " B"
	}
	if bytes < 1024*1024 {
		return formatInt(int(bytes/1024)) + " KB"
	}
	return formatInt(int(bytes/(1024*1024))) + " MB"
}

// Initialize flash page when loaded
func initFlashPage() {
	loadFlashDevices()
}
