# ESPBrew Cluster

ESP32 cluster flashing tool written in Go. Manages multiple ESP32 devices across multiple machines with web-based dashboard and CLI tools.

## Features

- **Cluster Mode**: Master/Worker architecture for distributed flashing
- **Device Discovery**: Automatic ESP device detection via USB serial
- **Job Queue**: Queue and manage flash jobs across all available devices
- **Device Locking**: Prevents concurrent access to serial ports
- **Web Dashboard**: Real-time status monitoring via HTTP/WebSocket
- **mDNS**: Automatic node discovery on local network
- **CLI Tools**: Flash and monitor ESP devices from command line
- **Boot Log Capture**: Reset device to observe startup messages

## Installation

```bash
# Build CLI tools
go build -o espbrew ./cmd/espbrew
go build -o espbrew-cluster ./cmd/espbrew-cluster
```

## CLI Usage

### List Devices

```bash
# List all serial devices
./espbrew devices

# List only ESP devices
./espbrew devices --esp

# Output as JSON
./espbrew devices --json
```

### Flash Firmware

```bash
# Flash with auto-detected ESP device
./espbrew flash firmware.bin

# Flash to specific port
./espbrew flash firmware.bin -p /dev/ttyUSB0

# Flash with options
./espbrew flash firmware.bin -p /dev/ttyUSB0 \
  --chip esp32-s3 \
  --baud 460800 \
  --no-compress

# Flash and monitor (capture boot logs)
./espbrew flash firmware.bin -m --monitor-reset --monitor-duration 10
```

### Monitor Serial Output

```bash
# Monitor with auto-detected device
./espbrew monitor

# Monitor specific port
./espbrew monitor -p /dev/ttyUSB0

# Monitor with device reset (capture boot logs)
./espbrew monitor --reset

# Exit on pattern (useful for CI/CD)
./espbrew monitor --exit-on "System ready" --exit-on-error "Fatal error"

# Set duration limit
./espbrew monitor --duration 30

# Non-interactive mode (for testing)
./espbrew monitor --no-raw --duration 5
```

### Monitor Controls

- `CTRL+R`: Reset device via RTS/DTR
- `CTRL+C`: Exit monitor

## Cluster Usage

### Standalone Mode (Single Machine)

```bash
./espbrew-cluster cluster --role standalone --port 8080
```

### Master Mode (Cluster Coordinator)

```bash
./espbrew-cluster cluster --role master --port 8080
```

### Worker Mode (Joins Cluster)

```bash
./espbrew-cluster cluster --role worker --master <master-ip>:8080 --port 8081
```

## Cluster Options

| Flag | Description | Default |
|------|-------------|---------|
| `--role` | Node role: master, worker, standalone | standalone |
| `--port` | HTTP port | 8080 |
| `--bind` | Bind address | 0.0.0.0 |
| `--master` | Master address (for workers) | - |
| `--workers` | Number of flash workers | 2 |
| `--no-mdns` | Disable mDNS discovery | false |
| `--log-level` | Log level: debug, info, warn, error | info |
| `-c, --config` | Config file path | - |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/` | Web dashboard |
| GET | `/api/v1/status` | Cluster status |
| GET | `/api/v1/nodes` | List nodes |
| GET | `/api/v1/devices` | List devices |
| GET | `/api/v1/jobs` | List jobs |
| POST | `/api/v1/jobs` | Create flash job |
| WS | `/api/v1/ws` | WebSocket updates |
| WS | `/api/v1/monitor/{port}` | Monitor device stream |

## Creating Flash Jobs via API

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "firmware": "/path/to/firmware.bin",
    "device_path": "/dev/ttyUSB0"
  }'
```

## Configuration File

```toml
cluster_name = "espbrew-cluster"
role = "master"
bind_address = "0.0.0.0"
http_port = 8080
heartbeat_interval = "5s"
node_timeout = "30s"
log_level = "info"
```

## Project Structure

```
esp-ci-cluster/
├── cmd/
│   ├── espbrew/           # CLI tool (flash, monitor, devices)
│   └── espbrew-cluster/   # Cluster daemon
├── internal/
│   ├── cluster/           # Master/Worker, job queue, mDNS
│   ├── device/            # Device discovery, serial scanning
│   ├── flash/             # Flash operations (uses espflasher)
│   ├── http/              # HTTP API, WebSocket, dashboard
│   ├── monitor/           # Serial stream multiplexing
│   ├── dashboard/         # Embedded dashboard files
│   └── config/            # Configuration management
├── pkg/protocol/          # Cluster message types
└── go.mod
```

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

## Flash Options

| Flag | Description | Default |
|------|-------------|---------|
| `--baud` | Flash baud rate | 460800 |
| `--chip` | Chip type (auto, esp32, esp32-s3, etc.) | auto |
| `--fm` | Flash mode (keep, qio, qout, dio, dout) | keep |
| `--ff` | Flash frequency (keep, 80m, 40m, 26m, 20m) | keep |
| `--fs` | Flash size (keep, 1MB, 2MB, 4MB, 8MB, 16MB) | keep |
| `--erase-all` | Erase entire flash before writing | false |
| `--no-compress` | Disable compression | false |
| `--reset` | Reset mode (default, no-reset, usb-jtag, auto) | default |

## License

MIT
