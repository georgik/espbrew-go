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

### Device Discovery and Configuration

Device identity is managed through an explicit `espbrew.toml` mapping on the leader, not through automatic probing. Auto-probe is disabled by default; discovery is on-demand, non-blocking, and bounded by a timeout so a board that never logs can never hold the port.

```bash
# Discover devices on unconfigured ports (non-blocking, on-demand)
./espbrew --cluster http://leader:8080 device discover          # scan all unconfigured ports
./espbrew --cluster http://leader:8080 device discover /dev/ttyACM0 /dev/ttyACM1
./espbrew --cluster http://leader:8080 device discover --timeout 10s   # per-port probe timeout
./espbrew --cluster http://leader:8080 device discover --save    # record results in espbrew.toml

# Register a device mapping in espbrew.toml (also probes the port locally if run without --cluster)
./espbrew --cluster http://leader:8080 device add /dev/ttyACM0 --id esp-aa:bb:cc:dd:ee:ff --alias lab-1 --chip esp32-s3
./espbrew device add /dev/ttyACM0 --id esp-aa:bb:cc:dd:ee:ff          # local mode

# Remove a device mapping from espbrew.toml (by path, alias, or id)
./espbrew --cluster http://leader:8080 device remove /dev/ttyACM0
./espbrew --cluster http://leader:8080 device remove lab-1
```

`device discover` returns a table of `PATH / DEVICE_ID / CHIP / MAC`. With `--save`, each discovered port is recorded in the leader's `espbrew.toml` and stamped onto live state; without `--save`, nothing is changed and you can register individual ports with `device add`.

See [Device Configuration](#device-configuration) below for the `espbrew.toml` file format and startup behavior.

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

# Device selection by identity (shared with `monitor`); no --port needed
./espbrew --cluster http://leader:8080 flash --filter-alias esp32-c3-lcdkit
./espbrew --cluster http://leader:8080 flash --filter-chip ESP32-S3
```

### Monitor

```bash
./espbrew monitor                 # Auto-detect device
./espbrew monitor -p /dev/ttyUSB0 # Specific port
./espbrew monitor --reset         # Reset to capture boot logs
./espbrew monitor --exit-on "ready" # Exit on pattern
```

**Cluster device selection:** When running against a cluster (`--cluster`) without `--port`,
the monitor auto-selects the first available device. You can narrow the selection the same way
`flash` does, most commonly by `alias` so a board resolves to the right physical port:

```bash
# Select the board by its espbrew.toml alias (mirrors flash --filter-alias)
./espbrew --cluster http://leader:8080 monitor --filter-alias esp32-c3-lcdkit

# Other selectors, all shared with `flash`
./espbrew --cluster http://leader:8080 monitor --filter-chip ESP32-S3     # all S3 boards
./espbrew --cluster http://leader:8080 monitor --filter-board ESP32-S3-BOX-3
./espbrew --cluster http://leader:8080 monitor --filter-tag bench
```

When `--port` is given, no filtering is applied and that port is used directly.

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

See [Snap Documentation](docs/snap.md) for complete details.

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

Device records persist in the embedded database and survive cluster restart. Stable identity is additionally described by an `espbrew.toml` mapping on the leader; see [Device Configuration](#device-configuration). Use the web dashboard to manage devices, or the CLI:

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

1. **Initial Discovery**: A probe captures the MAC address and chip information and stores it in the embedded database. For stable, probe-free identity prefer the explicit [Device Configuration](#device-configuration) mapping instead — the leader can stamp identity onto the record the moment its port connects, without waiting for a probe.
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

### Device Configuration

Device identity is driven by an `espbrew.toml` file rather than automatic probing. The leader loads the mapping on startup, stamps each configured device onto cluster state when its port is present, and writes new mappings there on `device add`/`device discover --save`. This keeps identity explicit and reviewable, instead of relying solely on the embedded database.

The file is auto-detected in the leader's working directory as `espbrew.toml`, or set explicitly with `--config <path>`.

```toml
[[devices]]
path = "/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:30:F9:5A:A3:A0-if00"
id   = "esp-30:30:f9:5a:a3:a0"
alias = "lab-1"
chip  = "ESP32-S3"
description = "Front bench station"
```

**Fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `path` | yes | Device path (stable by-id on Linux, `/dev/ttyACM0`, or a COM port) |
| `id` | no | Device ID; defaults to the MAC-derived ID `esp-<MAC>` after a probe |
| `alias` | no | Human-readable alias (also usable as a lookup match) |
| `chip` | no | Chip type (e.g. `ESP32-S3`) stamped onto the record |
| `description` | no | Free-text description |

**Startup behavior:** On start, the leader reports each configured device as present or absent (via `os.Stat` on the path). Absent ports are not probed or resurrected from hidden persistence; the device is registered automatically the moment it connects.

**Relationship to the dashboard:** The `device add`, `device remove`, and `device discover --save` commands update `espbrew.toml`. Manual edits through the web dashboard still write to the embedded database; prefer the CLI (or the `discover`/`config`/`remove` API endpoints) when you want the change reflected in `espbrew.toml`.

