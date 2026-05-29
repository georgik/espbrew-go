# ESPBrew Cluster

ESP32 cluster flashing tool written in Go. Manages multiple ESP32 devices across multiple machines with web-based dashboard and CLI tools.

## Features

- **Smart File Detection**: Auto-detects ELF, ESP32 binary, and raw firmware files
- **Chip-Specific Offsets**: Correct bootloader offsets per chip variant
- **Multi-Image Flashing**: Flash bootloader, partitions, and app in one command
- **Project Detection**: Automatically detects ESP-IDF, TinyGo, and Rust no_std projects and populates flash paths
- **ESP-IDF Integration**: Read flash_args directly from build directory
- **Cluster Mode**: Leader/peer architecture for distributed flashing
- **Device Discovery**: Automatic ESP device detection via USB serial
- **Device Persistence**: Device information survives cluster restart
- **Device Management**: View, edit, and delete device records via web UI
- **Device Disabling**: Administratively disable devices to prevent accidental flashing
- **Camera Support**: Discover and capture images from connected cameras
- **Job Queue**: Queue and manage flash jobs across all available devices
- **Device Locking**: Prevents concurrent access to serial ports
- **Remote Flashing**: Flash devices from any machine on the network
- **Progress Streaming**: Real-time progress updates via WebSocket
- **Remote Monitor**: Serial monitor over WebSocket with pattern matching
- **Web Dashboard**: Real-time status monitoring via HTTP
- **mDNS**: Automatic node discovery on local network
- **Boot Log Capture**: Reset device to observe startup messages

## Quick Start

```bash
# Build
go build -o espbrew ./cmd/espbrew

# Start standalone cluster (single machine with devices)
./espbrew cluster --role standalone --port 8080

# In another terminal - flash firmware
./espbrew flash firmware.bin

# Or access the dashboard
open http://localhost:8080
```

## Installation

```bash
go build -o espbrew ./cmd/espbrew
sudo mv espbrew /usr/local/bin/
```

## Documentation

- [Cluster Usage](docs/CLUSTER.md) - Multi-node setup, remote operations
- [HTTP API Reference](docs/API.md) - REST and WebSocket endpoints
- [Error Handling](docs/ERROR_HANDLING.md) - Timeouts, retries, recovery mechanisms

## CLI Quick Reference

### Devices

```bash
./espbrew devices              # List all serial devices
./espbrew devices --esp        # List only ESP devices
./espbrew devices --json       # Output as JSON
```

### Flash

```bash
./espbrew flash firmware.bin                     # Auto-detect device
./espbrew flash firmware.bin -p /dev/ttyUSB0     # Specific port
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

```bash
./espbrew capture --list           # List available cameras
./espbrew capture                  # Capture image with defaults
./espbrew capture --width 1920 --height 1080 --quality 90
./espbrew capture my-photo.jpg     # Save to specific file
```

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

Supports all ESP32 variants with chip-specific bootloader offsets and automatic bootloader management:

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

Camera support uses platform-specific tools:

| Platform | Capture Tool | Installation |
|----------|-------------|--------------|
| macOS    | imagesnap   | `brew install imagesnap` |
| Linux    | fswebcam    | `sudo apt install fswebcam` |
| Windows  | (planned)   | - |

### Discovery

Camera discovery uses pion/mediadevices library. Note that on some platforms (especially macOS), camera access requires permissions and may not work from CLI. The capture command will attempt to use the system default camera if discovery fails.

```bash
espbrew capture --list
```

### Capture

Capture images to `~/.espbrew/captures/` with timestamped filenames:

```bash
espbrew capture                                          # Use defaults
espbrew capture --width 1920 --height 1080 --quality 90  # Specify parameters
espbrew capture --camera-id cam-001                    # Specific camera
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

## Development

```bash
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
```

### Test Data

Some integration tests require ESP32 ELF files. See [internal/flash/testdata/README.md](internal/flash/testdata/README.md) for setup instructions.

Tests requiring external data will skip gracefully if not available. To run all tests:

```bash
export ESPBREW_TEST_ELF="/path/to/your/esp32s3-binary"
go test ./internal/flash/... -v
```

## License

MIT
