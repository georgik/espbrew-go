//go:build js
// +build js

package pages

import (
	"strings"
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/fileapi"
	"codeberg.org/georgik/espbrew-go/internal/project"
	"codeberg.org/georgik/espbrew-go/internal/ui/api"
	"codeberg.org/georgik/espbrew-go/internal/ui/components"
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
	"codeberg.org/georgik/espbrew-go/internal/ui/layout"
)

var (
	currentFileID      string
	currentJobID       string
	flashInProgress    bool
	currentProject     *project.WASMBuildArtifacts
	currentProjectType project.ProjectType
	currentDetector    project.WASMDetector
	// Multi-file flashing (ESP-IDF)
	bootloaderFileID string
	partitionsFileID string
	appFileID        string
	cachedDevices    []api.Device // Cache for device lookup
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
		Title: "Firmware Source",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "16px")

	// Upload mode tabs
	modeTabs := doc.CreateElement("div")
	modeTabs.SetStyle("display", "flex")
	modeTabs.SetStyle("gap", "8px")
	modeTabs.SetStyle("margin-bottom", "8px")

	// File mode button
	fileModeBtn := doc.CreateElement("button")
	fileModeBtn.SetID("mode-file-btn")
	fileModeBtn.SetClass("btn-secondary")
	fileModeBtn.SetStyle("flex", "1")
	fileModeBtn.SetTextContent("Single File")
	fileModeBtn.AddEventListener("click", func(_ *dom.Event) {
		switchUploadMode("file")
	})
	modeTabs.Append(fileModeBtn)

	// Folder mode button
	folderModeBtn := doc.CreateElement("button")
	folderModeBtn.SetID("mode-folder-btn")
	folderModeBtn.SetClass("btn-secondary")
	folderModeBtn.SetStyle("flex", "1")
	folderModeBtn.SetTextContent("Project Folder")
	folderModeBtn.AddEventListener("click", func(_ *dom.Event) {
		switchUploadMode("folder")
	})
	modeTabs.Append(folderModeBtn)

	content.Append(modeTabs)

	// Check File System Access API availability
	// API requires secure context (HTTPS or localhost)
	window := js.Global().Get("window")
	hasAPI := window.Get("showDirectoryPicker").Truthy()
	isSecureContext := window.Get("isSecureContext").Truthy()
	apiAvailable := hasAPI && isSecureContext

	// Show warning if API not available
	if !apiAvailable {
		warningDiv := doc.CreateElement("div")
		warningDiv.SetStyle("padding", "12px")
		warningDiv.SetStyle("background-color", "rgba(231, 76, 60, 0.1)")
		warningDiv.SetStyle("border-left", "3px solid #e74c3c")
		warningDiv.SetStyle("border-radius", "4px")
		warningDiv.SetStyle("font-size", "13px")
		warningDiv.SetStyle("color", "#e74c3c")

		if !hasAPI {
			warningDiv.SetTextContent("Project Folder not supported: Browser doesn't support File System Access API (use Chrome/Edge)")
		} else if !isSecureContext {
			warningDiv.SetTextContent("Project Folder requires HTTPS or localhost. Current connection is not secure. Use localhost address or enable HTTPS.")
		}

		content.Append(warningDiv)

		// Disable folder mode button
		folderModeBtn.SetDisabled(true)
		folderModeBtn.SetStyle("opacity", "0.5")
		folderModeBtn.SetStyle("cursor", "not-allowed")
	}

	// File input wrapper (single file mode)
	fileWrapper := doc.CreateElement("div")
	fileWrapper.SetID("file-upload-wrapper")
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

	// Folder picker wrapper (project mode - hidden initially)
	folderWrapper := doc.CreateElement("div")
	folderWrapper.SetID("folder-upload-wrapper")
	folderWrapper.SetStyle("display", "none")
	folderWrapper.SetStyle("flex-direction", "column")
	folderWrapper.SetStyle("gap", "8px")

	folderLabel := doc.CreateElement("label")
	folderLabel.SetTextContent("Select project folder:")
	folderLabel.SetStyle("font-weight", "500")
	folderWrapper.Append(folderLabel)

	// Folder picker button
	folderButton := doc.CreateElement("button")
	folderButton.SetID("folder-select-button")
	folderButton.SetClass("btn-secondary")
	folderButton.SetTextContent("Choose Project Folder")
	folderButton.AddEventListener("click", handleFolderSelect)
	folderWrapper.Append(folderButton)

	// Project info display
	projectInfo := doc.CreateElement("div")
	projectInfo.SetID("project-info")
	projectInfo.SetStyle("font-size", "13px")
	projectInfo.SetStyle("color", "#aaa")
	projectInfo.SetTextContent("No folder selected")
	folderWrapper.Append(projectInfo)

	// Detected artifacts display
	artifactsInfo := doc.CreateElement("div")
	artifactsInfo.SetID("artifacts-info")
	artifactsInfo.SetStyle("display", "none")
	artifactsInfo.SetStyle("padding", "8px")
	artifactsInfo.SetStyle("background-color", "rgba(108, 92, 231, 0.1)")
	artifactsInfo.SetStyle("border-radius", "4px")
	artifactsInfo.SetStyle("font-size", "13px")
	folderWrapper.Append(artifactsInfo)

	content.Append(folderWrapper)

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

	// Set initial mode
	switchUploadMode("file")

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
	// Check if we have project artifacts to upload first
	if currentProject != nil && currentProject.AppData != nil {
		uploadAndFlashProject()
		return
	}

	// Single file mode - need file ID
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

// uploadAndFlashProject uploads project binaries then flashes
func uploadAndFlashProject() {
	doc := dom.GlobalDocument()
	projectInfo := doc.GetElementByID("project-info")
	if projectInfo != nil {
		projectInfo.SetTextContent("Uploading binaries to server...")
		projectInfo.SetStyle("color", "#6c5ce7")
	}

	// Get selected device
	deviceSelect := doc.GetElementByID("flash-device-select")
	if deviceSelect == nil || deviceSelect.GetValue() == "" {
		showFlashError("Please select a device first")
		if projectInfo != nil {
			projectInfo.SetTextContent("Project loaded - select device to flash")
			projectInfo.SetStyle("color", "#aaa")
		}
		return
	}

	flashInProgress = true
	updateFlashButton()

	// Upload binaries and get file IDs - count only non-nil
	totalUploads := 0
	if currentProject.AppData != nil {
		totalUploads++
	}
	if currentProject.BootloaderData != nil {
		totalUploads++
	}
	if currentProject.PartitionsData != nil {
		totalUploads++
	}

	uploadCount := 0

	uploadBinary := func(name string, data []byte, isApp bool) {
		// Create Blob from binary data
		// Blob constructor expects array of blob parts, not raw Uint8Array
		uint8Array := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(uint8Array, data)
		blobArray := js.Global().Get("Array").New(uint8Array)
		blob := js.Global().Get("Blob").New(blobArray)

		// Create File object from Blob
		fileArray := js.Global().Get("Array").New(blob)
		file := js.Global().Get("File").New(fileArray, name)

		// Upload using existing API
		api.UploadFirmware(file, func(resp *api.FlashUploadResponse, err error) {
			if err != nil {
				js.Global().Get("console").Call("log", "Upload failed for", name, ":", err.Error())
				flashInProgress = false
				updateFlashButton()
				showFlashError("Upload failed for " + name + ": " + err.Error())
				return
			}

			js.Global().Get("console").Call("log", "Upload success for", name, "fileID:", resp.FileID)

			// Store file ID based on artifact type
			if isApp {
				appFileID = resp.FileID
				currentFileID = resp.FileID
			} else if name == "bootloader.bin" {
				bootloaderFileID = resp.FileID
			} else if name == "partitions.bin" {
				partitionsFileID = resp.FileID
			}

			uploadCount++
			js.Global().Get("console").Call("log", "Upload count:", uploadCount, "total:", totalUploads)
			if uploadCount == totalUploads {
				// All uploads complete - now flash
				js.Global().Get("console").Call("log", "All uploads complete, calling startFlashing")
				if projectInfo != nil {
					projectInfo.SetTextContent("Binaries uploaded! Starting flash...")
					projectInfo.SetStyle("color", "#4cd137")
				}
				startFlashing()
			}
		})
	}

	// Upload each binary
	if currentProject.AppData != nil {
		uploadBinary("app.bin", currentProject.AppData, true)
	}
	if currentProject.BootloaderData != nil {
		uploadBinary("bootloader.bin", currentProject.BootloaderData, false)
	}
	if currentProject.PartitionsData != nil {
		uploadBinary("partitions.bin", currentProject.PartitionsData, false)
	}
}

// startFlashing submits the flash job after uploads complete
func startFlashing() {
	doc := dom.GlobalDocument()

	js.Global().Get("console").Call("log", "startFlashing called, appFileID:", appFileID)

	// Show progress container
	progressContainer := doc.GetElementByID("flash-progress-container")
	if progressContainer != nil {
		progressContainer.SetStyle("display", "block")
	}

	selectedDevice := getSelectedDevice()
	if selectedDevice == nil {
		js.Global().Get("console").Call("log", "getSelectedDevice returned nil")
		flashInProgress = false
		updateFlashButton()
		showFlashError("Device selection lost")
		return
	}

	js.Global().Get("console").Call("log", "Selected device:", selectedDevice.DeviceID, "path:", selectedDevice.Path)

	// Submit flash job with app file
	// Use offset=65536 (0x10000) for standard app position in ESP-IDF format
	// Server will convert ELF to ESP-IDF format (bootloader + partitions + app)
	req := &api.FlashJobRequest{
		DevicePath: selectedDevice.Path,
		FileID:     appFileID,
		Offset:     65536, // 0x10000 - standard app offset
	}

	js.Global().Get("console").Call("log", "Submitting flash job:", req.DevicePath, req.FileID, "offset:", req.Offset)

	api.SubmitFlashJob(req, func(resp *api.FlashJobResponse, err error) {
		if err != nil {
			js.Global().Get("console").Call("log", "Flash job submission error:", err.Error())
			flashInProgress = false
			updateFlashButton()
			showFlashError("Flash job submission failed: " + err.Error())
			return
		}

		js.Global().Get("console").Call("log", "Flash job submitted successfully, jobID:", resp.JobID)
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

		// Check if complete (server returns "completed" or "succeeded")
		if progress.Status == "completed" || progress.Status == "succeeded" {
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

		// Cache devices for lookup
		cachedDevices = devices

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

	// Look up device from cache
	for _, dev := range cachedDevices {
		if dev.DeviceID == selectedID {
			return &dev
		}
	}

	return nil
}

func updateFlashButton() {
	doc := dom.GlobalDocument()
	flashBtn := doc.GetElementByID("flash-button")
	if flashBtn == nil {
		return
	}

	shouldEnable := !flashInProgress &&
		((currentFileID != "") || (currentProject != nil && currentProject.AppData != nil))

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

// switchUploadMode switches between file and folder upload modes
func switchUploadMode(mode string) {
	doc := dom.GlobalDocument()

	fileBtn := doc.GetElementByID("mode-file-btn")
	folderBtn := doc.GetElementByID("mode-folder-btn")
	fileWrapper := doc.GetElementByID("file-upload-wrapper")
	folderWrapper := doc.GetElementByID("folder-upload-wrapper")

	if fileBtn != nil {
		if mode == "file" {
			fileBtn.RemoveClass("btn-secondary")
			fileBtn.AddClass("btn-primary")
		} else {
			fileBtn.RemoveClass("btn-primary")
			fileBtn.AddClass("btn-secondary")
		}
	}

	if folderBtn != nil {
		if mode == "folder" {
			folderBtn.RemoveClass("btn-secondary")
			folderBtn.AddClass("btn-primary")
		} else {
			folderBtn.RemoveClass("btn-primary")
			folderBtn.AddClass("btn-secondary")
		}
	}

	if fileWrapper != nil {
		if mode == "file" {
			fileWrapper.SetStyle("display", "flex")
		} else {
			fileWrapper.SetStyle("display", "none")
		}
	}

	if folderWrapper != nil {
		if mode == "folder" {
			folderWrapper.SetStyle("display", "flex")
		} else {
			folderWrapper.SetStyle("display", "none")
		}
	}
}

// handleFolderSelect handles project folder selection
func handleFolderSelect(_ *dom.Event) {
	doc := dom.GlobalDocument()

	// Update UI to show loading
	projectInfo := doc.GetElementByID("project-info")
	if projectInfo != nil {
		projectInfo.SetTextContent("Opening folder picker...")
		projectInfo.SetStyle("color", "#6c5ce7")
	}

	picker := fileapi.NewFolderPicker()

	// Select folder using best available API with callback
	picker.SelectFolder(func(files *fileapi.FolderFiles, err error) {
		if err != nil {
			showFlashError("Folder selection failed: " + err.Error())
			return
		}

		if len(files.Files) == 0 {
			showFlashError("No files in selected folder")
			return
		}

		// Store folder handle for selective reading
		currentFolderHandle = files

		// Detect project type from marker files only
		detectProjectFromMarkerFiles(files)
	})
}

var currentFolderHandle *fileapi.FolderFiles

// Marker files needed for project detection
var markerFiles = []string{
	"CMakeLists.txt",
	"sdkconfig",
	"sdkconfig.defaults",
	"Cargo.toml",
	".cargo/config.toml",
	".cargo/config",
	"go.mod",
	"main.go",
}

// detectProjectFromMarkerFiles detects project using only marker files
func detectProjectFromMarkerFiles(folderFiles *fileapi.FolderFiles) {
	doc := dom.GlobalDocument()

	// Extract only marker files from the folder
	markerOnlyFiles := make(map[string][]byte)
	for _, marker := range markerFiles {
		if data, exists := folderFiles.Files[marker]; exists {
			markerOnlyFiles[marker] = data
		}
		// Also check for common nested paths
		for path, data := range folderFiles.Files {
			if path == marker || endsWithPath(path, "/"+marker) {
				markerOnlyFiles[path] = data
			}
		}
	}

	// Detect project type
	projType, detector, fileMap := project.DetectWASM(markerOnlyFiles)

	projectInfo := doc.GetElementByID("project-info")

	if projType == project.ProjectTypeNone || detector == nil {
		if projectInfo != nil {
			projectInfo.SetTextContent("No supported project detected (need ESP-IDF, Rust ESP, or TinyGo project)")
			projectInfo.SetStyle("color", "#e74c3c")
		}
		return
	}

	// Update project info
	if projectInfo != nil {
		projName := "Unknown"
		switch projType {
		case project.ProjectTypeESPIDF:
			projName = "ESP-IDF"
		case project.ProjectTypeRustESP:
			projName = "Rust ESP"
		case project.ProjectTypeTinyGo:
			projName = "TinyGo"
		}

		// Get chip type if available
		chipType := ""
		if d, ok := detector.(interface {
			GetChipType(*project.WASMFileMap) string
		}); ok {
			chipType = d.GetChipType(fileMap)
		}

		infoText := "Detected: " + projName
		if chipType != "" {
			infoText += " (" + chipType + ")"
		}
		projectInfo.SetTextContent(infoText)
		projectInfo.SetStyle("color", "#4cd137")
	}

	// Now selectively read only artifact files
	loadArtifactFiles(detector, fileMap)
}

// loadArtifactFiles loads only the binary files needed for flashing
func loadArtifactFiles(detector project.WASMDetector, fileMap *project.WASMFileMap) {
	// Get target triple for Rust projects
	targetTriple := ""
	if d, ok := detector.(interface {
		ExtractTargetTriple(*project.WASMFileMap) string
	}); ok {
		targetTriple = d.ExtractTargetTriple(fileMap)
	}

	// Use DirHandle to find ELF files at known paths
	elfPaths := []string{}
	if currentFolderHandle != nil {
		paths := currentFolderHandle.FindELFFilesInTarget(targetTriple)
		elfPaths = paths
		js.Global().Get("console").Call("log", "Found ELF files:", len(paths))
		for _, p := range paths {
			js.Global().Get("console").Call("log", "  -", p)
		}
	}

	// For ESP-IDF, also find .bin files in build/ directory
	binPaths := []string{}
	if currentFolderHandle != nil {
		paths := currentFolderHandle.FindBinFilesInBuild()
		binPaths = paths
		js.Global().Get("console").Call("log", "Found .bin files:", len(paths))
		for _, p := range paths {
			js.Global().Get("console").Call("log", "  -", p)
		}
	}

	// If we found files, re-detect with updated file map
	// (FindELFFilesInTarget and FindBinFilesInBuild already cached data)
	if len(elfPaths) > 0 || len(binPaths) > 0 {
		_, detector, fileMap = project.DetectWASM(currentFolderHandle.Files)
	}

	// Get artifact paths
	artifacts, err := detector.GetArtifacts(fileMap)
	if err != nil {
		showFlashError("Failed to get artifact paths: " + err.Error())
		return
	}

	// Read binary data for the artifacts
	selectiveBinaries := make(map[string][]byte)

	// Helper to read a single binary by path
	readBinary := func(path string) []byte {
		if currentFolderHandle == nil {
			return nil
		}
		// Check if already loaded
		if data, exists := currentFolderHandle.Files[path]; exists && len(data) > 0 {
			return data
		}
		// Use File System Access API to read on-demand
		data, err := currentFolderHandle.ReadSpecificFile(path)
		if err != nil {
			js.Global().Get("console").Call("log", "Failed to read", path, ":", err.Error())
			return nil
		}
		// Cache the loaded data
		currentFolderHandle.Files[path] = data
		return data
	}

	// Read artifacts
	if artifacts.Bootloader != "" {
		if data := readBinary(artifacts.Bootloader); data != nil {
			selectiveBinaries[artifacts.Bootloader] = data
		}
	}
	if artifacts.Partitions != "" {
		if data := readBinary(artifacts.Partitions); data != nil {
			selectiveBinaries[artifacts.Partitions] = data
		}
	}
	if artifacts.App != "" {
		if data := readBinary(artifacts.App); data != nil {
			selectiveBinaries[artifacts.App] = data
		}
	}

	// Display what we found
	displayArtifactInfo(artifacts, len(selectiveBinaries) > 0)

	// Store for flashing - binaries will be uploaded when Flash is clicked
	currentProject = &project.WASMBuildArtifacts{
		BootloaderData: selectiveBinaries[artifacts.Bootloader],
		PartitionsData: selectiveBinaries[artifacts.Partitions],
		AppData:        selectiveBinaries[artifacts.App],
		BootloaderPath: artifacts.Bootloader,
		PartitionsPath: artifacts.Partitions,
		AppPath:        artifacts.App,
	}
	currentProjectType = project.ProjectTypeRustESP // Will be set properly
	currentDetector = detector

	// Show result - binaries ready, will upload on Flash click
	if len(selectiveBinaries) > 0 {
		showFlashSuccess("Project detected! Ready to flash " + formatInt(len(selectiveBinaries)) + " binary files.")
		// Enable flash button when device is selected
		updateFlashButton()
	} else if len(elfPaths) > 0 {
		showFlashSuccess("Project detected! Found artifacts at: " + artifacts.App)
	} else {
		showFlashSuccess("Project detected, but no build artifacts found. Build the project first.")
	}
}

// uploadBinariesToServer uploads artifact binaries to the server
func uploadBinariesToServer(binaries map[string][]byte, artifacts *project.BuildArtifacts) {
	doc := dom.GlobalDocument()
	projectInfo := doc.GetElementByID("project-info")
	if projectInfo != nil {
		projectInfo.SetTextContent("Uploading binaries to server...")
		projectInfo.SetStyle("color", "#6c5ce7")
	}

	uploadCount := 0
	uploadBinary := func(name string, data []byte) {
		// Create Blob from binary data
		uint8Array := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(uint8Array, data)
		blob := js.Global().Get("Blob").New(uint8Array)

		// Create File object from Blob
		// File constructor expects: new File(fileBits, fileName)
		// fileBits must be an array-like object
		blobArray := js.Global().Get("Array").New(blob)
		file := js.Global().Get("File").New(blobArray, name)

		// Upload using existing API
		api.UploadFirmware(file, func(resp *api.FlashUploadResponse, err error) {
			if err != nil {
				js.Global().Get("console").Call("log", "Upload failed for", name, ":", err.Error())
				return
			}

			// Store file ID based on artifact type
			if name == "bootloader.bin" || strings.Contains(artifacts.Bootloader, "bootloader") {
				bootloaderFileID = resp.FileID
			} else if name == "partition-table.bin" || strings.Contains(artifacts.Partitions, "partition") {
				partitionsFileID = resp.FileID
			} else {
				appFileID = resp.FileID
				currentFileID = resp.FileID // For single-file compatibility
			}

			uploadCount++
			if uploadCount == len(binaries) {
				// All uploads complete
				if projectInfo != nil {
					projectInfo.SetTextContent("Binaries uploaded successfully!")
					projectInfo.SetStyle("color", "#4cd137")
				}
				updateFlashButton()
			}
		})
	}

	// Upload each binary
	for path, data := range binaries {
		// Extract filename from path
		parts := strings.Split(path, "/")
		name := parts[len(parts)-1]
		uploadBinary(name, data)
	}
}

// displayArtifactInfo shows detected artifact information
func displayArtifactInfo(artifacts *project.BuildArtifacts, hasBinaries bool) {
	doc := dom.GlobalDocument()
	artifactsInfo := doc.GetElementByID("artifacts-info")

	if artifactsInfo == nil {
		return
	}

	artifactsText := "Build artifacts:\n"
	if artifacts.Bootloader != "" {
		status := ""
		if hasBinaries {
			status = " (loaded)"
		}
		artifactsText += "- Bootloader: " + artifacts.Bootloader + status + "\n"
	}
	if artifacts.Partitions != "" {
		status := ""
		if hasBinaries {
			status = " (loaded)"
		}
		artifactsText += "- Partitions: " + artifacts.Partitions + status + "\n"
	}
	if artifacts.App != "" {
		status := ""
		if hasBinaries {
			status = " (loaded)"
		}
		artifactsText += "- App: " + artifacts.App + status + "\n"
	}

	if artifacts.FlashArgs != "" {
		artifactsText += "- Flash args: " + artifacts.FlashArgs + "\n"
	}

	artifactsInfo.SetInnerHTML("<pre style='margin:0;white-space:pre-wrap;font-size:12px;'>" + artifactsText + "</pre>")
	artifactsInfo.SetStyle("display", "block")
}

// endsWithPath checks if a path ends with a specific suffix
func endsWithPath(path, suffix string) bool {
	if len(path) < len(suffix) {
		return false
	}
	return path[len(path)-len(suffix):] == suffix
}
