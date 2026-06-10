# ESPBrew WASM UI

WebAssembly-based interface for ESPBrew built entirely in Go with no external JavaScript dependencies.

## Overview

The WASM interface (V2) provides a modern, responsive web UI for ESPBrew. It runs entirely in the browser as WebAssembly, offering the same functionality as the legacy HTML interface with improved performance and user experience.

## Access

- `http://localhost:8080/v2/` - WASM interface
- `http://localhost:8080/` - Legacy HTML interface

## Features

### Pages

**Dashboard**
- System overview with device, camera, and capture statistics
- Quick action buttons for common tasks
- Connected devices list with status indicators
- Recent activity feed with capture thumbnails

**Capture**
- Camera selection dropdown with automatic discovery
- Real-time camera information display
- Device mapping visualization
- One-click image capture
- Recent captures gallery with modal viewer

**Gallery**
- Grid view of all captured images
- Device-specific filtering
- Capture detail modal with metadata
- Device capture sub-images display
- Per-device brightness, contrast, saturation adjustments

**Devices**
- List of all connected devices with path and chip type
- Device status indicators (Available, Busy, Error)
- Device editing modal for aliases and protection settings
- Real-time device status updates

**Monitor**
- Terminal-style serial output display
- Port selection with baud rate configuration
- Bidirectional communication via WebSocket
- Reset on connect option
- Device reset button
- Real-time output streaming

**Mapping**
- Device-to-camera region assignment
- Canvas-based bounding box editor
- Calibration version tracking
- Device gallery screenshots

**Flash**
- Firmware file upload with progress tracking
- Device selection with chip type display
- Flash progress bar with percentage
- Real-time status updates

**Settings**
- Connection configuration
- Display preferences
- Camera properties configuration

### Components

The WASM UI uses reusable Go components:

- `Button` - Primary and secondary button styles
- `Card` - Content container with optional title
- `Modal` - Closable modal dialogs
- `FormInput` - Text input with label
- `Checkbox` - Toggle-style checkbox

## Building

### Quick Build

```bash
go run cmd/wasm-compiler
```

This command compiles the WASM module and places it in `web/main.wasm`.

### Manual Build

```bash
GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/wasm
```

### WASM Runtime

The interface requires `wasm_exec.js` from the Go SDK. If missing:

```bash
cp $(go env GOROOT)/misc/wasm/wasm_exec.js web/
```

## Project Structure

```
internal/ui/
├── api/              # REST and WebSocket API client
│   ├── camera.go     # Camera discovery and capture
│   ├── capture.go    # Capture gallery and management
│   ├── devices.go    # Device listing and updates
│   ├── flash.go      # Firmware upload and flashing
│   ├── mapping.go    # Device mapping operations
│   └── monitor.go    # Serial monitor WebSocket
├── components/       # Reusable UI components
│   ├── button.go    # Button component
│   ├── card.go      # Card container
│   ├── modal.go     # Modal dialog
│   └── form.go      # Form inputs
├── dom/             # DOM manipulation helpers
│   └── dom.go       # Element wrapper and utilities
├── layout/          # Layout components
│   ├── app.go       # Main application container
│   ├── tabbar.go    # Navigation tab bar
│   └── sidebar.go   # Sidebar navigation
├── pages/           # Page implementations
│   ├── dashboard.go # Dashboard page
│   ├── capture.go   # Capture page
│   ├── gallery.go   # Gallery page
│   ├── devices.go   # Devices page
│   ├── monitor.go   # Serial monitor page
│   ├── mapping.go   # Device mapping page
│   ├── flash.go     # Flash page
│   └── settings.go  # Settings page
└── main.go          # Application entry point
```

## API Client

The WASM UI communicates with the ESPBrew backend via:

- **REST API**: `fetch()` based client for device info, captures, mappings
- **WebSocket**: Real-time updates for serial monitor and flash progress

All API calls use callback-style functions to handle responses asynchronously.

## Serial Monitor

The monitor page provides terminal-style serial communication:

### Features

- Port selection from available devices
- Baud rate selection (9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600)
- Reset on connect option
- Device reset button
- Bidirectional data transfer
- Auto-scrolling output

### WebSocket API

```
ws://host/api/v1/monitor/{port}?baud=115200&reset=1&exit_on=pattern
```

### Message Format

**From Backend:**
```json
{"type":"data","data":"output text"}
```

**To Backend:**
```json
{"type":"data","data":"input text"}
```

## Development

### Adding a New Page

1. Create page function in `internal/ui/pages/`:

```go
package pages

import "codeberg.org/georgik/espbrew-go/internal/ui/layout"

func NewPage(app *layout.App) {
    app.SetTitle("Page Title")
    app.SetMainContentFunc(renderNewPageContent)
}
```

2. Add route in `internal/ui/pages/router.go`:

```go
return map[string]PageFunc{
    // ...
    "newpage": NewPage,
}
```

3. Add tab in `internal/ui/layout/tabbar.go`:

```go
var tabs = []Tab{
    // ...
    {ID: "newpage", Name: "New Page"},
}
```

### Component Pattern

Components follow a consistent pattern:

```go
type Component struct {
    *dom.Element
}

type ComponentConfig struct {
    // Configuration fields
}

func NewComponent(config ComponentConfig) *Component {
    // Create and configure component
}

func (c *Component) SetContent(content *dom.Element) {
    // Update content
}
```

## Technical Notes

### DOM Wrapper

The `dom.Element` wrapper provides Go-friendly access to browser DOM:

- `SetTextContent()` - Set text content
- `SetAttribute()` - Set HTML attributes
- `AddEventListener()` - Attach event handlers
- `Append()` - Add child elements
- `QuerySelector()` - CSS selector queries

### JavaScript Interop

All browser APIs are accessed via `syscall/js`:

```go
// Global object
js.Global()

// Call function
js.Global().Get("setTimeout").Invoke(callback, delay)

// Create function
js.FuncOf(func(this js.Value, args []js.Value) interface{} {
    return nil
})
```

### Event Handling

Event handlers use Go callbacks:

```go
element.AddEventListener("click", func(evt *dom.Event) {
    // Handle event
})
```

## Browser Compatibility

The WASM interface requires:

- WebAssembly support (all modern browsers)
- Fetch API
- WebSocket support
- ES6 JavaScript

Tested on Chrome, Firefox, Safari, and Edge.

## Performance

WASM module size: ~4 MB (uncompressed)
Initial load time: ~2 seconds on typical connection
Subsequent loads: cached by browser

## Debugging

Enable console logging:

```javascript
// In browser console
localStorage.debug = 'true'
```

Check WASM status:

```javascript
// In browser console
window.espbrewUI
```

## License

MIT
