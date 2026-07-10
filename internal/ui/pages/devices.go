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
		table.SetStyle("grid-template-columns", "1fr 1fr 1fr 120px 100px 100px")
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

		headerAliases := doc.CreateElement("div")
		headerAliases.SetTextContent("Aliases")
		table.Append(headerAliases)

		headerTags := doc.CreateElement("div")
		headerTags.SetTextContent("Tags")
		table.Append(headerTags)

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
	row.SetStyle("grid-template-columns", "1fr 1fr 1fr 120px 100px 100px")
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

	// Device path - combine Path and RealPath in one cell
	pathDiv := doc.CreateElement("div")
	pathDiv.SetStyle("font-family", "monospace")
	pathDiv.SetStyle("font-size", "12px")

	// Show real path (e.g., /dev/ttyACM0) if available, otherwise show the path
	displayPath := dev.RealPath
	if displayPath == "" || displayPath == dev.Path {
		displayPath = dev.Path
	}

	pathDiv.SetTextContent(displayPath)

	// Show alternative path in smaller text if different
	if dev.RealPath != "" && dev.RealPath != dev.Path {
		altPathSpan := doc.CreateElement("div")
		altPathSpan.SetStyle("font-size", "10px")
		altPathSpan.SetStyle("color", "#888")
		altPathSpan.SetTextContent(dev.Path)
		pathDiv.Append(altPathSpan)
	}

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

	// Aliases
	aliasesDiv := doc.CreateElement("div")
	aliasesDiv.SetStyle("font-size", "12px")
	aliasesDiv.SetStyle("overflow", "hidden")
	aliasesDiv.SetStyle("text-overflow", "ellipsis")
	aliasesDiv.SetStyle("white-space", "nowrap")
	if len(dev.Aliases) > 0 {
		aliasesDiv.SetTextContent(joinStrings(dev.Aliases, ", "))
	} else {
		aliasesDiv.SetTextContent("-")
		aliasesDiv.SetStyle("color", "#666")
	}
	row.Append(aliasesDiv)

	// Tags
	tagsDiv := doc.CreateElement("div")
	tagsDiv.SetStyle("font-size", "12px")
	tagsDiv.SetStyle("overflow", "hidden")
	tagsDiv.SetStyle("text-overflow", "ellipsis")
	tagsDiv.SetStyle("white-space", "nowrap")
	if len(dev.Tags) > 0 {
		tagsDiv.SetTextContent(joinStrings(dev.Tags, ", "))
	} else {
		tagsDiv.SetTextContent("-")
		tagsDiv.SetStyle("color", "#666")
	}
	row.Append(tagsDiv)

	// Actions
	actionsDiv := doc.CreateElement("div")
	actionsDiv.SetStyle("display", "flex")
	actionsDiv.SetStyle("gap", "6px")

	// Handle different device states
	if dev.DeviceID == "" {
		// Device without ID - show Edit, Probe and Forget buttons
		// Edit allows manual entry when probe fails
		editBtn := components.NewButton(components.ButtonConfig{
			Text:    "Edit",
			Class:   "btn-sm btn-secondary",
			OnClick: func(_ *dom.Event) { editUnprobedDevice(dev) },
		})
		actionsDiv.Append(editBtn.Element)

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
	} else {
		// Device has ID - check if it's a fallback (unprobed) device
		isFallbackDevice := strings.HasPrefix(dev.DeviceID, "unprobed-")

		// Show warning for fallback devices or access errors
		if isFallbackDevice || dev.AccessError != "" {
			warningBtn := components.NewButton(components.ButtonConfig{
				Text:  "⚠",
				Class: "btn-sm btn-warning",
				OnClick: func(_ *dom.Event) {
					if isFallbackDevice {
						js.Global().Get("window").Call("alert", "Device probe failed. Set Chip Type in Edit to enable device.")
					} else {
						js.Global().Get("window").Call("alert", dev.AccessError)
					}
				},
			})
			actionsDiv.Append(warningBtn.Element)
		}

		// Edit button - always show for devices with ID (including fallback)
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
	displayPath := dev.Path
	if dev.RealPath != "" && dev.RealPath != dev.Path {
		displayPath = dev.RealPath + " (" + dev.Path + ")"
	}
	pathRow := createFormField("Path", displayPath, true)
	content.Append(pathRow)

	// Device identification section (for physical devices)
	if dev.Backend == "" || (!strings.HasPrefix(dev.Path, "wokwi:") && !strings.HasPrefix(dev.Path, "qemu:") && !strings.HasPrefix(dev.Path, ":virtual:")) {
		identSection := doc.CreateElement("div")
		identSection.SetStyle("margin-top", "8px")
		identSection.SetStyle("padding-top", "8px")
		identSection.SetStyle("border-top", "1px solid rgba(255,255,255,0.1)")

		identHeader := doc.CreateElement("div")
		identHeader.SetStyle("font-weight", "500")
		identHeader.SetStyle("font-size", "13px")
		identHeader.SetStyle("margin-bottom", "8px")
		identHeader.SetStyle("color", "#6c5ce7")
		identHeader.SetTextContent("Device Identification")
		identSection.Append(identHeader)

		// Serial Number
		if dev.SerialNumber != "" {
			serialRow := createFormField("Serial Number", dev.SerialNumber, true)
			identSection.Append(serialRow)
		}

		// VID/PID
		vidPid := ""
		if dev.VID != "" || dev.PID != "" {
			vidPid = dev.VID + ":" + dev.PID
		}
		if vidPid != "" {
			vidPidRow := createFormField("VID:PID", vidPid, true)
			identSection.Append(vidPidRow)
		}

		// Manufacturer
		if dev.Manufacturer != "" {
			mfgRow := createFormField("Manufacturer", dev.Manufacturer, true)
			identSection.Append(mfgRow)
		}

		// Product
		if dev.Product != "" {
			productRow := createFormField("Product", dev.Product, true)
			identSection.Append(productRow)
		}

		// Reset Device button
		resetBtn := components.NewButton(components.ButtonConfig{
			Text:    "Reset Device",
			Class:   "btn-secondary",
			OnClick: func(_ *dom.Event) { resetDevice(dev) },
		})
		resetBtn.Element.SetStyle("margin-top", "8px")
		identSection.Append(resetBtn.Element)

		content.Append(identSection)
	}

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

	// Tags
	tagsStr := ""
	if len(dev.Tags) > 0 {
		tagsStr = joinStrings(dev.Tags, ", ")
	}
	tagsRow := createFormFieldWithID("Tags", "device-tags-input", tagsStr, false)
	content.Append(tagsRow)

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

	// Use path for unprobed devices instead of fallback deviceID
	identifier := dev.DeviceID
	displayID := dev.DeviceID
	if strings.HasPrefix(dev.DeviceID, "unprobed-") {
		identifier = dev.Path
		displayID = dev.Path
	}

	// Add delete button for all devices
	deleteBtn := components.NewButton(components.ButtonConfig{
		Text:  "Delete",
		Class: "btn-danger",
		OnClick: func(_ *dom.Event) {
			// Confirm deletion using JavaScript confirm
			result := js.Global().Get("window").Call("confirm", "Are you sure you want to delete device "+displayID+"?")
			if result.Bool() {
				api.DeleteDevice(identifier, func(success bool, err error) {
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
			saveDeviceAttributes(identifier)
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

// editUnprobedDevice opens modal for manually editing unprobed device info
func editUnprobedDevice(dev api.Device) {
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

	// Warning banner
	warning := doc.CreateElement("div")
	warning.SetStyle("background-color", "rgba(255, 165, 2, 0.1)")
	warning.SetStyle("border", "1px solid rgba(255, 165, 2, 0.3)")
	warning.SetStyle("border-radius", "6px")
	warning.SetStyle("padding", "12px")
	warning.SetStyle("font-size", "12px")
	warning.SetStyle("color", "#ffa502")
	warning.SetTextContent("Device not probed. Enter details manually or use Probe button to auto-detect.")
	content.Append(warning)

	// Device header
	header := doc.CreateElement("div")
	header.SetStyle("font-weight", "500")
	header.SetTextContent("Edit Device: " + dev.Path)
	content.Append(header)

	// Path (read-only)
	pathRow := createFormField("Device Path", dev.Path, true)
	content.Append(pathRow)

	// Real path if available
	if dev.RealPath != "" && dev.RealPath != dev.Path {
		realPathRow := createFormField("Real Path", dev.RealPath, true)
		content.Append(realPathRow)
	}

	// Device Identification section
	identSection := doc.CreateElement("div")
	identSection.SetStyle("margin-top", "8px")
	identSection.SetStyle("padding-top", "8px")
	identSection.SetStyle("border-top", "1px solid rgba(255,255,255,0.1)")

	identHeader := doc.CreateElement("div")
	identHeader.SetStyle("font-weight", "500")
	identHeader.SetStyle("font-size", "13px")
	identHeader.SetStyle("margin-bottom", "8px")
	identHeader.SetStyle("color", "#6c5ce7")
	identHeader.SetTextContent("Device Identification")
	identSection.Append(identHeader)

	// Serial Number (if available)
	if dev.SerialNumber != "" {
		serialRow := createFormField("Serial Number", dev.SerialNumber, true)
		identSection.Append(serialRow)
	}

	// VID:PID (if available)
	vidPid := ""
	if dev.VID != "" || dev.PID != "" {
		vidPid = dev.VID + ":" + dev.PID
	}
	if vidPid != "" {
		vidPidRow := createFormField("VID:PID", vidPid, true)
		identSection.Append(vidPidRow)
	}

	content.Append(identSection)

	// MAC Address (required for DeviceID generation)
	macRow := createFormFieldWithID("MAC Address", "device-mac-input", dev.MACAddress, false)
	macHint := doc.CreateElement("div")
	macHint.SetStyle("font-size", "11px")
	macHint.SetStyle("color", "#888")
	macHint.SetStyle("margin-top", "2px")
	macHint.SetTextContent("Format: AA:BB:CC:DD:EE:FF (Optional but recommended)")
	macRow.Append(macHint)
	content.Append(macRow)

	// Chip Type (editable selector)
	chipRow := createChipTypeSelector(dev.ChipType)
	content.Append(chipRow)

	// Chip Revision
	chipRevRow := createFormFieldWithID("Chip Revision", "device-chiprev-input", "", false)
	content.Append(chipRevRow)

	// Flash Size
	flashRow := createFormFieldWithID("Flash Size (bytes)", "device-flash-input", "", false)
	flashHint := doc.CreateElement("div")
	flashHint.SetStyle("font-size", "11px")
	flashHint.SetStyle("color", "#888")
	flashHint.SetStyle("margin-top", "2px")
	flashHint.SetTextContent("e.g., 4194304 (4MB)")
	flashRow.Append(flashHint)
	content.Append(flashRow)

	// Board Model
	boardRow := createFormFieldWithID("Board Model", "device-board-input", "", false)
	content.Append(boardRow)

	// Description
	descRow := createFormFieldWithID("Description", "device-desc-input", "", false)
	content.Append(descRow)

	// Aliases
	aliasesStr := ""
	if len(dev.Aliases) > 0 {
		aliasesStr = joinStrings(dev.Aliases, ", ")
	}
	aliasesRow := createFormFieldWithID("Aliases", "device-aliases-input", aliasesStr, false)
	content.Append(aliasesRow)

	// Tags
	tagsStr := ""
	if len(dev.Tags) > 0 {
		tagsStr = joinStrings(dev.Tags, ", ")
	}
	tagsRow := createFormFieldWithID("Tags", "device-tags-input", tagsStr, false)
	content.Append(tagsRow)

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

	probeBtn := components.NewButton(components.ButtonConfig{
		Text:  "Probe",
		Class: "btn-primary",
		OnClick: func(_ *dom.Event) {
			modal.Close()
			probeDevice(dev.Path)
		},
	})
	actions.Append(probeBtn.Element)

	saveBtn := components.NewButton(components.ButtonConfig{
		Text:  "Save",
		Class: "btn-primary",
		OnClick: func(_ *dom.Event) {
			saveUnprobedDevice(dev)
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

// saveUnprobedDevice saves manually entered device info via POST /api/v1/devices
func saveUnprobedDevice(dev api.Device) {
	doc := dom.GlobalDocument()

	macInput := doc.QuerySelector("#device-mac-input")
	chipTypeSelect := doc.QuerySelector("#device-chip-type")
	chipRevInput := doc.QuerySelector("#device-chiprev-input")
	flashInput := doc.QuerySelector("#device-flash-input")
	boardInput := doc.QuerySelector("#device-board-input")
	descInput := doc.QuerySelector("#device-desc-input")
	aliasesInput := doc.QuerySelector("#device-aliases-input")
	tagsInput := doc.QuerySelector("#device-tags-input")

	if macInput == nil || chipTypeSelect == nil {
		showError("Failed to read form values")
		return
	}

	mac := macInput.GetValue()
	chipType := chipTypeSelect.GetValue()
	chipRev := ""
	if chipRevInput != nil {
		chipRev = chipRevInput.GetValue()
	}
	flashSize := ""
	if flashInput != nil {
		flashSize = flashInput.GetValue()
	}
	boardModel := ""
	if boardInput != nil {
		boardModel = boardInput.GetValue()
	}
	description := ""
	if descInput != nil {
		description = descInput.GetValue()
	}

	// Get aliases
	aliasesStr := ""
	aliases := []string{}
	if aliasesInput != nil {
		aliasesStr = aliasesInput.GetValue()
		if aliasesStr != "" {
			aliases = splitString(aliasesStr, ",")
		}
	}

	// Get tags
	tagsStr := ""
	tags := []string{}
	if tagsInput != nil {
		tagsStr = tagsInput.GetValue()
		if tagsStr != "" {
			tags = splitString(tagsStr, ",")
		}
	}

	// Handle custom chip type
	if chipType == "Custom" {
		customInput := doc.QuerySelector("#device-chip-type-custom")
		if customInput != nil {
			customValue := customInput.GetValue()
			if customValue != "" {
				chipType = customValue
			}
		}
	}

	// Require chip type
	if chipType == "" {
		showError("Chip Type is required")
		return
	}

	// Validate MAC format if provided
	if mac != "" && !isValidMAC(mac) {
		showError("Invalid MAC address format. Use AA:BB:CC:DD:EE:FF")
		return
	}

	// Create device record via API
	req := map[string]interface{}{
		"path":        dev.Path,
		"mac_address": mac,
		"chip_type":   chipType,
		"aliases":     aliases,
		"tags":        tags,
	}

	if chipRev != "" {
		req["chip_rev"] = chipRev
	}
	if flashSize != "" {
		if flashSizeInt := parseFlashSize(flashSize); flashSizeInt > 0 {
			req["flash_size"] = flashSizeInt
		}
	}
	if boardModel != "" {
		req["board_model"] = boardModel
	}
	if description != "" {
		req["description"] = description
	}

	// Use UpdateDevice (PATCH) instead of CreateDevice (POST)
	// The device already exists in memory, we're just adding its identity
	api.UpdateDevice(dev.Path, req, func(success bool, err error) {
		if err != nil || !success {
			showError("Failed to save device: " + err.Error())
		} else {
			showSuccess("Device saved successfully")
			loadDevices() // Refresh the list
		}
	})
}

// isValidMAC validates MAC address format
func isValidMAC(mac string) bool {
	parts := splitString(mac, ":")
	if len(parts) != 6 {
		return false
	}
	for _, part := range parts {
		if len(part) != 2 {
			return false
		}
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// parseFlashSize parses flash size string to integer
func parseFlashSize(s string) int {
	// Try to parse as integer directly
	size := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			size = size*10 + int(c-'0')
		} else {
			break
		}
	}
	return size
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
	tagsInput := doc.QuerySelector("#device-tags-input")
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

	tagsStr := ""
	tags := []string{}
	if tagsInput != nil {
		tagsStr = tagsInput.GetValue()
		if tagsStr != "" {
			tags = splitString(tagsStr, ",")
		}
	}

	protected := protectedToggle.GetChecked()

	// Update request for basic device attributes
	req := map[string]interface{}{
		"aliases":   aliases,
		"tags":      tags,
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

// resetDevice sends a reset command to the device
func resetDevice(dev api.Device) {
	// For physical devices, trigger reset via DTR/RTS
	if dev.Backend == "" || (!strings.HasPrefix(dev.Path, "wokwi:") && !strings.HasPrefix(dev.Path, "qemu:") && !strings.HasPrefix(dev.Path, ":virtual:")) {
		// Call reset API
		api.ResetDevice(dev.Path, func(success bool, err error) {
			if err != nil || !success {
				showError("Failed to reset device: " + err.Error())
			} else {
				showSuccess("Device reset initiated - watch for reconnect")
				// Refresh device list after a short delay
				js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					loadDevices()
					return nil
				}), 2000)
			}
		})
		return
	}

	showError("Reset not available for virtual devices")
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
