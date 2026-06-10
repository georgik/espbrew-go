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

	// Device path
	pathDiv := doc.CreateElement("div")
	pathDiv.SetStyle("font-family", "monospace")
	pathDiv.SetTextContent(dev.Path)
	row.Append(pathDiv)

	// Chip type
	chipDiv := doc.CreateElement("div")
	chipDiv.SetTextContent(dev.ChipType)
	row.Append(chipDiv)

	// Status badge
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

	// Edit button
	editBtn := components.NewButton(components.ButtonConfig{
		Text:    "Edit",
		Class:   "btn-sm",
		OnClick: func(_ *dom.Event) { editDevice(dev) },
	})
	actionsDiv.Append(editBtn.Element)

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

	// Chip Type
	chipRow := createFormField("Chip Type", dev.ChipType, true)
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

func saveDeviceAttributes(deviceID string) {
	doc := dom.GlobalDocument()
	aliasesInput := doc.QuerySelector("#device-aliases-input")
	protectedToggle := doc.QuerySelector("#device-protected")

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

	// Update request
	req := map[string]interface{}{
		"aliases":   aliases,
		"protected": protected,
	}

	// Call update API
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
