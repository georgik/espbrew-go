# ESPBrew Cluster

ESP32 cluster flashing tool written in Go. Manages multiple ESP32 devices across multiple machines with web-based dashboard and CLI tools.

## Features

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
│   ├── dashboard/         # Embedded dashboard files
│   └── config/            # Configuration management
├── pkg/protocol/          # Cluster message types
├── docs/                  # Documentation
└── go.mod
```

## Hardware Support

Supports all ESP32 variants:
- ESP8266
- ESP32 (original)
- ESP32-S2
- ESP32-S3
- ESP32-C2
- ESP32-C3
- ESP32-C5
- ESP32-C6
- ESP32-H2

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
