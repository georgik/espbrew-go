# ESPBrew Cluster

ESP32 cluster flashing tool written in Go. Manages multiple ESP32 devices across multiple machines with web-based dashboard and CLI tools.

## Features

- **Smart File Detection**: Auto-detects ELF, ESP32 binary, and raw firmware files
- **Chip-Specific Offsets**: Correct bootloader offsets per chip variant
- **Multi-Image Flashing**: Flash bootloader, partitions, and app in one command
- **Project Detection**: Automatically detects ESP-IDF projects and populates flash paths
- **ESP-IDF Integration**: Read flash_args directly from build directory
- **Cluster Mode**: Leader/peer architecture for distributed flashing
- **Device Discovery**: Automatic ESP device detection via USB serial
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

### Cluster

```bash
./espbrew cluster --role leader --port 8080                             # Start leader
./espbrew cluster --role peer --leader IP:8080 --node-id "station-1"    # Start named peer
./espbrew --cluster http://IP:8080 flash firmware.bin                   # Remote flash
./espbrew --cluster http://IP:8080 monitor                                # Remote monitor
```

## Project Structure

```
esp-ci-cluster/
├── cmd/
│   └── espbrew/           # CLI tool (flash, monitor, devices, cluster)
├── internal/
│   ├── cluster/           # Leader/peer, job queue, mDNS
│   ├── device/            # Device discovery, serial scanning
│   ├── flash/             # Flash operations (uses espflasher)
│   ├── http/              # HTTP API, WebSocket, dashboard
│   ├── monitor/           # Serial stream multiplexing
│   ├── project/           # Project detection (ESP-IDF, etc.)
│   ├── dashboard/         # Embedded dashboard files
│   └── config/            # Configuration management
├── pkg/protocol/          # Cluster message types
├── docs/                  # Documentation
└── go.mod
```

## Hardware Support

Supports all ESP32 variants with chip-specific bootloader offsets:

| Chip      | Bootloader Offset | Notes                   |
|-----------|-------------------|-------------------------|
| ESP8266   | 0x0               |                         |
| ESP32     | 0x1000            | Original ESP32 only     |
| ESP32-S2  | 0x1000            |                         |
| ESP32-S3  | 0x0               |                         |
| ESP32-C2  | 0x0               |                         |
| ESP32-C3  | 0x0               |                         |
| ESP32-C5  | 0x2000            |                         |
| ESP32-C6  | 0x0               |                         |
| ESP32-H2  | 0x0               |                         |
| ESP32-P4  | 0x2000            | Rev 1                   |

## File Type Detection

ESPBrew automatically detects firmware file types:

- **ELF files**: Rejected with error (use `espflash save-image` or build .bin)
- **ESP32 Binary**: Magic 0xE9 (ESP32) or 0xEA (ESP8266)
- **Raw Binary**: Any other data (flashed as-is)

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

## Development

```bash
# Format code
gofmt -w .

# Run linter
go vet ./...

# Run tests
go test ./...

# Build all
go build ./...
```

## License

MIT
