## Web Interface

ESPBrew provides two web interfaces:

### WASM Interface

A modern WebAssembly-based interface at `/` built entirely in Go with no external JavaScript dependencies.

**Access:**
```
http://localhost:8080/
```

**Features:**

- **Dashboard**: System overview with device counts, camera status, and recent captures
- **Capture**: Camera selection, image capture, and preview gallery
- **Gallery**: Browse all captures with device-specific filtering and modal viewer
- **Devices**: View connected devices, edit device attributes, manage protection status
- **Monitor**: Serial terminal with bidirectional communication, baud rate selection, reset options
- **Mapping**: Device-to-camera region mapping for automated screenshot extraction
- **Flash**: Web-based firmware flashing with progress tracking
- **Project Folder**: Browser-based project folder picker for ESP-IDF, Rust ESP, and TinyGo projects
- **Settings**: Connection and display configuration
- **Operational Modes**: Switch between discovery mode (10-second auto-detection) and operational mode (normal flashing)

**Operational Modes:**

The cluster supports two operational modes accessible via the WASM dashboard:

- **Discovery Mode**: Auto-detects ESP devices for 10 seconds after startup, then switches to operational mode
- **Operational Mode**: Normal flashing and monitoring operations

Mode switching is available on the dashboard with current status display and manual control buttons.

```bash
# Using the helper command
go run cmd/wasm-compiler

# Or manually
GOOS=js GOARCH=wasm go build -o web/main.wasm ./cmd/wasm
```

**WASM Runtime:**

The WASM interface requires `wasm_exec.js` from the Go SDK:

```bash
cp $(go env GOROOT)/misc/wasm/wasm_exec.js web/
```

**Technical Implementation:**

The V2 interface is built with pure Go using `syscall/js`:

- `internal/ui/dom` - DOM manipulation helpers
- `internal/ui/components` - Reusable UI components (Button, Card, Modal, etc.)
- `internal/ui/layout` - Layout components (App, TabBar, Sidebar)
- `internal/ui/pages` - Page implementations (Dashboard, Capture, Gallery, etc.)
- `internal/ui/api` - REST and WebSocket API client

**No npm, no JavaScript frameworks, no build tools** - just Go.

**Serial Monitor Features:**

The WASM monitor page provides terminal-style serial communication:

- Bidirectional data transfer via WebSocket
- Baud rate selection (9600 to 921600)
- Reset on connect option
- Device reset button (CTRL+R support)
- Real-time output with immediate display after reset
- Pattern-based exit conditions
- Proper terminal restoration on disconnect

**Recent Improvements:**

- Fixed monitor output buffering after CTRL+R reset
- Improved interrupt handling for clean shutdown
- Non-blocking stdin for responsive keyboard input

**Project Folder Feature:**

The Flash page in the V2 interface includes a Project Folder button that allows selecting entire project folders from the browser. This feature:

- Automatically detects project type (ESP-IDF, Rust ESP, TinyGo)
- Extracts build artifacts (bootloader, partitions, application) from selected folder
- Uploads artifacts to server for flashing when user clicks Flash button
- Provides same functionality as CLI project detection without leaving browser

**Supported Project Types:**

- **ESP-IDF Projects**: Detected by `CMakeLists.txt`, `sdkconfig`, and build output structure
- **Rust ESP Projects**: Detected by `Cargo.toml`, `.cargo/config.toml` with ESP target triples
- **TinyGo Projects**: Detected by `go.mod` with TinyGo dependencies or machine imports

**Browser Support and Limitations:**

The Project Folder feature requires a secure context (HTTPS or localhost) for the File System Access API:

- **Chrome/Edge**: Full support via `showDirectoryPicker()` API
- **Firefox/Safari**: Falls back to `webkitdirectory` attribute (all files prompt)
- **Non-localhost HTTP**: API blocked by browser security, button disabled with warning

**Limitations:**

When accessing the server via non-localhost HTTP (e.g., `http://192.168.1.100:8080`), the File System Access API is unavailable due to browser security requirements. Users in this configuration should:

- Use `http://localhost:8080` for local development
- Configure valid HTTPS certificate for remote access (requires SSL/TLS setup)
- Use alternative flashing methods (CLI, direct file upload)

The button is automatically disabled when the API is unavailable, with a message explaining the limitation. This is a browser security restriction, not an ESPBrew limitation.

**WebSocket API:**
```
ws://host/api/v1/monitor/{port}?baud=115200&reset=1&exit_on=pattern
```

