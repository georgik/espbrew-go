# ESPBrew Cluster

ESP32 cluster flashing tool written in Go. Manages multiple ESP32 devices across multiple machines with web-based dashboard and CLI tools.

## Demo

Try ESPBrew in your browser without installation:

**[Live Demo](https://georgik.github.io/espbrew-go/?demo=true)** - WebAssembly interface running in demo mode

The demo showcases the full WASM UI with mock data for devices, cameras, captures, and serial monitoring.

## Features

- **Smart File Detection**: Auto-detects ELF, ESP32 binary, and raw firmware files
- **Chip-Specific Offsets**: Correct bootloader offsets per chip variant
- **Multi-Image Flashing**: Flash bootloader, partitions, and app in one command
- **Project Detection**: Automatically detects ESP-IDF, TinyGo, and Rust no_std projects and populates flash paths
- **ESP-IDF Integration**: Read flash_args directly from build directory
- **Cluster Mode**: Leader/peer architecture for distributed flashing
- **Device Discovery**: Automatic ESP device detection via USB serial with stable /dev/serial/by-id paths on Linux
- **Device Persistence**: Device information survives cluster restart
- **Device Management**: View, edit, and delete device records via web UI and CLI
- **Device Disabling**: Administratively disable devices to prevent accidental flashing
- **Device Protection**: Flash read-only mode for production devices while allowing serial monitoring
- **Camera Support**: Discover and capture images from connected cameras
- **Snap Command**: Flash, monitor serial output, and capture camera image in one command
- **Operational Modes**: Discovery mode for device auto-detection, operational mode for normal flashing
- **Environment Variables**: Configure cluster endpoints via ESPBREW_CLUSTER and ESPBREW_LEADER
- **Job Queue**: Queue and manage flash jobs across all available devices
- **Device Locking**: Prevents concurrent access to serial ports
- **Remote Flashing**: Flash devices from any machine on the network
- **Progress Streaming**: Real-time progress updates via WebSocket
- **Remote Monitor**: Serial monitor over WebSocket with pattern matching
- **Web Dashboard**: Real-time status monitoring via HTTP
- **mDNS**: Automatic node discovery on local network
- **Boot Log Capture**: Reset device to observe startup messages with port busy retry
- **Cross-Platform**: Support for Windows, Linux, and macOS with automatic COM port detection on Windows
- **Simulator Backends**: Wokwi simulator integration for testing without hardware (QEMU planned)

## Quick Start

```bash
# Clone repository
git clone https://codeberg.org/georgik/espbrew-go.git
cd espbrew-go

# Initialize submodules (contains ESP stub loaders)
git submodule update --init --recursive

# Build
go build -o espbrew ./cmd/espbrew

# Windows users: add .exe extension
go build -o espbrew.exe ./cmd/espbrew

# Start standalone cluster (single machine with devices)
./espbrew cluster --role standalone --port 8080
# On Windows:
./espbrew.exe cluster --role standalone --port 8080

# In another terminal - flash firmware
./espbrew flash firmware.bin
# On Windows:
./espbrew.exe flash firmware.bin

# Or access the dashboard
# Linux/macOS: open http://localhost:8080
# Windows: Navigate to http://localhost:8080 in your browser
```

## Installation

```bash
# Clone repository
git clone https://codeberg.org/georgik/espbrew-go.git
cd espbrew-go

# Initialize submodules
git submodule update --init --recursive

# Build the application
go build -o espbrew ./cmd/espbrew

# Linux/macOS: Install to system path
sudo mv espbrew /usr/local/bin/

# Windows: Add espbrew.exe to your PATH or use from current directory
```

## Environment Variables

ESPBrew supports environment variables for configuration without command-line flags:

### Cluster Configuration

- **ESPBREW_CLUSTER**: Default cluster URL for remote operations
- **ESPBREW_LEADER**: Default leader address for cluster mode

```bash
# Set default cluster URL
export ESPBREW_CLUSTER=http://leader:8080

# All cluster commands now use the default
espbrew flash firmware.bin              # Uses ESPBREW_CLUSTER
espbrew monitor                         # Uses ESPBREW_CLUSTER
espbrew snap                             # Uses ESPBREW_CLUSTER

# Override with --cluster flag when needed
espbrew --cluster http://other:8080 flash firmware.bin
```

### Leader Node Configuration

```bash
# Set default leader for peer nodes
export ESPBREW_LEADER=leader:8080

# Start peer with default leader
espbrew cluster --role peer --node-id station-1
```

### Usage Examples

```bash
# Daily workflow with environment variables
export ESPBREW_CLUSTER=http://build-server:8080
idf.py build
espbrew flash                          # Auto-build, auto-flash to cluster
espbrew snap --duration 5              # Quick verification
```

```

## Windows Support

ESPBrew provides full support for Windows with automatic COM port detection and monitoring.

### COM Port Detection

On Windows, ESPBrew automatically detects COM ports (COM1, COM2, etc.) used by ESP32 devices. The application identifies USB serial devices by common patterns including:
- USB modem and serial device identifiers
- Common USB-to-serial chip manufacturers (SLAB, CP21, FTDI, CH340)
- COM port prefixes

### Device Discovery

```bash
# List all serial devices on Windows
espbrew.exe devices

# List only ESP devices
espbrew.exe devices --esp
```

### Flashing on Windows

```bash
# Flash to auto-detected COM port
espbrew.exe flash firmware.bin

# Flash to specific COM port
espbrew.exe flash firmware.bin -p COM5

# Flash with chip specification
espbrew.exe flash firmware.bin -p COM5 --chip esp32-s3
```

### Serial Monitoring on Windows

```bash
# Monitor auto-detected COM port
espbrew.exe monitor

# Monitor specific COM port
espbrew.exe monitor -p COM5

# Monitor with reset to capture boot logs
espbrew.exe monitor -p COM5 --reset
```

### Web Interface on Windows

The web dashboard provides full monitoring capabilities for Windows COM ports:

```
http://localhost:8080
```

The serial monitor interface automatically handles Windows COM port paths without the `/dev/` prefix used on Unix systems.

## Linux Support

ESPBrew provides full support for Linux with stable device paths via `/dev/serial/by-id` and USB hub power control integration.

### Device Discovery

On Linux, ESPBrew automatically discovers USB serial devices using stable `/dev/serial/by-id/` symlinks. This provides persistent device identification across reconnections and reboots.

```bash
# List all serial devices on Linux
./espbrew devices

# List only ESP devices
./espbrew devices --esp

# View detailed device information
./espbrew devices --json
```

### Stable Device Paths

ESPBrew resolves temporary ports (`/dev/ttyUSB0`, `/dev/ttyACM0`) to stable symlinks:
```
/dev/ttyUSB0 → /dev/serial/by-id/usb-Espressif_ESP32-S3-DevKitC-1_1234567890-if00
```

This ensures device records persist across reconnections and system reboots.

### Device Permissions

Access to USB serial devices on Linux typically requires group membership or device-specific permissions.

**Check current access:**
```bash
# Check dialout group membership (for serial ports)
groups | grep dialout

# Check device permissions
ls -la /dev/serial/by-id/
```

**Add user to dialout group:**
```bash
# Add user to dialout group for serial port access
sudo usermod -aG dialout $USER

# Relogin required for group membership to take effect
# Or test immediately:
newgrp dialout
```

**Important**: After adding yourself to a group, you must **log out and log back in** for the change to take effect. Simply restarting your terminal is not enough — the group membership is set at login time.

### USB Hub Power Control (uhubctl)

ESPBrew supports `uhubctl` for USB hub power control on Linux, enabling automated device reset cycles and power management for cluster operations.

**Install uhubctl:**
```bash
# Ubuntu/Debian
sudo apt install uhubctl

# Or build from source
git clone https://github.com/mvp/uhubctl
cd uhubctl && make && sudo make install
```

**Configure Device Access**

uhubctl requires elevated permissions to access USB hub devices. Two approaches:

**Option 1: udev rules (recommended)**

Create a udev rule to grant your user group access to USB hub devices:

```bash
# Get your primary group
MYGROUP=$(id -gn)

# Create udev rule
echo "SUBSYSTEM==\"usb\", ATTR{bDeviceClass}==\"09\", GROUP=\"${MYGROUP}\", MODE=\"0660\"" | sudo tee /etc/udev/rules.d/90-usb-hub.rules

# Reload udev
sudo udevadm control --reload-rules
sudo udevadm trigger
```

This allows uhubctl to run without sudo for your user. Uses your primary group for simplicity — no additional group management needed.

**Option 2: sudoers**

Allow passwordless uhubctl execution:

```bash
echo "$USER ALL=(ALL) NOPASSWD: /usr/bin/uhubctl" | sudo tee /etc/sudoers.d/uhubctl
sudo chmod 440 /etc/sudoers.d/uhubctl
```

**Verify setup:**
```bash
# List hubs
uhubctl

# Test power cycling (requires specific hub/location)
sudo uhubctl -l <bus> -p <port> -P  # Power off
sudo uhubctl -l <bus> -p <port> -p  # Power on
```

### Native USB Hub Power Control

ESPBrew includes native USB hub power control support on Linux (kernel >= 6.0), enabling automated device reset cycles for cold boot testing without external dependencies.

**Requirements:**
- Linux kernel >= 6.0
- USB hub with per-port power switching (e.g., Rosonway RSH-A10, 0BDA:0411)

**Dual-Interface Hub Architecture:**

Modern USB 3.x hubs present as two separate hub devices to the system:
- USB 2.0 hub - handles USB 2.0 traffic
- USB 3.x hub - handles SuperSpeed traffic

For complete power control, both interfaces must be powered simultaneously. ESPBrew automatically detects dual-interface hubs and coordinates power operations across both interfaces.

**Automatic Hub Discovery:**

ESPBrew automatically discovers leaf hubs (hubs with no downstream hub devices) where ESP32 devices are actually connected. This ensures correct power control even when devices are connected through intermediate sub-hubs in the USB topology.

When you specify a hub location, ESPBrew will:
1. Discover all available hubs with per-port power switching
2. Filter to leaf hubs (hubs without downstream hubs on their ports)
3. Automatically pair dual-interface hubs (USB 2.0 + USB 3.x) for coordinated power control

**Commands:**
```bash
# Auto-detect supported hub
./espbrew power auto-detect

# Show hub and port status
./espbrew power status

# Power control (specify leaf hub location from auto-detect or status output)
./espbrew power on <port> --location <leaf-hub-location>
./espbrew power off <port> --location <leaf-hub-location>
./espbrew power cycle <port> --location <leaf-hub-location>

# Power cycle with custom delay
./espbrew power cycle <port> --location <leaf-hub-location> --delay 3s
```

**Example: Rosonway RSH-A10 (10-port hub)**

The Rosonway RSH-A10 presents as multiple hubs in the USB topology. ESPBrew automatically identifies and controls the leaf hubs where devices are connected:

```bash
# Auto-detect finds the correct leaf hub
./espbrew power auto-detect
# Output: Found hub at location: 4-2.3 (USB 3.0)

# Show status to see all discovered hubs
./espbrew power status
# Lists all hubs with their port statuses

# Power control (auto-detect identifies the correct hub)
./espbrew power off 3 --location 4-2.3
./espbrew power on 3 --location 4-2.3

# Power cycle with 3 second delay
./espbrew power cycle 3 --location 4-2.3 --delay 3s
```

ESPBrew automatically pairs the USB 2.0 and USB 3.x leaf hubs and powers both simultaneously for complete device disconnect.

**Permissions:**

The native implementation requires write access to USB device files. Create a udev rule:

```bash
# Add udev rule for USB hub access
sudo tee /etc/udev/rules.d/99-espbrew-hub.rules <<EOF
SUBSYSTEM=="usb", ATTR{idVendor}=="0bda", ATTR{idProduct}=="0411", TAG+="uaccess"
EOF

# Reload rules
sudo udevadm control --reload-rules
sudo udevadm trigger
```

**Kernel Compatibility:**

If running on kernel < 6.0, the power commands will return an error indicating kernel upgrade is required. As an alternative, use the `uhubctl` integration described above.

For detailed implementation information, see [docs/POWER_CONTROL.md](docs/POWER_CONTROL.md).

### Flashing on Linux

```bash
# Flash to auto-detected device
./espbrew flash firmware.bin

# Flash to specific device path
./espbrew flash firmware.bin -p /dev/serial/by-id/usb-Espressif_...

# Flash with chip specification
./espbrew flash firmware.bin --chip esp32-s3
```

### Serial Monitoring on Linux

```bash
# Monitor auto-detected device
./espbrew monitor

# Monitor specific device
./espbrew monitor -p /dev/serial/by-id/usb-Espressif_...

# Monitor with reset to capture boot logs
./espbrew monitor -p /dev/serial/by-id/usb-Espressif_... --reset
```

### Web Interface on Linux

The web dashboard provides full monitoring capabilities:

```
http://localhost:8080
```

The serial monitor interface automatically handles Linux device paths including `/dev/serial/by-id/` symlinks.

## Simulator Backends

ESPBrew supports simulator backends for testing without physical hardware. This enables testing and development workflows without requiring actual ESP32 devices.

### Supported Backends

**Wokwi Simulator**
- ESP32, ESP32-S2, ESP32-S3, ESP32-C3, ESP32-C6 support
- Diagram-based circuit simulation
- Serial output monitoring
- Firmware flashing simulation

**QEMU (Planned)**
- Full system emulation
- Debugging support

### Creating Virtual Devices

Create virtual devices via the REST API:

```bash
# Create a Wokwi device
curl -X POST http://localhost:8080/api/v1/devices/virtual \
  -H "Content-Type: application/json" \
  -d '{
    "device_id": "wokwi-esp32-test",
    "chip_type": "ESP32",
    "description": "Test Wokwi device",
    "backend": "wokwi",
    "backend_config": {
      "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
    }
  }'
```

### Using Virtual Devices

Virtual devices work with the same commands as physical devices:

```bash
# Flash to virtual device
./espbrew flash firmware.bin -d wokwi-esp32-test

# Monitor virtual device
./espbrew monitor -d wokwi-esp32-test

# Snap with virtual device
./espbrew snap firmware.bin -d wokwi-esp32-test
```

### Backend Configuration

Set backend configuration for existing devices:

```bash
# Configure a device as Wokwi simulator
curl -X PUT http://localhost:8080/api/v1/devices/{id}/backend \
  -H "Content-Type: application/json" \
  -d '{
    "backend": "wokwi",
    "backend_config": {
      "chip_type": "ESP32",
      "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
    }
  }'
```

### Requirements

**Wokwi Simulator:**
- Install wokwi-cli: `npm install -g wokwi-cli`
- Valid diagram.json for your chip type
- ELF firmware file

## Documentation

- [Flashing Implementation](docs/FLASHING_IMPLEMENTATION.md) - ESP32-S3 USB-JTAG/Serial support, reset modes, connection handling
- [Snap Command](docs/SNAP.md) - Flash, monitor, capture workflow
- [Cluster Usage](docs/CLUSTER.md) - Multi-node setup, remote operations
- [HTTP API Reference](docs/API.md) - REST and WebSocket endpoints
- [Error Handling](docs/ERROR_HANDLING.md) - Timeouts, retries, recovery mechanisms
- [Hash-Based Flash](docs/HASH_BASED_FLASH.md) - Optimized flashing with hash detection
- [Image Mapping](docs/IMAGE_MAPPING.md) - Device mapping and automated screenshot extraction

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

## CLI Quick Reference

Note: Windows users should use `espbrew.exe` instead of `./espbrew` in the examples below.

### Devices

```bash
./espbrew devices              # List all serial devices
./espbrew devices --esp        # List only ESP devices
./espbrew devices --json       # Output as JSON
```

### Device Management (Cluster)

```bash
# List all cluster devices with detailed information
./espbrew --cluster http://leader:8080 device list

# Delete a device record
./espbrew --cluster http://leader:8080 device delete /dev/serial/by-id/usb-Espressif_...
```

### Flash

```bash
./espbrew flash firmware.bin                     # Auto-detect device
./espbrew flash firmware.bin -p /dev/ttyUSB0     # Specific port
./espbrew flash firmware.bin -p /dev/ttyACM0     # USB-JTAG/Serial device
./espbrew flash firmware.bin --chip esp32-s3     # Specify chip
./espbrew flash firmware.bin --monitor           # Flash and monitor
./espbrew flash firmware.bin --offset 0x10000    # Flash at custom offset

# Preset offsets (recommended)
./espbrew flash firmware.bin --preset bootloader # Chip-specific bootloader offset
./espbrew flash firmware.bin --preset partitions # 0x8000
./espbrew flash firmware.bin --preset app        # 0x10000 (default)

# Multi-image mode
./espbrew flash --bootloader bootloader.bin --partitions partitions.bin --app app.bin

# ESP-IDF integration (reads flash_args)
./espbrew flash --build-dir build/

# Project detection (auto-detects ESP-IDF projects)
cd esp-idf-project
idf.py build
./espbrew --cluster http://leader:8080 flash    # Auto-populates bootloader, partitions, app
./espbrew --cluster http://leader:8080 flash --no-detect  # Disable auto-detection
```

### Monitor

```bash
./espbrew monitor                 # Auto-detect device
./espbrew monitor -p /dev/ttyUSB0 # Specific port
./espbrew monitor --reset         # Reset to capture boot logs
./espbrew monitor --exit-on "ready" # Exit on pattern
```

### Web Serial Monitor

The web-based serial monitor provides real-time serial output with color support and pattern matching:

```
http://localhost:8080/monitor
```

**Features:**
- Real-time serial output via WebSocket
- **Terminal emulator mode**: Type directly to send keystrokes (Enter, Backspace, Ctrl+C, arrows, etc.)
- ANSI color escape sequence rendering (ESP-IDF colored logs)
- Auto-scroll and pause controls
- Pattern matching for automated testing
- Reset device control
- Log download (plain text, ANSI sequences stripped)
- Baud rate selection (9600 - 921600)
- Device pre-selection via URL (`?device=/dev/ttyUSB0`)

**Monitor Button:** Dashboard device list includes "Monitor" button for quick access to each device.

**Terminal Mode:** Click on terminal output to focus, then type normally. Special keys supported:
- Enter, Backspace, Tab, Escape
- Arrow keys, Home, End, Page Up/Down, Delete, Insert
- Ctrl+A through Ctrl+Z, Ctrl+Space

**WebSocket API:** `ws://host/api/v1/monitor/{port}?baud=115200&reset=1&exit_on=pattern`

Send data: `{type: "data", data: "character"}`

### Erase Flash

```bash
./espbrew erase --all                                          # Erase entire flash (auto-detect device)
./espbrew erase -p /dev/ttyUSB0 --all                          # Erase entire flash on specific port
./espbrew erase --address 0x10000 --size 0x1000                # Erase region (hex)
./espbrew erase --address 65536 --size 4096                    # Erase region (decimal)
```

**Cluster Mode:**

```bash
./espbrew --cluster http://leader:8080 erase --device /dev/ttyUSB0 --all
./espbrew --cluster http://leader:8080 erase --device esp-aa:bb:cc:dd:ee:ff --address 0x10000 --size 0x1000
```

### Read Flash

```bash
./espbrew read-flash app.bin                                    # Read to file (auto-detect device)
./espbrew read-flash -p /dev/ttyUSB0 --address 0x10000 app.bin  # Specific port and address
./espbrew read-flash --size 0x100000 app.bin                     # Read 1MB
./espbrew read-flash --chip esp32s3 app.bin                      # Specify chip type
```

**Cluster Mode:**

```bash
./espbrew --cluster http://leader:8080 read-flash --device /dev/ttyUSB0 --address 0x10000 --size 0x100000 app.bin
```

### Camera

The `cameras` command lists available cameras with platform-specific IDs and backend information:

```bash
./espbrew cameras                                  # List available cameras (local)
./espbrew --cluster http://leader:8080 cameras    # List cameras on cluster node
```

The `capture` command provides camera discovery and image capture functionality:

```bash
./espbrew capture --list                          # List available cameras (local)
./espbrew capture                                 # Capture image with defaults
./espbrew capture --width 1920 --height 1080 --quality 90
./espbrew capture my-photo.jpg                    # Save to specific file
```

**Cluster Mode:**

Camera operations can be performed remotely via cluster:

```bash
./espbrew --cluster http://leader:8080 capture --list
./espbrew --cluster http://leader:8080 capture test.jpg
```

**Cluster Capture Features:**
- List cameras connected to cluster nodes
- Capture images from remote cameras
- Automatic download of captured files to local system
- Environment variable support via ESPBREW_CLUSTER

**Camera Selection:**
- `--camera-id`: Specify camera by ID (auto-selects first available if omitted)
- `--width/--height`: Set capture resolution (default: 1280x720)
- `--format`: Output format (default: jpg)
- `--quality`: JPEG quality 1-100 (default: 85)

### Snap

The `snap` command combines flashing, serial monitoring, and camera capture into a single streamlined operation:

```bash
./espbrew snap                                          # Auto-detect device and firmware
./espbrew snap --device esp-aa:bb:cc:dd:ee:ff          # Use device from inventory
./espbrew snap --duration 5                            # Monitor for 5 seconds
./espbrew snap --skip-flash                             # Monitor and capture only
./espbrew snap --no-capture                             # Flash and monitor only
./espbrew snap --cluster http://leader:8080             # Remote snap via cluster
```

**Snap Features:**
- Auto-detects devices and firmware from project directory
- Default 10-second monitoring duration for quick verification
- Hash-based flash optimization skips unchanged regions
- Monitors serial output for boot verification
- Captures camera image for visual confirmation
- Works in local and cluster modes
- Automatic client timeout calculation based on duration

See [Snap Documentation](docs/SNAP.md) for complete details.

### Captures Management

```bash
./espbrew captures list                                    # List all captured images
./espbrew captures delete "2026-05-27/*.jpg"              # Delete by pattern
./espbrew captures delete --all                           # Delete all (with confirmation)
./espbrew captures delete --older-than 7d                 # Delete captures older than 7 days
./espbrew captures delete --yes "2026-05-27/*"            # Delete without confirmation
```

### Cluster

```bash
./espbrew cluster --role leader --port 8080                             # Start leader
./espbrew cluster --role peer --leader IP:8080 --node-id "station-1"    # Start named peer
./espbrew --cluster http://IP:8080 flash firmware.bin                   # Remote flash
./espbrew --cluster http://IP:8080 monitor                              # Remote monitor
./espbrew --cluster http://IP:8080 capture                             # Remote camera capture
./espbrew --cluster http://IP:8080 capture test.jpg                      # Capture and download
./espbrew --cluster http://IP:8080 read-flash --device /dev/ttyUSB0 --address 0x10000 --size 0x100000 app.bin  # Remote read flash
```

### Device Management

Device information is persisted in the embedded database and survives cluster restart. Use the web dashboard to manage devices:

```bash
open http://localhost:8080    # Access dashboard
```

**Device Operations via Web UI:**

- **View Details**: Click on any device to view complete information including MAC address, chip type, flash size, PSRAM, board model, and custom tags
- **Edit Device**: Update chip type, board model, description, aliases, and tags
- **Delete Device**: Remove incorrect device registrations from inventory
- **Manual Addition**: Add devices that cannot be auto-detected

**Device Identification:**

Devices are identified by MAC address when available. The system maintains:
- `device_id`: Unique identifier (ESP-<MAC> format)
- `mac_address`: Hardware MAC address
- `chip_type`: Detected or specified chip variant
- `last_path`: Most recent connection path
- `first_seen` / `last_seen`: Connection timestamps
- `aliases`: Custom names for device identification
- `tags`: User-defined labels for organization

**Device Lookup:**

Devices can be looked up by:
- Device ID (ESP-<MAC>)
- MAC address
- Alias
- Connection path (/dev/ttyUSB0, etc.)

**Device Protection (Read-Only Flash Mode):**

Production devices can be protected from flash operations while remaining accessible for serial monitoring. This prevents accidental firmware overwrites on critical devices.

- **Protection Scope**: Flash and erase operations are blocked
- **Allowed Operations**: Serial monitoring, read operations, device information viewing
- **Persistence**: Protected state survives cluster restarts and device reconnections
- **Management**: Protect/unprotect devices via web dashboard with optional reason tracking
- **Visual Indicators**: Protected devices display "READ-ONLY" badge in device list
- **Use Cases**:
  - Production devices that must retain their firmware
  - Shared workstations where accidental flashing must be prevented
  - Reference devices used for monitoring and testing only

**Device Persistence and Rediscovery:**

Device information persists across cluster restarts and device reconnections. When a device is unplugged and reconnected:

1. **Initial Discovery**: First connection probes the device, captures MAC address and chip information, stores in persistence database
2. **Unplug Handling**: Device removed from in-memory state, but record remains in database with `last_path` tracking the connection path
3. **Reconnection**: Same device on same path automatically restores identity from persistence:
   - Device ID, MAC address, chip type restored from database
   - No duplicate records created
   - Disabled state preserved
   - Aliases and tags retained
4. **Path Changes**: If device moves to different port (e.g., /dev/ttyUSB0 → /dev/ttyUSB1), treated as new connection until probed

This behavior ensures stable device identity across reconnections, useful for:
- Fixed deployments where devices always use same USB port
- Development boards repeatedly connected/disconnected
- Maintaining aliases and tags across sessions

## Project Structure

```
esp-ci-cluster/
├── cmd/
│   └── espbrew/           # CLI tool (flash, monitor, devices, cluster)
├── internal/
│   ├── camera/            # Camera discovery and capture
│   ├── chips/             # Common chip type definitions
│   ├── cluster/           # Leader/peer, job queue, mDNS
│   ├── device/            # Device discovery, serial scanning
│   ├── flash/             # Flash operations, ELF parsing
│   │   ├── bootloaders/   # Bootloader cache manager
│   │   └── espfmt/        # ESP-IDF image format builder
│   ├── http/              # HTTP API, WebSocket, dashboard
│   ├── monitor/           # Serial stream multiplexing
│   ├── project/           # Project detection (ESP-IDF, Rust ESP)
│   ├── dashboard/         # Embedded dashboard files
│   └── config/            # Configuration management
├── pkg/protocol/          # Cluster message types
├── docs/                  # Documentation
└── go.mod
```

## Hardware Support

Supports all ESP32 variants with chip-specific bootloader offsets and automatic bootloader management. ESP32-S3, ESP32-C3, ESP32-C5, ESP32-C6, and ESP32-H2 chips with USB-JTAG/Serial support are automatically detected and use the appropriate reset method.

| Chip      | Bootloader Offset | Bootloader Size | Notes                   |
|-----------|-------------------|-----------------|-------------------------|
| ESP32     | 0x1000            | ~26 KB          | Original ESP32 only     |
| ESP32-S2  | 0x1000            | ~22 KB          |                         |
| ESP32-S3  | 0x0               | ~21 KB          |                         |
| ESP32-C2  | 0x0               | ~20 KB          |                         |
| ESP32-C3  | 0x0               | ~21 KB          |                         |
| ESP32-C5  | 0x2000            | ~22 KB          |                         |
| ESP32-C6  | 0x0               | ~23 KB          |                         |
| ESP32-H2  | 0x0               | ~22 KB          |                         |
| ESP32-C61 | 0x0               | ~22 KB          |                         |
| ESP32-P4  | 0x2000            | ~23 KB          | Rev 0, Rev 1 supported   |

Bootloaders are automatically downloaded from the espflash project repository on first use and cached locally for subsequent operations.

## File Type Detection

ESPBrew automatically detects and processes firmware file types:

- **ELF files**: Automatically converted to ESP-IDF format with bootloader and partition table
- **ESP32 Binary**: Magic 0xE9 (ESP32) or 0xEA (ESP8266)
- **Raw Binary**: Flashed as-is to specified offset

### ELF File Support

When flashing ELF files (Rust no_std or TinyGo ESP projects), ESPBrew:

1. Extracts ROM and RAM segments from the ELF
2. Downloads appropriate bootloader for the target chip
3. Generates default partition table
4. Creates ESP-IDF format image with proper checksums
5. Flashes the complete image

This enables direct flashing of Rust ESP and TinyGo projects without intermediate conversion steps.

### Technical Implementation Notes

ELF to ESP image conversion uses ESP-IDF ExtendedImageHeader format (24 bytes header):

- **Flash Size Encoding**: Byte 3 encodes both size and frequency as `(size_enum << 4) | freq_enum`
  - Size enum: 0x00=1MB, 0x01=2MB, 0x02=4MB, 0x04=16MB, etc.
  - Frequency enum: 0x00=40MHz, 0x01=26MHz, 0x02=20MHz, 0x0F=80MHz
  - Example: 16MB @ 40MHz = 0x40 (0x04 << 4 | 0x00)

- **Segment Merging**: ROM segments from adjacent memory regions are merged with proper padding
  - IROM segment starts at 0x42000000 (ESP32-S3 code flash)
  - DROM segment starts at 0x3C000000 (ESP32-S3 data flash)
  - Segments are sorted by address and merged with zero padding for gaps

- **Image Structure**: App image follows ESP-IDF v3.0 format
  - 24-byte extended header with chip ID, entry point, WP pin
  - Segment headers (8 bytes each): address + length
  - Segment data followed by SHA-256 digest

## Project Detection

ESPBrew automatically detects project types when run from a project directory. This eliminates the need to manually specify paths to bootloader, partition table, and application binaries.

### ESP-IDF Projects

Detection requires:
- `CMakeLists.txt` in project root
- `sdkconfig` or `sdkconfig.defaults` file
- `build/` directory with compiled binaries

When detected, ESPBrew automatically locates:
- `build/bootloader/bootloader.bin`
- `build/partition_table/partition-table.bin`
- `build/<project_name>.bin` (or largest `.bin` file)

```bash
cd esp-idf-project
idf.py build
espbrew --cluster http://leader:8080 flash
# Output: Detected ESP-IDF project, auto-populated flash paths
```

### TinyGo Projects

Detection requires:
- `go.mod` in project root
- `tinygo.org/x/*` dependencies (e.g., `tinygo.org/x/drivers`)
- OR source files importing `"machine"` package

When detected, ESPBrew automatically:
- Converts ELF output to ESP-IDF format with bootloader
- Injects partition table
- Flashes complete image to device

```bash
cd tinygo-project
tinygo build -target=esp32s3-box-3 .
espbrew --cluster http://leader:8080 flash
# Output: Detected tinygo project, auto-populated flash paths
```

**Supported TinyGo targets:** ESP32, ESP32-S2, ESP32-S3, ESP32-C3, ESP32-C6, ESP32-H2

### Rust no_std Projects

Detection requires:
- `Cargo.toml` in project root
- ESP HAL dependencies (e.g., `esp-hal`, `esp-backtrace`)
- `.cargo/config.toml` with ESP target triple

When detected, ESPBrew automatically:
- Converts ELF output to ESP-IDF format with bootloader
- Injects partition table
- Flashes complete image to device

```bash
cd rust-esp-project
cargo build --release
espbrew --cluster http://leader:8080 flash
# Output: Detected rust-esp project, auto-populated flash paths
```

### Disabling Auto-Detection

To disable automatic project detection:

```bash
espbrew flash --no-detect
```

### Explicit Override

Explicitly specified paths always override auto-detected paths:

```bash
espbrew flash --app custom.bin --partitions custom-partitions.bin
```

## ESP-IDF Integration

Use `--build-dir` to flash ESP-IDF projects:

```bash
cd esp-idf-project
idf.py build
espbrew flash --build-dir build/
```

Reads `build/flash_args` for flash settings and file list. Automatically finds files in:
- `{build-dir}/{filename}`
- `{build-dir}/bootloader/{filename}`
- `{filename}` (fallback)

## Bootloader Management

ESPBrew automatically downloads and caches ESP32 bootloaders from the official espflash repository on first use. Bootloaders are stored in `~/.espbrew/bootloaders/` and reused for subsequent flashes.

### Supported Chips

Bootloaders are downloaded from espflash v3.1.0 for:

ESP32, ESP32-S2, ESP32-S3, ESP32-C2, ESP32-C3, ESP32-C5, ESP32-C6, ESP32-C61, ESP32-H2, ESP32-P4

### Custom Bootloaders

To use a custom bootloader binary:

```bash
espbrew flash --bootloader /path/to/custom-bootloader.bin firmware.bin
```

### Cache Management

The bootloader cache is automatically managed:

- First use: Downloads required bootloader (~21 KB per chip)
- Subsequent uses: Loads from cache (no network access)
- Cache location: `~/.espbrew/bootloaders/`

To clear the cache:

```bash
rm -rf ~/.espbrew/bootloaders/
```

### Offline Operation

Once bootloaders are cached, ESPBrew works offline. For air-gapped environments, pre-populate the cache by running espbrew once with network access, or manually copy bootloader binaries to `~/.espbrew/bootloaders/`.

## Camera Support

ESPBrew can discover and capture images from connected cameras, useful for HMI demos, automated testing, and remote monitoring.

### Platform Support

Camera support uses native platform APIs:

| Platform | Backend | Installation |
|----------|---------|--------------|
| macOS    | AVFoundation | Built-in (no external tools required) |
| Linux    | V4L2/fswebcam | `sudo apt install fswebcam` |
| Windows  | DirectShow | Planned |

### Camera Discovery

List available cameras with friendly names and backend information:

```bash
espbrew cameras
```

Output example:
```
Found 2 camera(s):

1. FaceTime HD Camera
   ID:     EAB7A68F-EC2B-4487-AADF-D8A91C1CB782
   Backend: avfoundation
   Path:   EAB7A68F-EC2B-4487-AADF-D8A91C1CB782

2. UVC CAM1
   ID:     0x1134000303a8000
   Backend: avfoundation
   Path:   0x1134000303a8000
```

Camera discovery uses pion/mediadevices library with native platform backends:
- **macOS**: AVFoundation for device enumeration and capture (no external tools)
- **Linux**: V4L2 for device discovery, fswebcam for capture
- **Windows**: DirectShow (planned)

### ESP32-S3-EYE as USB Camera

The ESP32-S3-EYE development board can be used as a USB camera when running appropriate firmware. This enables using ESP32 hardware as part of your camera setup for HMI testing, device monitoring, and automated capture workflows.

**Hardware Setup:**

- ESP32-S3-EYE devkit (includes built-in camera module and USB support)
- Documentation: [ESP32-S3-EYE Getting Started Guide](https://github.com/espressif/esp-who/blob/master/docs/en/get-started/ESP32-S3-EYE_Getting_Started_Guide.md)

**Firmware:**

Use the USB webcam firmware from Espressif's IoT solutions:

- Repository: [esp-iot-solution](https://github.com/espressif/esp-iot-solution)
- Firmware path: `examples/usb/device/usb_webcam`
- Build and flash using ESP-IDF

```bash
# Clone repository
git clone https://github.com/espressif/esp-iot-solution.git
cd esp-iot-solution/examples/usb/device/usb_webcam

# Build with ESP-IDF
idf.py set-target esp32s3
idf.py build

# Flash to ESP32-S3-EYE
idf.py flash
```

**Using ESP32-S3-EYE with ESPBrew:**

After flashing the webcam firmware:
1. Connect ESP32-S3-EYE via USB
2. It appears as a standard USB camera (UVC device)
3. Use `espbrew cameras` to verify detection
4. Capture images normally

```bash
espbrew cameras                    # Should show ESP32-S3-EYE as UVC device
espbrew capture --camera-id <ID>   # Capture from ESP32-S3-EYE
```

**Advantages:**
- Low-cost camera solution using ESP32 hardware
- Direct USB connection, no network setup required
- Integrates with existing ESPBrew workflows
- Useful for multi-device testing setups

### Capture

Capture images to `~/.espbrew/captures/` with timestamped filenames:

```bash
espbrew capture                                          # Use defaults (1280x720, JPEG 85%)
espbrew capture --width 1920 --height 1080 --quality 90  # Specify parameters
espbrew capture --camera-id 0x1134000303a8000          # Specific camera (use ID from cameras command)
espbrew capture output.jpg                              # Custom output path
```

Captured images are organized by date:
```
~/.espbrew/captures/
├── 2026-05-27/
│   ├── cam-abc123-001.jpg
│   ├── cam-abc123-002.jpg
│   └── metadata.json
```

### Storage

- **Location**: `~/.espbrew/captures/`
- **Organization**: Daily directories with metadata.json
- **Formats**: JPEG (default), PNG
- **Metadata**: Camera ID, timestamp, dimensions, file size

### Managing Captures

List and delete captured images via CLI:

```bash
espbrew captures list                    # List all captures with size and date
espbrew captures delete --all            # Delete all (prompts for confirmation)
espbrew captures delete --older-than 7d  # Delete captures older than 7 days
espbrew captures delete "2026-05-*"      # Delete by pattern
```

### Web Dashboard

The ESPBrew dashboard includes camera controls and capture gallery:

- **Camera Tab**: View available cameras, trigger captures with custom resolution
- **Gallery Tab**: Browse captured images with modal viewer
- **Delete**: Remove individual captures via web UI

Access at `http://localhost:8080` when cluster is running.

### Use Cases

- **HMI Testing**: Capture screenshots of GUI demos for verification
- **AI Observation**: Feed camera images to AI for automated testing
- **Remote Monitoring**: Capture and review images from cluster nodes
- **Documentation**: Generate visual records of device states

### Image Mapping and Device Screenshots

ESPBrew can map physical device locations within camera captures using bounding boxes, enabling automated device-specific screenshot extraction. See [Image Mapping Documentation](docs/IMAGE_MAPPING.md) for details.

**Features:**
- **Bounding Box Editor**: Web UI for drawing device regions on camera captures
- **Device Mapping**: Associate device IDs with camera regions
- **Auto-Extraction**: Automatically extract device subimages after capture
- **Per-Device Adjustments**: Configure brightness, contrast, saturation per device
- **Device Gallery**: View device-specific screenshots in web UI
- **Storage**: Device subimages stored alongside full captures

**CLI Commands:**
```bash
# List device mappings
espbrew mapping list --device-id esp-aa:bb:cc:dd:ee:ff

# Create or update mapping
espbrew mapping set --device-id esp-aa:bb:cc:dd:ee:ff --camera /dev/video0 --bounds 0.1,0.2,0.3,0.4

# Capture with device extraction
espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff --camera /dev/video0

# Export/import mappings
espbrew mapping export --device-id esp-aa:bb:cc:dd:ee:ff --output mappings.json
espbrew mapping import mappings.json
```

**Web Interface:**
- Device Mapping tab with canvas-based bounding box editor
- Device gallery thumbnails with click-to-view
- Camera calibration version tracking

## Development

```bash
# Initialize submodules (required for build)
git submodule update --init --recursive

# Update submodules to latest versions
git submodule update --remote

# Format code
gofmt -w .

# Run linter
go vet ./...

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Build all
go build ./...

# Run E2E tests (requires hardware)
make e2e

# Run E2E tests in short mode (skip flash)
make e2e-short
```

### Linting

The project uses [golangci-lint](https://golangci-lint.run/) for code quality checks. CI runs linter automatically on all pull requests.

**Install golangci-lint:**
```bash
# Via install script (recommended)
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Or via package manager
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

**Run linter locally:**
```bash
# Basic run
golangci-lint run

# Run with specific config
golangci-lint run --config .golangci.yml

# Run only specific linters
golangci-lint run --disable-all --enable errcheck,govet

# Fix issues automatically
golangci-lint run --fix
```

**Common Issues and Fixes:**

1. **errcheck (unchecked error returns)**
   ```go
   // Bad: defer port.Close()
   // Good:
   defer func() { _ = port.Close() }()
   
   // Bad: _ = fmt.Scanf(...)
   // Good (scanf returns 2 values):
   _, _ = fmt.Scanf(...)
   
   // Bad: conn.WriteMessage(...)
   // Good:
   _ = conn.WriteMessage(...)
   ```

2. **govet (structural issues)**
   ```go
   // Bad: if v.Kind() == reflect.Ptr
   // Good (inline constant):
   const ptrKind = reflect.Ptr
   if v.Kind() == ptrKind
   ```

3. **ineffassign (ineffectual assignments)**
   ```go
   // Bad: assignment before return
   if oldState != nil {
       term.Restore(int(os.Stdin.Fd()), oldState)
       oldState = nil  // Useless, function returns
   }
   return nil
   
   // Good: remove useless assignment
   if oldState != nil {
       term.Restore(int(os.Stdin.Fd()), oldState)
   }
   return nil
   ```

**CI Configuration:** `.github/workflows/ci.yml` runs linter with latest version.

### End-to-End Tests

E2E tests validate the complete snap workflow against a real cluster server with actual hardware:

```bash
# Via Makefile (recommended)
make e2e              # Full E2E test with flash
make e2e-short        # Skip flash, faster testing

# Via go test
go test -tags=e2e -v -run TestE2E_SnapWithCluster ./cmd/espbrew
go test -tags=e2e -short -v ./cmd/espbrew
```

**Prerequisites:**
- ESP32-S3-Box-3 device connected via USB
- Project firmware built (from test project or your own)
- Device available at `/dev/ttyACM0` or `/dev/ttyUSB0`

**Test Coverage:**
- Server startup with dev mode enabled
- Health check endpoint validation
- Device discovery and selection
- Snap with skip-flash (5s duration)
- Optional snap with flash (full workflow)
- Response validation (snap_id, status, duration)
- Server shutdown via API

**Test Project:** By default uses `/home/georgik/projects/esp32-conways-game-of-life-rs/esp32-s3-box-3`. Modify `e2eProjectDir` in `cmd/espbrew/snap_e2e_test.go` for your setup.

### Stub Loaders

ESP stub loaders are tracked via git submodule from esp-rs/espflash repository. The stubs enable advanced flashing features (MD5 verification, compressed flashing, region erase).

**Update stubs from upstream:**
```bash
# Checkout specific version in submodule
cd vendor/espflash && git checkout v3.1.0 && cd ../..

# Copy stubs to local directory
go run tools/update-stubs.go
```

**Stub location:** `internal/espflash/stubs/` (embedded in binary via go:embed)

### Test Data

Some integration tests require ESP32 ELF files. See [internal/flash/testdata/README.md](internal/flash/testdata/README.md) for setup instructions.

Tests requiring external data will skip gracefully if not available. To run all tests:

```bash
export ESPBREW_TEST_ELF="/path/to/your/esp32s3-binary"
go test ./internal/flash/... -v
```

## License

MIT
