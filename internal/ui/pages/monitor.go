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

// Monitor renders the serial monitor page
func Monitor(app *layout.App) {
	app.SetTitle("Serial Monitor")
	app.SetMainContentFunc(renderMonitorContent)
}

var (
	monitorWS     *api.MonitorWebSocket
	monitorOutput *dom.Element
	currentInput  string
	inputStartPos int
)

func renderMonitorContent() *dom.Element {
	doc := dom.GlobalDocument()
	container := doc.CreateElement("div")
	container.SetClass("page")

	header := doc.CreateElement("div")
	header.SetClass("page-header")
	header.SetTextContent("Serial Monitor")
	container.Append(header)

	// Controls card
	controlsCard := createMonitorControls()
	container.Append(controlsCard)

	// Output card
	outputCard := createMonitorOutput()
	container.Append(outputCard)

	return container
}

func createMonitorControls() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Connection",
	})

	content := doc.CreateElement("div")
	content.SetStyle("display", "flex")
	content.SetStyle("flex-direction", "column")
	content.SetStyle("gap", "12px")

	// Port selection
	portRow := doc.CreateElement("div")
	portRow.SetStyle("display", "flex")
	portRow.SetStyle("gap", "8px")
	portRow.SetStyle("align-items", "center")

	portLabel := doc.CreateElement("label")
	portLabel.SetTextContent("Port:")
	portLabel.SetStyle("min-width", "40px")
	portRow.Append(portLabel)

	portSelect := doc.CreateElement("select")
	portSelect.SetID("monitor-port-select")
	portSelect.SetStyle("flex", "1")
	portSelect.SetStyle("padding", "8px")
	portSelect.SetStyle("border-radius", "4px")
	portSelect.SetStyle("background-color", "#161634")
	portSelect.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	portSelect.SetStyle("color", "#eee")
	portRow.Append(portSelect)

	content.Append(portRow)

	// Baud rate selection
	baudRow := doc.CreateElement("div")
	baudRow.SetStyle("display", "flex")
	baudRow.SetStyle("gap", "8px")
	baudRow.SetStyle("align-items", "center")

	baudLabel := doc.CreateElement("label")
	baudLabel.SetTextContent("Baud:")
	baudLabel.SetStyle("min-width", "40px")
	baudRow.Append(baudLabel)

	baudSelect := doc.CreateElement("select")
	baudSelect.SetID("monitor-baud-select")
	baudSelect.SetStyle("flex", "1")
	baudSelect.SetStyle("padding", "8px")
	baudSelect.SetStyle("border-radius", "4px")
	baudSelect.SetStyle("background-color", "#161634")
	baudSelect.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	baudSelect.SetStyle("color", "#eee")

	baudRates := []string{"9600", "19200", "38400", "57600", "115200", "230400", "460800", "921600"}
	for _, rate := range baudRates {
		option := doc.CreateElement("option")
		option.SetTextContent(rate)
		option.SetAttribute("value", rate)
		if rate == "115200" {
			option.SetAttribute("selected", "selected")
		}
		baudSelect.Append(option)
	}

	baudRow.Append(baudSelect)
	content.Append(baudRow)

	// Reset options row
	resetRow := doc.CreateElement("div")
	resetRow.SetStyle("display", "flex")
	resetRow.SetStyle("gap", "16px")
	resetRow.SetStyle("align-items", "center")

	// Reset on connect checkbox
	resetOnConnectLabel := doc.CreateElement("label")
	resetOnConnectLabel.SetStyle("display", "flex")
	resetOnConnectLabel.SetStyle("align-items", "center")
	resetOnConnectLabel.SetStyle("gap", "8px")
	resetOnConnectLabel.SetStyle("cursor", "pointer")
	resetOnConnectLabel.SetStyle("user-select", "none")

	resetOnConnectCheckbox := doc.CreateElement("input")
	resetOnConnectCheckbox.SetAttribute("type", "checkbox")
	resetOnConnectCheckbox.SetID("monitor-reset-on-connect")
	resetOnConnectLabel.Append(resetOnConnectCheckbox)

	resetOnConnectText := doc.CreateElement("span")
	resetOnConnectText.SetTextContent("Reset on connect")
	resetOnConnectText.SetStyle("font-size", "13px")
	resetOnConnectText.SetStyle("color", "#bbb")
	resetOnConnectLabel.Append(resetOnConnectText)

	resetRow.Append(resetOnConnectLabel)

	// Reset button
	resetBtn := components.NewButton(components.ButtonConfig{
		Text:  "Reset Device",
		Class: "btn-secondary",
		OnClick: func(_ *dom.Event) {
			resetDevice()
		},
	})
	resetBtn.SetID("monitor-reset-btn")
	resetRow.Append(resetBtn.Element)

	content.Append(resetRow)

	// Buttons
	buttonRow := doc.CreateElement("div")
	buttonRow.SetStyle("display", "flex")
	buttonRow.SetStyle("gap", "8px")

	connectBtn := components.NewButton(components.ButtonConfig{
		Text:  "Connect",
		Class: "btn-primary",
		OnClick: func(_ *dom.Event) {
			connectMonitor()
		},
	})
	buttonRow.Append(connectBtn.Element)

	disconnectBtn := components.NewButton(components.ButtonConfig{
		Text:  "Disconnect",
		Class: "btn-secondary",
		OnClick: func(_ *dom.Event) {
			disconnectMonitor()
		},
	})
	disconnectBtn.Element.SetStyle("display", "none") // Initially hidden
	disconnectBtn.SetID("monitor-disconnect-btn")
	buttonRow.Append(disconnectBtn.Element)

	clearBtn := components.NewButton(components.ButtonConfig{
		Text:  "Clear",
		Class: "btn-secondary",
		OnClick: func(_ *dom.Event) {
			clearMonitorOutput()
		},
	})
	buttonRow.Append(clearBtn.Element)

	content.Append(buttonRow)

	card.SetContent(content)
	return card.Element
}

func createMonitorOutput() *dom.Element {
	doc := dom.GlobalDocument()
	card := components.NewCard(components.CardConfig{
		Title: "Output",
	})

	content := doc.CreateElement("div")
	content.SetStyle("position", "relative")
	content.SetStyle("height", "400px")

	monitorOutput = doc.CreateElement("div")
	monitorOutput.SetID("monitor-output")
	monitorOutput.SetStyle("height", "100%")
	monitorOutput.SetStyle("overflow-y", "auto")
	monitorOutput.SetStyle("background-color", "#0a0a0a")
	monitorOutput.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	monitorOutput.SetStyle("border-radius", "4px")
	monitorOutput.SetStyle("padding", "12px")
	monitorOutput.SetStyle("font-family", "monospace")
	monitorOutput.SetStyle("font-size", "13px")
	monitorOutput.SetStyle("line-height", "1.4")
	monitorOutput.SetStyle("color", "#0f0")
	monitorOutput.SetStyle("white-space", "pre-wrap")
	monitorOutput.SetStyle("word-break", "break-all")
	monitorOutput.SetStyle("outline", "none")
	monitorOutput.SetAttribute("tabindex", "0")
	monitorOutput.SetAttribute("contenteditable", "true")

	// Add keyboard event handler for terminal-style input
	monitorOutput.AddEventListener("keydown", func(evt *dom.Event) {
		if monitorWS == nil || !monitorWS.IsConnected() {
			return
		}

		event := evt.Value()
		key := event.Get("key").String()

		// Handle Enter key - send current input line
		if key == "Enter" {
			// Prevent default to avoid adding extra newline
			// Send the current input with newline
			textContent := monitorOutput.GetTextContent()
			// Extract the last line (user input)
			lastNewline := -1
			for i := len(textContent) - 1; i >= 0; i-- {
				if textContent[i] == '\n' {
					lastNewline = i
					break
				}
			}
			var inputLine string
			if lastNewline >= 0 {
				inputLine = textContent[lastNewline+1:]
			} else {
				inputLine = textContent
			}

			if inputLine != "" {
				err := monitorWS.Send(inputLine + "\r\n")
				if err != nil {
					showMonitorError("Send failed: " + err.Error())
				}
			} else {
				// Just send newline if input is empty
				err := monitorWS.Send("\r\n")
				if err != nil {
					showMonitorError("Send failed: " + err.Error())
				}
			}
			currentInput = ""
		}
	})

	placeholder := doc.CreateElement("div")
	placeholder.SetStyle("color", "#666")
	placeholder.SetStyle("text-align", "center")
	placeholder.SetStyle("padding-top", "180px")
	placeholder.SetTextContent("Not connected. Select a port and click Connect.")
	placeholder.SetID("monitor-placeholder")
	monitorOutput.Append(placeholder)

	content.Append(monitorOutput)
	card.SetContent(content)
	return card.Element
}

func connectMonitor() {
	doc := dom.GlobalDocument()

	// Get selected port
	portSelect := doc.GetElementByID("monitor-port-select")
	if portSelect == nil {
		showMonitorError("No port selected")
		return
	}
	port := portSelect.GetValue()
	if port == "" {
		showMonitorError("No port selected")
		return
	}

	// Get selected baud rate
	baudSelect := doc.GetElementByID("monitor-baud-select")
	if baudSelect == nil {
		showMonitorError("No baud rate selected")
		return
	}
	baudRate := parseInt(baudSelect.GetValue())

	// Get reset on connect option
	resetOnConnectCheckbox := doc.GetElementByID("monitor-reset-on-connect")
	resetOnConnect := false
	if resetOnConnectCheckbox != nil {
		resetOnConnect = resetOnConnectCheckbox.Value().Get("checked").Bool()
	}

	// Remove /dev/ prefix if present (backend expects bare port name)
	portName := port
	if len(port) > 5 && port[:5] == "/dev/" {
		portName = port[5:]
	}

	// Close existing connection
	if monitorWS != nil {
		monitorWS.Close()
	}

	// Create WebSocket connection
	config := &api.MonitorConfig{
		Port:     portName,
		BaudRate: baudRate,
		Reset:    resetOnConnect,
	}
	monitorWS = api.NewMonitorWebSocket(config)

	// Set up handlers
	monitorWS.SetMessageHandler(func(data string) {
		appendMonitorOutput(data)
	})

	monitorWS.SetErrorHandler(func(err error) {
		showMonitorError("Connection error: " + err.Error())
	})

	monitorWS.SetCloseHandler(func() {
		showMonitorDisconnected()
	})

	// Update UI
	showMonitorConnected()
	showMonitorInfo("Connecting to " + port + " at " + intToString(baudRate) + " baud...")
}

func disconnectMonitor() {
	if monitorWS != nil {
		monitorWS.Close()
		monitorWS = nil
	}
	showMonitorDisconnected()
}

func clearMonitorOutput() {
	if monitorOutput == nil {
		return
	}

	doc := dom.GlobalDocument()
	placeholder := doc.CreateElement("div")
	placeholder.SetStyle("color", "#666")
	placeholder.SetStyle("text-align", "center")
	placeholder.SetStyle("padding-top", "180px")
	placeholder.SetTextContent("Output cleared")
	placeholder.SetID("monitor-placeholder")

	monitorOutput.RemoveChildren()
	monitorOutput.Append(placeholder)
}

func appendMonitorOutput(data string) {
	if monitorOutput == nil {
		return
	}

	// Remove placeholder if present
	placeholder := dom.GlobalDocument().QuerySelector("#monitor-placeholder")
	if placeholder != nil {
		placeholder.Remove()
	}

	// Append data
	dataElem := dom.GlobalDocument().CreateElement("span")
	dataElem.SetTextContent(data)
	monitorOutput.Append(dataElem)

	// Auto-scroll to bottom
	monitorOutput.Value().Set("scrollTop", monitorOutput.Value().Get("scrollHeight"))
}

func showMonitorConnected() {
	doc := dom.GlobalDocument()

	// Hide connect button, show disconnect button
	connectBtn := findButtonByText("Connect")
	if connectBtn != nil {
		connectBtn.SetStyle("display", "none")
	}

	disconnectBtn := doc.GetElementByID("monitor-disconnect-btn")
	if disconnectBtn != nil {
		disconnectBtn.SetStyle("display", "inline-block")
	}

	// Disable port and baud selects
	portSelect := doc.GetElementByID("monitor-port-select")
	if portSelect != nil {
		portSelect.SetAttribute("disabled", "disabled")
	}

	baudSelect := doc.GetElementByID("monitor-baud-select")
	if baudSelect != nil {
		baudSelect.SetAttribute("disabled", "disabled")
	}

	// Focus on terminal output for typing
	if monitorOutput != nil {
		monitorOutput.Value().Call("focus")
	}

	// Disable reset on connect checkbox
	resetOnConnectCheckbox := doc.GetElementByID("monitor-reset-on-connect")
	if resetOnConnectCheckbox != nil {
		resetOnConnectCheckbox.SetAttribute("disabled", "disabled")
	}
}

func showMonitorDisconnected() {
	doc := dom.GlobalDocument()

	// Show connect button, hide disconnect button
	connectBtn := findButtonByText("Connect")
	if connectBtn != nil {
		connectBtn.SetStyle("display", "inline-block")
	}

	disconnectBtn := doc.GetElementByID("monitor-disconnect-btn")
	if disconnectBtn != nil {
		disconnectBtn.SetStyle("display", "none")
	}

	// Enable port and baud selects
	portSelect := doc.GetElementByID("monitor-port-select")
	if portSelect != nil {
		portSelect.RemoveAttribute("disabled")
	}

	baudSelect := doc.GetElementByID("monitor-baud-select")
	if baudSelect != nil {
		baudSelect.RemoveAttribute("disabled")
	}

	// Re-enable reset on connect checkbox
	resetOnConnectCheckbox := doc.GetElementByID("monitor-reset-on-connect")
	if resetOnConnectCheckbox != nil {
		resetOnConnectCheckbox.RemoveAttribute("disabled")
	}
}

func showMonitorError(message string) {
	showMonitorToast(message, "error")
}

func showMonitorInfo(message string) {
	showMonitorToast(message, "info")
}

func showMonitorToast(message, toastType string) {
	doc := dom.GlobalDocument()

	existing := doc.GetElementByID("monitor-toast")
	if existing != nil {
		existing.Remove()
	}

	toast := doc.CreateElement("div")
	toast.SetID("monitor-toast")
	toast.SetTextContent(message)

	if toastType == "error" {
		toast.SetStyle("background-color", "rgba(255, 71, 87, 0.9)")
	} else {
		toast.SetStyle("background-color", "rgba(59, 130, 246, 0.9)")
	}

	toast.SetStyle("position", "fixed")
	toast.SetStyle("bottom", "20px")
	toast.SetStyle("right", "20px")
	toast.SetStyle("padding", "12px 16px")
	toast.SetStyle("border-radius", "6px")
	toast.SetStyle("color", "#fff")
	toast.SetStyle("z-index", "1000")
	toast.SetStyle("box-shadow", "0 4px 12px rgba(0,0,0,0.3)")

	doc.GetBody().Append(toast)

	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		toast.Remove()
		return nil
	}), 3000)
}

func findButtonByText(text string) *dom.Element {
	doc := dom.GlobalDocument()
	buttons := doc.QuerySelectorAll("button")
	if buttons == nil {
		return nil
	}
	for _, btn := range buttons {
		if btn.GetTextContent() == text {
			return btn
		}
	}
	return nil
}

func parseInt(s string) int {
	result := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + intToString(-n)
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func resetDevice() {
	if monitorWS == nil || !monitorWS.IsConnected() {
		showMonitorError("Not connected")
		return
	}

	// Send reset control message via WebSocket
	// The backend handles this by toggling DTR/RTS
	showMonitorInfo("Resetting device...")
	// Note: Reset functionality would need backend support via WebSocket messages
	// For now, this is a placeholder - the reset on connect works via URL parameter
}

func initMonitorPage() {
	// Load available serial ports
	loadSerialPorts()
}

func loadSerialPorts() {
	// For now, add common serial ports
	// In the future, this could come from an API endpoint
	doc := dom.GlobalDocument()
	portSelect := doc.GetElementByID("monitor-port-select")
	if portSelect == nil {
		return
	}

	commonPorts := []string{
		"/dev/ttyUSB0", "/dev/ttyUSB1", "/dev/ttyUSB2",
		"/dev/ttyACM0", "/dev/ttyACM1", "/dev/ttyACM2",
		"/dev/ttyAMA0", "/dev/ttyS0", "/dev/ttyS1",
	}

	portSelect.RemoveChildren()

	placeholder := doc.CreateElement("option")
	placeholder.SetTextContent("-- Select Port --")
	placeholder.SetAttribute("value", "")
	portSelect.Append(placeholder)

	for _, port := range commonPorts {
		option := doc.CreateElement("option")
		option.SetTextContent(port)
		option.SetAttribute("value", port)
		portSelect.Append(option)
	}
}
