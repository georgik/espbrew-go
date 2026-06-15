//go:build js
// +build js

package components

import (
	"syscall/js"

	"codeberg.org/georgik/espbrew-go/internal/ui/api"
	"codeberg.org/georgik/espbrew-go/internal/ui/dom"
)

// BoundingBoxEditor is a canvas-based editor for defining device regions
type BoundingBoxEditor struct {
	*dom.Element
	canvas          *dom.Element
	ctx             js.Value
	image           *dom.Element
	boxes           []BoundingBox
	selectedBox     *BoundingBox
	mode            string // "draw", "select", "edit"
	isDragging      bool
	isResizing      bool
	dragHandle      string
	dragStart       struct{ x, y float64 }
	lastPointer     struct{ x, y float64 }
	cameraID        string
	onBoxCreate     func(box *BoundingBox, boxID string)
	devices         []api.Device
	selectedDevice  string // Currently selected device for auto-assignment
	deviceSelector  *dom.Element
	pendingMappings []api.DeviceMappingWithDevice // Mappings to load after image loads
}

// BoundingBox represents a device region
type BoundingBox struct {
	ID       string
	DeviceID string
	X        float64
	Y        float64
	Width    float64
	Height   float64
}

// EditorConfig configures the bounding box editor
type EditorConfig struct {
	CameraID    string
	ImageSrc    string
	OnBoxCreate func(box *BoundingBox, boxID string)
	Devices     []api.Device // Available devices for selection
}

// NewBoundingBoxEditor creates a new bounding box editor
func NewBoundingBoxEditor(config EditorConfig) *BoundingBoxEditor {
	doc := dom.GlobalDocument()

	editor := &BoundingBoxEditor{
		Element:     doc.CreateElement("div"),
		mode:        "draw",
		cameraID:    config.CameraID,
		onBoxCreate: config.OnBoxCreate,
		devices:     config.Devices,
	}

	editor.SetClass("bbox-editor")
	editor.SetStyle("position", "relative")
	editor.SetStyle("display", "flex")
	editor.SetStyle("flex-direction", "column")
	editor.SetStyle("gap", "12px")

	// Create device selector at top
	editor.createDeviceSelector()

	// Create image element
	editor.image = doc.CreateElement("img")
	editor.image.SetID("editor-image")
	editor.image.SetStyle("max-width", "100%")
	editor.image.SetStyle("display", "block")
	editor.image.SetStyle("user-select", "none")
	editor.image.SetStyle("-webkit-user-select", "none")
	editor.image.AddEventListener("load", func(_ *dom.Event) {
		// Use setTimeout to ensure image is fully rendered
		js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			editor.resizeCanvas()
			editor.render()
			// Load pending mappings now that canvas has correct dimensions
			if len(editor.pendingMappings) > 0 {
				editor.loadBoxes(editor.pendingMappings)
				editor.pendingMappings = nil
			}
			return nil
		}), 50)
	})
	editor.image.SetAttribute("src", config.ImageSrc)

	// Create canvas wrapper for proper positioning
	canvasWrapper := doc.CreateElement("div")
	canvasWrapper.SetStyle("position", "relative")
	canvasWrapper.SetStyle("display", "inline-block")

	// Create image and canvas inside wrapper
	canvasWrapper.Append(editor.image)

	// Create canvas overlay - must be after image in DOM
	editor.canvas = doc.CreateElement("canvas")
	editor.canvas.SetID("editor-canvas")
	editor.canvas.SetStyle("position", "absolute")
	editor.canvas.SetStyle("top", "0")
	editor.canvas.SetStyle("left", "0")
	editor.canvas.SetStyle("pointer-events", "auto")
	editor.canvas.SetStyle("z-index", "10")
	editor.canvas.SetStyle("touch-action", "none") // Prevent default touch behaviors
	editor.canvas.SetStyle("cursor", "crosshair")  // Show crosshair cursor
	// Set initial size to avoid 0x0 before image loads
	editor.canvas.Value().Set("width", 100)
	editor.canvas.Value().Set("height", 100)
	canvasWrapper.Append(editor.canvas)

	// Append canvas wrapper to editor
	editor.Append(canvasWrapper)

	// Setup canvas context
	editor.ctx = editor.canvas.Value().Call("getContext", "2d")

	// Setup event handlers
	editor.setupEventHandlers()

	return editor
}

// createDeviceSelector creates the device dropdown at the top
func (e *BoundingBoxEditor) createDeviceSelector() {
	doc := dom.GlobalDocument()

	// Create toolbar container
	toolbar := doc.CreateElement("div")
	toolbar.SetStyle("display", "flex")
	toolbar.SetStyle("flex-wrap", "wrap")
	toolbar.SetStyle("gap", "12px")
	toolbar.SetStyle("align-items", "center")
	toolbar.SetStyle("padding", "12px")
	toolbar.SetStyle("background-color", "rgba(255,255,255,0.05)")
	toolbar.SetStyle("border-radius", "6px")
	toolbar.SetStyle("margin-bottom", "8px")

	// Instructions
	instructions := doc.CreateElement("div")
	instructions.SetStyle("font-size", "13px")
	instructions.SetStyle("color", "#aaa")
	instructions.SetTextContent("Draw regions to assign devices")
	toolbar.Append(instructions)

	// Device selector label
	label := doc.CreateElement("label")
	label.SetTextContent("Auto-assign to:")
	label.SetStyle("font-size", "13px")
	label.SetStyle("font-weight", "500")
	toolbar.Append(label)

	// Device dropdown
	e.deviceSelector = doc.CreateElement("select")
	e.deviceSelector.SetID("editor-device-selector")
	e.deviceSelector.SetStyle("padding", "6px 12px")
	e.deviceSelector.SetStyle("border-radius", "4px")
	e.deviceSelector.SetStyle("background-color", "#161634")
	e.deviceSelector.SetStyle("border", "1px solid rgba(255,255,255,0.1)")
	e.deviceSelector.SetStyle("color", "#eee")
	e.deviceSelector.SetStyle("font-size", "13px")
	e.deviceSelector.SetStyle("min-width", "200px")

	// Add "None" option
	noneOption := doc.CreateElement("option")
	noneOption.SetAttribute("value", "")
	noneOption.SetTextContent("-- None (ask each time) --")
	e.deviceSelector.Append(noneOption)

	// Add device options
	for _, dev := range e.devices {
		option := doc.CreateElement("option")
		option.SetAttribute("value", dev.DeviceID)

		displayName := dev.DeviceID
		if len(dev.Aliases) > 0 {
			displayName = dev.Aliases[0]
		}
		option.SetTextContent(displayName + " (" + dev.ChipType + ")")
		e.deviceSelector.Append(option)
	}

	// Add change listener
	e.deviceSelector.AddEventListener("change", func(_ *dom.Event) {
		e.selectedDevice = e.deviceSelector.GetValue()
		if e.selectedDevice != "" {
			println("Auto-assigning to device:", e.selectedDevice)
		}
	})

	toolbar.Append(e.deviceSelector)

	// Mode buttons
	modeLabel := doc.CreateElement("label")
	modeLabel.SetTextContent("Mode:")
	modeLabel.SetStyle("font-size", "13px")
	modeLabel.SetStyle("font-weight", "500")
	toolbar.Append(modeLabel)

	drawBtn := doc.CreateElement("button")
	drawBtn.SetTextContent("Draw")
	drawBtn.SetStyle("padding", "6px 12px")
	drawBtn.SetStyle("border-radius", "4px")
	drawBtn.SetStyle("background-color", "#6c5ce7")
	drawBtn.SetStyle("border", "none")
	drawBtn.SetStyle("color", "#fff")
	drawBtn.SetStyle("cursor", "pointer")
	drawBtn.SetStyle("font-size", "13px")
	toolbar.Append(drawBtn)

	selectBtn := doc.CreateElement("button")
	selectBtn.SetTextContent("Edit")
	selectBtn.SetStyle("padding", "6px 12px")
	selectBtn.SetStyle("border-radius", "4px")
	selectBtn.SetStyle("background-color", "transparent")
	selectBtn.SetStyle("border", "1px solid rgba(255,255,255,0.2)")
	selectBtn.SetStyle("color", "#aaa")
	selectBtn.SetStyle("cursor", "pointer")
	selectBtn.SetStyle("font-size", "13px")
	toolbar.Append(selectBtn)

	// Add event listeners after both buttons are created
	drawBtn.AddEventListener("click", func(_ *dom.Event) {
		e.SetMode("draw")
		drawBtn.SetStyle("background-color", "#6c5ce7")
		drawBtn.SetStyle("color", "#fff")
		selectBtn.SetStyle("background-color", "transparent")
		selectBtn.SetStyle("border-color", "rgba(255,255,255,0.2)")
		selectBtn.SetStyle("color", "#aaa")
	})

	selectBtn.AddEventListener("click", func(_ *dom.Event) {
		e.SetMode("edit")
		selectBtn.SetStyle("background-color", "#6c5ce7")
		selectBtn.SetStyle("border-color", "#6c5ce7")
		selectBtn.SetStyle("color", "#fff")
		drawBtn.SetStyle("background-color", "transparent")
		drawBtn.SetStyle("border", "1px solid rgba(255,255,255,0.2)")
		drawBtn.SetStyle("color", "#aaa")
	})

	// Clear all button
	clearBtn := doc.CreateElement("button")
	clearBtn.SetTextContent("Clear All")
	clearBtn.SetStyle("padding", "6px 12px")
	clearBtn.SetStyle("border-radius", "4px")
	clearBtn.SetStyle("background-color", "transparent")
	clearBtn.SetStyle("border", "1px solid rgba(255,100,100,0.3)")
	clearBtn.SetStyle("color", "#ff6b6b")
	clearBtn.SetStyle("cursor", "pointer")
	clearBtn.SetStyle("font-size", "13px")
	clearBtn.AddEventListener("click", func(_ *dom.Event) {
		e.boxes = nil
		e.selectedBox = nil
		e.render()
	})
	toolbar.Append(clearBtn)

	e.Append(toolbar)
}

// setupEventHandlers attaches mouse/touch event listeners
func (e *BoundingBoxEditor) setupEventHandlers() {
	e.canvas.AddEventListener("pointerdown", func(evt *dom.Event) {
		e.handlePointerDown(evt)
	})

	e.canvas.AddEventListener("pointermove", func(evt *dom.Event) {
		e.handlePointerMove(evt)
	})

	e.canvas.AddEventListener("pointerup", func(evt *dom.Event) {
		e.handlePointerUp(evt)
	})

	e.canvas.AddEventListener("pointerleave", func(evt *dom.Event) {
		e.handlePointerUp(evt)
	})
}

// resizeCanvas adjusts canvas size to match image
func (e *BoundingBoxEditor) resizeCanvas() {
	if e.image == nil || e.canvas == nil {
		return
	}

	imageWidth := e.image.Value().Get("clientWidth").Int()
	imageHeight := e.image.Value().Get("clientHeight").Int()

	// Ensure we have valid dimensions
	if imageWidth <= 0 || imageHeight <= 0 {
		imageWidth = 800
		imageHeight = 600
	}

	e.canvas.Value().Set("width", imageWidth)
	e.canvas.Value().Set("height", imageHeight)
	e.canvas.SetStyle("width", js.ValueOf(imageWidth).String()+"px")
	e.canvas.SetStyle("height", js.ValueOf(imageHeight).String()+"px")
}

// handlePointerDown handles mouse/touch down
func (e *BoundingBoxEditor) handlePointerDown(evt *dom.Event) {
	jsEvt := evt.Value()

	// Capture pointer for drag tracking
	pointerID := jsEvt.Get("pointerId").Int()
	e.canvas.Value().Call("setPointerCapture", pointerID)

	point := e.getCanvasPoint(jsEvt)

	e.lastPointer.x = point.x
	e.lastPointer.y = point.y
	e.isDragging = true

	switch e.mode {
	case "draw":
		e.startDrawing(point)
	case "select", "edit":
		e.handleSelectOrEdit(point)
	}

	e.render()
}

// handlePointerMove handles mouse/touch move
func (e *BoundingBoxEditor) handlePointerMove(evt *dom.Event) {
	jsEvt := evt.Value()
	point := e.getCanvasPoint(jsEvt)

	if !e.isDragging {
		e.updateCursor(point)
		return
	}

	deltaX := point.x - e.lastPointer.x
	deltaY := point.y - e.lastPointer.y

	switch e.mode {
	case "draw":
		if e.selectedBox != nil {
			e.updateDrawingBox(point)
		}
	case "edit":
		if e.isResizing && e.selectedBox != nil && e.dragHandle != "" {
			e.resizeBox(deltaX, deltaY, e.dragHandle)
		} else if e.selectedBox != nil {
			e.moveBox(deltaX, deltaY)
		}
	}

	e.lastPointer.x = point.x
	e.lastPointer.y = point.y
	e.render()
}

// handlePointerUp handles mouse/touch up
func (e *BoundingBoxEditor) handlePointerUp(evt *dom.Event) {
	jsEvt := evt.Value()

	// Release pointer capture
	pointerID := jsEvt.Get("pointerId").Int()
	e.canvas.Value().Call("releasePointerCapture", pointerID)

	e.isDragging = false
	e.isResizing = false
	e.dragHandle = ""

	// Finalize drawn box
	if e.mode == "draw" && e.selectedBox != nil {
		box := e.selectedBox
		if box.Width < 20 || box.Height < 20 {
			// Remove too-small boxes
			e.boxes = removeBox(e.boxes, box.ID)
			e.selectedBox = nil
		} else {
			// Check if we have a pre-selected device
			if e.selectedDevice != "" {
				// Auto-assign to selected device
				box.DeviceID = e.selectedDevice
				println("Auto-assigned box", box.ID, "to device", e.selectedDevice)

				// Save the mapping
				normalized := e.pixelsToNormalized(box)
				if e.onBoxCreate != nil {
					// Call with empty device ID since we already assigned it
					e.onBoxCreate(&normalized, box.ID)
				}

				// Trigger save immediately
				js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					// This will be handled by the callback
					return nil
				}), 0)
			} else {
				// No device selected, show device selector
				normalized := e.pixelsToNormalized(box)
				if e.onBoxCreate != nil {
					e.onBoxCreate(&normalized, box.ID)
				}
			}
			e.selectedBox = nil
		}
	}

	e.render()
}

// getCanvasPoint converts event to canvas coordinates
func (e *BoundingBoxEditor) getCanvasPoint(evt js.Value) struct{ x, y float64 } {
	rect := e.canvas.Value().Call("getBoundingClientRect")
	return struct{ x, y float64 }{
		x: evt.Get("clientX").Float() - rect.Get("left").Float(),
		y: evt.Get("clientY").Float() - rect.Get("top").Float(),
	}
}

// startDrawing begins a new box
func (e *BoundingBoxEditor) startDrawing(point struct{ x, y float64 }) {
	box := &BoundingBox{
		ID:     "box-" + js.Global().Get("Date").Call("now").String(),
		X:      point.x,
		Y:      point.y,
		Width:  0,
		Height: 0,
	}
	e.boxes = append(e.boxes, *box)
	e.selectedBox = box
}

// updateDrawingBox updates the box being drawn
func (e *BoundingBoxEditor) updateDrawingBox(point struct{ x, y float64 }) {
	if e.selectedBox == nil {
		return
	}

	startX := e.selectedBox.X
	startY := e.selectedBox.Y

	e.selectedBox.Width = absFloat(point.x - startX)
	e.selectedBox.Height = absFloat(point.y - startY)
	e.selectedBox.X = minFloat(point.x, startX)
	e.selectedBox.Y = minFloat(point.y, startY)

	e.clampBox(e.selectedBox)
}

// handleSelectOrEdit handles selection/editing
func (e *BoundingBoxEditor) handleSelectOrEdit(point struct{ x, y float64 }) {
	// Check for resize handle first
	if e.mode == "edit" && e.selectedBox != nil {
		handle := e.getHandleAtPoint(point)
		if handle != "" {
			e.isResizing = true
			e.dragHandle = handle
			return
		}
	}

	// Check for box click
	box := e.getBoxAtPoint(point)
	if box != nil {
		e.selectedBox = box
	} else {
		e.selectedBox = nil
	}
}

// moveBox moves the selected box
func (e *BoundingBoxEditor) moveBox(deltaX, deltaY float64) {
	if e.selectedBox == nil {
		return
	}

	e.selectedBox.X += deltaX
	e.selectedBox.Y += deltaY

	e.clampBox(e.selectedBox)
}

// resizeBox resizes the selected box by handle
func (e *BoundingBoxEditor) resizeBox(deltaX, deltaY float64, handle string) {
	if e.selectedBox == nil {
		return
	}

	box := e.selectedBox

	switch handle {
	case "nw":
		box.X += deltaX
		box.Y += deltaY
		box.Width -= deltaX
		box.Height -= deltaY
	case "ne":
		box.Y += deltaY
		box.Width += deltaX
		box.Height -= deltaY
	case "sw":
		box.X += deltaX
		box.Width -= deltaX
		box.Height += deltaY
	case "se":
		box.Width += deltaX
		box.Height += deltaY
	}

	// Handle negative size
	if box.Width < 0 {
		box.X += box.Width
		box.Width = absFloat(box.Width)
	}
	if box.Height < 0 {
		box.Y += box.Height
		box.Height = absFloat(box.Height)
	}

	e.clampBox(box)
}

// getBoxAtPoint finds a box at the given point
func (e *BoundingBoxEditor) getBoxAtPoint(point struct{ x, y float64 }) *BoundingBox {
	// Search in reverse (topmost first)
	for i := len(e.boxes) - 1; i >= 0; i-- {
		box := &e.boxes[i]
		if point.x >= box.X && point.x <= box.X+box.Width &&
			point.y >= box.Y && point.y <= box.Y+box.Height {
			return box
		}
	}
	return nil
}

// getHandleAtPoint finds a resize handle at point
func (e *BoundingBoxEditor) getHandleAtPoint(point struct{ x, y float64 }) string {
	if e.selectedBox == nil {
		return ""
	}

	box := e.selectedBox
	handles := []struct {
		name string
		x, y float64
	}{
		{"nw", box.X, box.Y},
		{"ne", box.X + box.Width, box.Y},
		{"sw", box.X, box.Y + box.Height},
		{"se", box.X + box.Width, box.Y + box.Height},
	}

	hitRadius := 12.0

	for _, handle := range handles {
		distance := sqrt(
			pow(point.x-handle.x, 2) +
				pow(point.y-handle.y, 2),
		)
		if distance <= hitRadius {
			return handle.name
		}
	}

	return ""
}

// updateCursor updates cursor based on hover
func (e *BoundingBoxEditor) updateCursor(point struct{ x, y float64 }) {
	if e.mode == "draw" {
		e.canvas.SetStyle("cursor", "crosshair")
		return
	}

	if e.mode == "edit" && e.selectedBox != nil {
		handle := e.getHandleAtPoint(point)
		if handle != "" {
			cursors := map[string]string{
				"nw": "nw-resize",
				"ne": "ne-resize",
				"sw": "sw-resize",
				"se": "se-resize",
			}
			if cursor, ok := cursors[handle]; ok {
				e.canvas.SetStyle("cursor", cursor)
				return
			}
		}
	}

	if e.getBoxAtPoint(point) != nil {
		if e.mode == "edit" {
			e.canvas.SetStyle("cursor", "move")
		} else {
			e.canvas.SetStyle("cursor", "pointer")
		}
	} else {
		e.canvas.SetStyle("cursor", "default")
	}
}

// clampBox constrains box to canvas bounds
func (e *BoundingBoxEditor) clampBox(box *BoundingBox) {
	canvasWidth := float64(e.canvas.Value().Get("width").Int())
	canvasHeight := float64(e.canvas.Value().Get("height").Int())

	box.X = clampFloat(box.X, 0, canvasWidth-box.Width)
	box.Y = clampFloat(box.Y, 0, canvasHeight-box.Height)
	box.Width = clampFloat(box.Width, 20, canvasWidth-box.X)
	box.Height = clampFloat(box.Height, 20, canvasHeight-box.Y)
}

// pixelsToNormalized converts pixel coords to normalized (0-1)
func (e *BoundingBoxEditor) pixelsToNormalized(box *BoundingBox) BoundingBox {
	canvasWidth := float64(e.canvas.Value().Get("width").Int())
	canvasHeight := float64(e.canvas.Value().Get("height").Int())

	return BoundingBox{
		ID:       box.ID,
		DeviceID: box.DeviceID,
		X:        box.X / canvasWidth,
		Y:        box.Y / canvasHeight,
		Width:    box.Width / canvasWidth,
		Height:   box.Height / canvasHeight,
	}
}

// normalizedToPixels converts normalized coords to pixels
func (e *BoundingBoxEditor) normalizedToPixels(bounds api.BoundingBox) BoundingBox {
	canvasWidth := float64(e.canvas.Value().Get("width").Int())
	canvasHeight := float64(e.canvas.Value().Get("height").Int())

	return BoundingBox{
		X:      bounds.X * canvasWidth,
		Y:      bounds.Y * canvasHeight,
		Width:  bounds.Width * canvasWidth,
		Height: bounds.Height * canvasHeight,
	}
}

// setMode sets the editor mode
func (e *BoundingBoxEditor) SetMode(mode string) {
	e.mode = mode
	e.selectedBox = nil
	e.render()
}

// loadBoxes loads existing boxes from API
func (e *BoundingBoxEditor) loadBoxes(boxes []api.DeviceMappingWithDevice) {
	e.boxes = nil
	for _, box := range boxes {
		pixelBox := e.normalizedToPixels(box.Bounds)
		pixelBox.ID = box.ID
		pixelBox.DeviceID = box.DeviceID
		e.boxes = append(e.boxes, pixelBox)
	}
	e.render()
}

// LoadMappings is exported alias for loadBoxes
func (e *BoundingBoxEditor) LoadMappings(boxes []api.DeviceMappingWithDevice) {
	// Check if canvas has valid dimensions (image loaded)
	canvasWidth := e.canvas.Value().Get("width").Int()
	if canvasWidth <= 100 {
		// Image not loaded yet, store as pending
		e.pendingMappings = boxes
	} else {
		// Canvas ready, load immediately
		e.loadBoxes(boxes)
	}
}

// AssignDevice assigns a device to a box by ID
func (e *BoundingBoxEditor) AssignDevice(boxID, deviceID string) {
	for i := range e.boxes {
		if e.boxes[i].ID == boxID {
			e.boxes[i].DeviceID = deviceID
			e.render()
			break
		}
	}
}

// render draws all boxes on canvas
func (e *BoundingBoxEditor) render() {
	if e.ctx.IsUndefined() || e.ctx.IsNull() {
		return
	}

	canvasWidth := e.canvas.Value().Get("width").Int()
	canvasHeight := e.canvas.Value().Get("height").Int()

	e.ctx.Call("clearRect", 0, 0, canvasWidth, canvasHeight)

	for _, box := range e.boxes {
		e.renderBox(box)
	}

	if e.selectedBox != nil && e.mode == "edit" {
		e.renderHandles(*e.selectedBox)
	}
}

// renderBox draws a single box
func (e *BoundingBoxEditor) renderBox(box BoundingBox) {
	isSelected := e.selectedBox != nil && e.selectedBox.ID == box.ID
	isMapped := box.DeviceID != ""

	var borderColor, fillColor string
	if isSelected {
		borderColor = "#f59e0b"
		fillColor = "rgba(245, 158, 11, 0.2)"
	} else if isMapped {
		borderColor = "#22c55e"
		fillColor = "rgba(34, 197, 94, 0.2)"
	} else {
		borderColor = "#3b82f6"
		fillColor = "rgba(59, 130, 246, 0.2)"
	}

	lineWidth := 2
	if isSelected {
		lineWidth = 3
	}

	e.ctx.Set("fillStyle", fillColor)
	e.ctx.Call("fillRect", box.X, box.Y, box.Width, box.Height)

	e.ctx.Set("strokeStyle", borderColor)
	e.ctx.Set("lineWidth", lineWidth)
	e.ctx.Call("strokeRect", box.X, box.Y, box.Width, box.Height)

	if isMapped && box.DeviceID != "" {
		e.renderLabel(box)
	}
}

// renderLabel draws device label on box
func (e *BoundingBoxEditor) renderLabel(box BoundingBox) {
	label := box.DeviceID
	if len(label) > 12 {
		label = label[:12]
	}

	e.ctx.Set("font", "11px monospace")
	textWidth := e.ctx.Call("measureText", label).Get("width").Int()
	padding := 4.0

	e.ctx.Set("fillStyle", "#22c55e")
	e.ctx.Call("fillRect", box.X, box.Y-18, float64(textWidth)+padding*2, 18)

	e.ctx.Set("fillStyle", "#fff")
	e.ctx.Set("textBaseline", "middle")
	e.ctx.Call("fillText", label, box.X+padding, box.Y-9)
}

// renderHandles draws resize handles
func (e *BoundingBoxEditor) renderHandles(box BoundingBox) {
	handles := []struct{ x, y float64 }{
		{box.X, box.Y},
		{box.X + box.Width, box.Y},
		{box.X, box.Y + box.Height},
		{box.X + box.Width, box.Y + box.Height},
	}

	for _, handle := range handles {
		e.ctx.Set("fillStyle", "#f59e0b")
		e.ctx.Call("beginPath")
		e.ctx.Call("arc", handle.x, handle.y, 8, 0, js.Global().Get("Math").Call("PI").Float()*2)
		e.ctx.Call("fill")

		e.ctx.Set("fillStyle", "#fff")
		e.ctx.Call("beginPath")
		e.ctx.Call("arc", handle.x, handle.y, 4, 0, js.Global().Get("Math").Call("PI").Float()*2)
		e.ctx.Call("fill")
	}
}

// Helper functions
func removeBox(boxes []BoundingBox, id string) []BoundingBox {
	result := []BoundingBox{}
	for _, box := range boxes {
		if box.ID != id {
			result = append(result, box)
		}
	}
	return result
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func clampFloat(x, min, max float64) float64 {
	if x < min {
		return min
	}
	if x > max {
		return max
	}
	return x
}

func sqrt(x float64) float64 {
	return js.Global().Get("Math").Call("sqrt", x).Float()
}

func pow(x, y float64) float64 {
	return js.Global().Get("Math").Call("pow", x, y).Float()
}

func formatInt(n int) string {
	// Proper integer to string conversion
	if n < 0 {
		return "0"
	}
	if n == 0 {
		return "0"
	}

	// Convert integer to string
	result := ""
	for n > 0 {
		digit := n % 10
		result = string(rune('0'+digit)) + result
		n = n / 10
	}
	return result
}
