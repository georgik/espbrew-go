//go:build js
// +build js

package pages

import (
	"sort"
	"strings"
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/api"
	"codeberg.org/georgik/espbrew-go/internal/ui/components"
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
	"codeberg.org/georgik/espbrew-go/internal/ui/layout"
)

// Devices renders the device management page
func Devices(app *layout.App) {
	app.SetTitle("Devices")
	app.SetMainContentFunc(renderDevicesContent)
}

func renderDevicesContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Connected Devices")
	container.Append(header)

	// Devices list
	devicesCard := createDevicesListCard()
	container.Append(devicesCard)

	return container
}

func createDevicesListCard() *dom.Element {
	doc := dom.GlobalDocument()
	content := doc.CreateElement("div")
	content.SetID("devices-list")
	content.SetClass("devices-list")

	// Loading state
	loading := doc.CreateElement("div")
	loading.SetClass("loading")
	loading.SetTextContent("Loading devices...")
	content.Append(loading)

	card := components.NewCard(components.CardConfig{
		Title:   "Devices",
		Content: content,
	})
	return card.Element
}

func loadDevices() {
	doc := dom.GlobalDocument()
	loading := doc.GetElementByID("devices-loading")
	devicesList := doc.GetElementByID("devices-list")

	if loading != nil {
		loading.SetStyle("display", "block")
	}

	api.GetDevices(func(devices []api.Device, err error) {
		if loading != nil {
			loading.SetStyle("display", "none")
		}

		if err != nil {
			if devicesList != nil {
				devicesList.SetTextContent("Error loading devices: " + err.Error())
			}
			return
		}

		if devicesList == nil {
			return
		}

		if len(devices) == 0 {
			devicesList.SetTextContent("No devices connected")
			return
		}

		// Sort devices by path
		sort.Slice(devices, func(i, j int) bool {
			return devices[i].Path < devices[j].Path
		})

		// Clear loading state
		devicesList.RemoveChildren()

		// Create table header
		table := doc.CreateElement("div")
		table.SetStyle("display", "grid")
		table.SetStyle("grid-template-columns", "1fr 1fr 1fr 100px")
		table.SetStyle("gap", "12px")
		table.SetStyle("padding", "8px")
		table.SetStyle("background-color", "rgba(255,255,255,0.05)")
		table.SetStyle("border-radius", "6px")
		table.SetStyle("margin-bottom", "12px")
		table.SetStyle("font-weight", "500")
		table.SetStyle("font-size", "13px")

		headerPath := doc.CreateElement("div")
		headerPath.SetTextContent("Path")
		table.Append(headerPath)

		headerChip := doc.CreateElement("div")
		headerChip.SetTextContent("Chip Type")
		table.Append(headerChip)

		headerStatus := doc.CreateElement("div")
		headerStatus.SetTextContent("Status")
		table.Append(headerStatus)

		headerActions := doc.CreateElement("div")
		headerActions.SetTextContent("Actions")
		table.Append(headerActions)

		devicesList.Append(table)

		// Create device rows
		for _, dev := range devices {
			deviceRow := createDeviceRow(dev)
			devicesList.Append(deviceRow)
		}
	})
}

func createDeviceRow(dev api.Device) *dom.Element {
	doc := dom.GlobalDocument()
	row := doc.CreateElement("div")
	row.SetStyle("display", "grid")
	row.SetStyle("grid-template-columns", "1fr 1fr 1fr 100px")
	row.SetStyle("gap", "12px")
	row.SetStyle("padding", "12px 8px")
	row.SetStyle("background-color", "rgba(255,255,255,0.02)")
	row.SetStyle("border-radius", "4px")
	row.SetStyle("border", "1px solid rgba(255,255,255,0.05)")
	row.SetStyle("align-items", "center")
	row.SetStyle("font-size", "13px")
	row.SetAttribute("data-device-id", dev.DeviceID)

	// Add visual warning for access error
	if dev.AccessError != "" {
		row.SetStyle("border", "1px solid rgba(255, 71, 87, 0.5)")
		row.SetStyle("background-color", "rgba(255, 71, 87, 0.05)")
		row.SetAttribute("title", dev.AccessError)
	}

	// Device path
	pathDiv := doc.CreateElement("div")
	pathDiv.SetStyle("font-family", "monospace")
	pathDiv.SetTextContent(dev.Path)
	row.Append(pathDiv)

	// Chip type
	chipDiv := doc.CreateElement("div")
	if dev.ChipType != "" {
		chipDiv.SetTextContent(dev.ChipType)
	} else if dev.AccessError != "" {
		chipDiv.SetTextContent("N/A")
		chipDiv.SetStyle("color", "#ff4757")
		chipDiv.SetStyle("font-style", "italic")
	} else {
		chipDiv.SetTextContent("Unknown")
	}
	row.Append(chipDiv)

	// Status badge (or access error warning)
	statusDiv := doc.CreateElement("div")
	statusDiv.SetStyle("display", "inline-block")
	statusDiv.SetStyle("padding", "2px 8px")
	statusDiv.SetStyle("border-radius", "4px")
	statusDiv.SetStyle("font-size", "11px")
	if dev.Status == "available" {
		statusDiv.SetStyle("background-color", "rgba(76, 209, 135, 0.2)")
		statusDiv.SetStyle("color", "#4cd137")
		statusDiv.SetTextContent("Available")
	} else if dev.Status == "busy" {
		statusDiv.SetStyle("background-color", "rgba(255, 165, 2, 0.2)")
		statusDiv.SetStyle("color", "#ffa502")
		statusDiv.SetTextContent("Busy")
	} else {
		statusDiv.SetStyle("background-color", "rgba(255, 71, 87, 0.2)")
		statusDiv.SetStyle("color", "#ff4757")
		statusDiv.SetTextContent(dev.Status)
	}
	row.Append(statusDiv)

	// Actions
	actionsDiv := doc.CreateElement("div")
	actionsDiv.SetStyle("display", "flex")
	actionsDiv.SetStyle("gap", "6px")

	// Handle different device states
	if dev.DeviceID == "" {
		// Device without ID - show Probe and Forget buttons
		probeBtn := components.NewButton(components.ButtonConfig{
			Text:    "Probe",
			Class:   "btn-sm btn-primary",
			OnClick: func(_ *dom.Event) { probeDevice(dev.Path) },
		})
		actionsDiv.Append(probeBtn.Element)

		forgetBtn := components.NewButton(components.ButtonConfig{
			Text:  "Forget",
			Class: "btn-sm btn-danger",
			OnClick: func(_ *dom.Event) {
				result := js.Global().Get("window").Call("confirm", "Remove device "+dev.Path+" from list?")
				if result.Bool() {
					forgetDevice(dev.Path)
				}
			},
		})
		actionsDiv.Append(forgetBtn.Element)
	} else if dev.AccessError != "" {
		// Show warning icon for access error
		warningSpan := doc.CreateElement("span")
		warningSpan.SetTextContent("!")
		warningSpan.SetStyle("cursor", "help")
		warningSpan.SetAttribute("title", dev.AccessError)
		actionsDiv.Append(warningSpan)
	} else {
		// Edit button
		editBtn := components.NewButton(components.ButtonConfig{
			Text:    "Edit",
			Class:   "btn-sm btn-secondary",
			OnClick: func(_ *dom.Event) { editDevice(dev) },
		})
		actionsDiv.Append(editBtn.Element)
	}

	row.Append(actionsDiv)

	return row
}

func editDevice(dev api.Device) {
	doc := dom.GlobalDocument()

	// Remove existing modal
	existingModal := doc.GetElementByID("device-edit-modal")
	if existingModal != nil {
		existingModal.Remove()
	}

	// Create modal for editing device
	modal := components.NewModal(components.ModalConfig{
		ID:       "device-edit-modal",
		Closable: true,
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "16px")
	content.SetStyle("min-width", "400px")

	// Device header
	header := doc.CreateElement("div")
	header.SetStyle("font-weight", "500")
	header.SetTextContent("Edit Device: " + dev.Path)
	content.Append(header)

	// Device ID
	deviceIDRow := createFormField("Device ID", dev.DeviceID, true)
	content.Append(deviceIDRow)

	// Path
	pathRow := createFormField("Path", dev.Path, true)
	content.Append(pathRow)

	// Chip Type - editable selector
	chipRow := createChipTypeSelector(dev.ChipType)
	content.Append(chipRow)

	// Aliases
	aliasesStr := ""
	if len(dev.Aliases) > 0 {
		aliasesStr = joinStrings(dev.Aliases, ", ")
	}
	aliasesRow := createFormFieldWithID("Aliases", "device-aliases-input", aliasesStr, false)
	content.Append(aliasesRow)

	// MAC Address
	macRow := createFormField("MAC Address", dev.MACAddress, true)
	content.Append(macRow)

	// Protected status
	protectedRow := doc.CreateElement("div")
	protectedRow.SetStyle("display", "flex")
	protectedRow.SetStyle("justify-content", "space-between")
	protectedRow.SetStyle("align-items", "center")

	protectedLabel := doc.CreateElement("label")
	protectedLabel.SetTextContent("Protected")
	protectedRow.Append(protectedLabel)

	protectedToggle := doc.CreateElement("input")
	protectedToggle.SetAttribute("type", "checkbox")
	protectedToggle.SetAttribute("id", "device-protected")
	if dev.Protected {
		protectedToggle.SetAttribute("checked", "checked")
	}
	protectedRow.Append(protectedToggle)

	content.Append(protectedRow)

	// Backend configuration for virtual devices
	if dev.Backend != "" || strings.HasPrefix(dev.Path, "wokwi:") || strings.HasPrefix(dev.Path, "qemu:") {
		backendSection := doc.CreateElement("div")
		backendSection.SetStyle("margin-top", "12px")
		backendSection.SetStyle("padding-top", "12px")
		backendSection.SetStyle("border-top", "1px solid rgba(255,255,255,0.1)")

		backendHeader := doc.CreateElement("div")
		backendHeader.SetStyle("font-weight", "500")
		backendHeader.SetStyle("font-size", "13px")
		backendHeader.SetStyle("margin-bottom", "8px")
		backendHeader.SetTextContent("Backend Configuration")
		backendSection.Append(backendHeader)

		// Backend type
		backendType := dev.Backend
		if backendType == "" && strings.HasPrefix(dev.Path, "wokwi:") {
			backendType = "wokwi"
		} else if backendType == "" && strings.HasPrefix(dev.Path, "qemu:") {
			backendType = "qemu"
		}

		backendTypeRow := createFormField("Backend Type", backendType, true)
		backendSection.Append(backendTypeRow)

		// Diagram JSON for Wokwi
		if backendType == "wokwi" {
			diagramJSON := ""
			if dev.BackendConfig != nil {
				if dj, ok := dev.BackendConfig["diagram_json"].(string); ok {
					diagramJSON = dj
				}
			}

			diagramRow := doc.CreateElement("div")
			diagramRow.SetStyle("display", "flex")
			diagramRow.SetStyle("flex-direction", "column")
			diagramRow.SetStyle("gap", "4px")

			diagramLabel := doc.CreateElement("label")
			diagramLabel.SetTextContent("Diagram JSON")
			diagramLabel.SetStyle("font-size", "12px")
			diagramLabel.SetStyle("color", "#aaa")
			diagramRow.Append(diagramLabel)

			diagramTextarea := doc.CreateElement("textarea")
			diagramTextarea.SetAttribute("id", "device-diagram-json")
			diagramTextarea.SetTextContent(diagramJSON)
			diagramTextarea.SetStyle("background-color", "rgba(0,0,0,0.3)")
			diagramTextarea.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
			diagramTextarea.SetStyle("border-radius", "4px")
			diagramTextarea.SetStyle("padding", "8px")
			diagramTextarea.SetStyle("color", "#fff")
			diagramTextarea.SetStyle("font-family", "monospace")
			diagramTextarea.SetStyle("font-size", "11px")
			diagramTextarea.SetStyle("min-height", "80px")
			diagramTextarea.SetStyle("width", "100%")
			diagramRow.Append(diagramTextarea)

			backendSection.Append(diagramRow)
		}

		content.Append(backendSection)
	}

	// Actions
	actions := doc.CreateElement("div")
	actions.SetStyle("display", "flex")
	actions.SetStyle("gap", "8px")
	actions.SetStyle("justify-content", "flex-end")
	actions.SetStyle("margin-top", "8px")

	cancelBtn := components.NewButton(components.ButtonConfig{
		Text:  "Cancel",
		Class: "btn-secondary",
		OnClick: func(_ *dom.Event) {
			modal.Close()
		},
	})
	actions.Append(cancelBtn.Element)

	// Add delete button for virtual devices or manual devices
	if dev.Backend == "wokwi" || dev.Backend == "qemu" || strings.HasPrefix(dev.DeviceID, "manual-") {
		deleteBtn := components.NewButton(components.ButtonConfig{
			Text:  "Delete",
			Class: "btn-danger",
			OnClick: func(_ *dom.Event) {
				// Confirm deletion using JavaScript confirm
				result := js.Global().Get("window").Call("confirm", "Are you sure you want to delete device "+dev.DeviceID+"?")
				if result.Bool() {
					api.DeleteDevice(dev.DeviceID, func(success bool, err error) {
						if err != nil || !success {
							showError("Failed to delete device")
						} else {
							showSuccess("Device deleted successfully")
							modal.Close()
							loadDevices() // Refresh the list
						}
					})
				}
			},
		})
		actions.Append(deleteBtn.Element)
	}

	// Add Forget button for physical devices (removes from list but doesn't delete from persistence)
	if dev.Backend != "wokwi" && dev.Backend != "qemu" {
		forgetBtn := components.NewButton(components.ButtonConfig{
			Text:  "Forget",
			Class: "btn-warning",
			OnClick: func(_ *dom.Event) {
				result := js.Global().Get("window").Call("confirm", "Remove device "+dev.Path+" from list? (Device remains in database)")
				if result.Bool() {
					forgetDevice(dev.Path)
					modal.Close()
				}
			},
		})
		actions.Append(forgetBtn.Element)
	}

	saveBtn := components.NewButton(components.ButtonConfig{
		Text:  "Save",
		Class: "btn-primary",
		OnClick: func(_ *dom.Event) {
			saveDeviceAttributes(dev.DeviceID)
			modal.Close()
		},
	})
	actions.Append(saveBtn.Element)

	content.Append(actions)

	modal.SetContent(content)

	// Append modal to body
	doc.GetBody().Append(modal.Element)
	modal.Show()
}

func createFormField(label, value string, readonly bool) *dom.Element {
	return createFormFieldWithID(label, "", value, readonly)
}

func createFormFieldWithID(label, id, value string, readonly bool) *dom.Element {
	doc := dom.GlobalDocument()
	row := doc.CreateElement("div")
	row.SetStyle("display", "flex")
	row.SetStyle("flex-direction", "column")
	row.SetStyle("gap", "4px")

	labelElem := doc.CreateElement("label")
	labelElem.SetTextContent(label)
	labelElem.SetStyle("font-size", "12px")
	labelElem.SetStyle("color", "#aaa")
	row.Append(labelElem)

	input := doc.CreateElement("input")
	input.SetAttribute("type", "text")
	input.SetValue(value)
	if id != "" {
		input.SetAttribute("id", id)
	}
	input.SetStyle("padding", "6px 8px")
	input.SetStyle("border-radius", "4px")
	input.SetStyle("background-color", "#161634")
	input.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	input.SetStyle("color", "#eee")
	if readonly {
		input.SetAttribute("readonly", "readonly")
		input.SetStyle("opacity", "0.7")
	}
	row.Append(input)

	return row
}

// Chip type options for selector
var chipTypeOptions = []string{
	"ESP32",
	"ESP32-S2",
	"ESP32-S3",
	"ESP32-C3",
	"ESP32-C6",
	"ESP32-H2",
	"Custom",
}

func createChipTypeSelector(currentChipType string) *dom.Element {
	doc := dom.GlobalDocument()
	row := doc.CreateElement("div")
	row.SetStyle("display", "flex")
	row.SetStyle("flex-direction", "column")
	row.SetStyle("gap", "4px")

	labelElem := doc.CreateElement("label")
	labelElem.SetTextContent("Chip Type")
	labelElem.SetStyle("font-size", "12px")
	labelElem.SetStyle("color", "#aaa")
	row.Append(labelElem)

	selectElem := doc.CreateElement("select")
	selectElem.SetAttribute("id", "device-chip-type")
	selectElem.SetStyle("padding", "6px 8px")
	selectElem.SetStyle("border-radius", "4px")
	selectElem.SetStyle("background-color", "#161634")
	selectElem.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	selectElem.SetStyle("color", "#eee")

	// Add default option
	defaultOption := doc.CreateElement("option")
	defaultOption.SetAttribute("value", "")
	defaultOption.SetTextContent("Select chip...")
	selectElem.Append(defaultOption)

	// Check if current chip is a custom value (not in predefined list)
	isCustom := true
	for _, chip := range chipTypeOptions {
		if chip == "Custom" {
			continue
		}
		if chip == currentChipType {
			isCustom = false
			break
		}
	}

	// Add chip type options
	for _, chip := range chipTypeOptions {
		option := doc.CreateElement("option")
		option.SetAttribute("value", chip)
		option.SetTextContent(chip)
		if chip == currentChipType || (chip == "Custom" && isCustom && currentChipType != "") {
			option.SetAttribute("selected", "selected")
		}
		selectElem.Append(option)
	}

	// Add custom chip type input (hidden by default)
	customInput := doc.CreateElement("input")
	customInput.SetAttribute("id", "device-chip-type-custom")
	customInput.SetAttribute("type", "text")
	customInput.SetAttribute("placeholder", "Enter custom chip type...")
	customInput.SetStyle("padding", "6px 8px")
	customInput.SetStyle("border-radius", "4px")
	customInput.SetStyle("background-color", "#161634")
	customInput.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	customInput.SetStyle("color", "#eee")
	customInput.SetStyle("margin-top", "4px")
	customInput.SetStyle("display", "none")

	// If current chip is custom, show input and set value
	if isCustom && currentChipType != "" {
		customInput.SetStyle("display", "block")
		customInput.SetValue(currentChipType)
	}

	// Append elements to row
	row.Append(selectElem)
	row.Append(customInput)

	// Add event listener to show/hide custom input when selection changes
	selectElem.AddEventListener(dom.EventChange, func(e *dom.Event) {
		selectedValue := selectElem.GetValue()
		customElem := doc.GetElementByID("device-chip-type-custom")
		if customElem != nil {
			if selectedValue == "Custom" {
				customElem.SetStyle("display", "block")
			} else {
				customElem.SetStyle("display", "none")
			}
		}
	})

	return row
}

func saveDeviceAttributes(deviceID string) {
	doc := dom.GlobalDocument()
	aliasesInput := doc.QuerySelector("#device-aliases-input")
	protectedToggle := doc.QuerySelector("#device-protected")
	diagramTextarea := doc.QuerySelector("#device-diagram-json")
	chipTypeSelect := doc.QuerySelector("#device-chip-type")

	if aliasesInput == nil || protectedToggle == nil {
		showError("Failed to read form values")
		return
	}

	aliasesStr := aliasesInput.GetValue()
	aliases := []string{}
	if aliasesStr != "" {
		aliases = splitString(aliasesStr, ",")
	}

	protected := protectedToggle.GetChecked()

	// Update request for basic device attributes
	req := map[string]interface{}{
		"aliases":   aliases,
		"protected": protected,
	}

	// Add chip type if changed
	if chipTypeSelect != nil {
		chipType := chipTypeSelect.GetValue()
		if chipType == "Custom" {
			customInput := doc.QuerySelector("#device-chip-type-custom")
			if customInput != nil {
				customValue := customInput.GetValue()
				if customValue != "" {
					req["chip_type"] = customValue
				}
			}
		} else if chipType != "" {
			req["chip_type"] = chipType
		}
	}

	// Check if this is a wokwi device with diagram config
	if diagramTextarea != nil {
		diagramJSON := diagramTextarea.GetValue()
		if diagramJSON != "" {
			chipType := extractChipType(deviceID, diagramJSON)

			backendConfig := map[string]interface{}{
				"chip_type":    chipType,
				"diagram_json": diagramJSON,
			}

			api.SetBackendConfig(deviceID, "wokwi", backendConfig, func(success bool, err error) {
				if err != nil {
					showError("Failed to save diagram: " + err.Error())
				} else if !success {
					showError("Failed to save diagram")
				}
			})
		}
	}

	// Call update API for basic attributes
	api.UpdateDevice(deviceID, req, func(success bool, err error) {
		if err != nil || !success {
			showError("Failed to update device: " + err.Error())
		} else {
			showSuccess("Device updated successfully")
			loadDevices() // Refresh the list
		}
	})
}

func showError(message string) {
	showToast(message, "error")
}

func showSuccess(message string) {
	showToast(message, "success")
}

// extractChipType determines chip type from deviceID or diagram content
// Returns chip type in ESP32-S3 format (uppercase with hyphen)
func extractChipType(deviceID, diagramJSON string) string {
	// First try to extract from deviceID (wokwi:esp32-s3 -> ESP32-S3)
	if strings.HasPrefix(deviceID, "wokwi:") {
		chip := strings.ToUpper(deviceID[6:]) // Remove "wokwi:" and uppercase
		// If already in hyphenated format (ESP32-S3), return as is
		if strings.Contains(chip, "-") {
			return chip
		}
		// Normalize old format without hyphens
		switch chip {
		case "ESP32S3":
			return "ESP32-S3"
		case "ESP32C3":
			return "ESP32-C3"
		case "ESP32C6":
			return "ESP32-C6"
		case "ESP32":
			return "ESP32"
		default:
			// For unknown chips, insert hyphen after ESP32
			if len(chip) > 5 && chip[:5] == "ESP32" {
				return "ESP32-" + chip[5:]
			}
			return chip
		}
	}

	// Also handle old format (wokwi-esp32s3 -> ESP32-S3)
	if strings.HasPrefix(deviceID, "wokwi-") {
		chip := strings.ToUpper(deviceID[6:]) // Remove "wokwi-" and uppercase
		switch chip {
		case "ESP32S3":
			return "ESP32-S3"
		case "ESP32C3":
			return "ESP32-C3"
		case "ESP32C6":
			return "ESP32-C6"
		case "ESP32":
			return "ESP32"
		default:
			return chip
		}
	}

	// Fall back to diagram content detection
	lowerDiagram := strings.ToLower(diagramJSON)
	switch {
	case strings.Contains(lowerDiagram, "esp32-c3") || strings.Contains(diagramJSON, "ESP32-C3"):
		return "ESP32-C3"
	case strings.Contains(lowerDiagram, "esp32-c6") || strings.Contains(diagramJSON, "ESP32-C6"):
		return "ESP32-C6"
	case strings.Contains(lowerDiagram, "esp32-s3") || strings.Contains(diagramJSON, "ESP32-S3"):
		return "ESP32-S3"
	case strings.Contains(lowerDiagram, "esp32-s2") || strings.Contains(diagramJSON, "ESP32-S2"):
		return "ESP32-S2"
	case strings.Contains(lowerDiagram, "esp32") || strings.Contains(diagramJSON, "ESP32"):
		return "ESP32"
	default:
		return "ESP32-S3" // Default
	}
}

func showToast(message, toastType string) {
	doc := dom.GlobalDocument()

	// Remove existing toast
	existing := doc.GetElementByID("toast")
	if existing != nil {
		existing.Remove()
	}

	toast := doc.CreateElement("div")
	toast.SetID("toast")
	toast.SetTextContent(message)

	if toastType == "error" {
		toast.SetStyle("background-color", "rgba(255, 71, 87, 0.9)")
	} else if toastType == "info" {
		toast.SetStyle("background-color", "rgba(9, 132, 227, 0.9)")
	} else {
		toast.SetStyle("background-color", "rgba(76, 209, 135, 0.9)")
	}

	toast.SetStyle("position", "fixed")
	toast.SetStyle("top", "20px")
	toast.SetStyle("right", "20px")
	toast.SetStyle("padding", "12px 16px")
	toast.SetStyle("border-radius", "6px")
	toast.SetStyle("color", "#fff")
	toast.SetStyle("z-index", "1000")
	toast.SetStyle("box-shadow", "0 4px 12px rgba(0,0,0,0.3)")

	doc.GetBody().Append(toast)

	// Auto-hide after 3 seconds
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		toast.Remove()
		return nil
	}), 3000)
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	parts := []string{}
	current := ""
	for _, c := range s {
		if string(c) == sep {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// Initialize devices page
func initDevicesPage() {
	loadDevices()
}

// probeDevice attempts to identify an unidentified device
func probeDevice(path string) {
	showToast("Probing device...", "info")
	api.ProbeDevice(path, func(success bool, deviceID, chipType string, err error) {
		if err != nil {
			showError("Probe failed: " + err.Error())
			return
		}
		if !success {
			showError("Probe failed - device may not be an ESP32 or is in use")
			return
		}
		showSuccess("Device identified: " + chipType)
		loadDevices() // Refresh the list
	})
}

// forgetDevice removes an unidentified device from the list
func forgetDevice(path string) {
	api.ForgetDevice(path, func(success bool, err error) {
		if err != nil {
			showError("Failed to forget device: " + err.Error())
			return
		}
		if !success {
			showError("Failed to forget device")
			return
		}
		showSuccess("Device removed from list")
		loadDevices() // Refresh the list
	})
}
