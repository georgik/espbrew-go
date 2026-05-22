# ESPBrew Cluster

ESP32 cluster flashing tool written in Go. Manages multiple ESP32 devices across multiple machines with web-based dashboard.

## Features

- **Cluster Mode**: Master/Worker architecture for distributed flashing
- **Device Discovery**: Automatic USB serial device detection (platform-specific)
- **Job Queue**: Queue and manage flash jobs across all available devices
- **Web Dashboard**: Real-time status monitoring via HTTP/WebSocket
- **mDNS**: Automatic node discovery on local network
- **Fast Builds**: ~100ms incremental compilation (vs 10-60s in Rust)

## Installation

```bash
go build -o espbrew-cluster ./cmd/espbrew-cluster
```

## Usage

### Standalone Mode (Single Machine)

```bash
espbrew-cluster -r standalone -p 8080
```

### Master Mode (Cluster Coordinator)

```bash
espbrew-cluster -r master -p 8080
```

### Worker Mode (Joins Cluster)

```bash
espbrew-cluster -r worker -master <master-ip>:8080 -p 8081
```

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `-r, --role` | Node role: master, worker, standalone | standalone |
| `-p, --port` | HTTP port | 8080 |
| `--bind` | Bind address | 0.0.0.0 |
| `--master` | Master address (for workers) | - |
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

## Creating Flash Jobs

```bash
# Via API
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"firmware": "/path/to/firmware.bin", "device_path": "/dev/ttyUSB0"}'

# Via dashboard
# Open http://localhost:8080 in your browser
```

## Configuration File (TOML)

```toml
cluster_name = "espbrew-cluster"
role = "master"
bind_address = "0.0.0.0"
http_port = 8080
heartbeat_interval = "5s"
node_timeout = "30s"
log_level = "info"
```

## Development

### Project Structure

```
esp-ci-cluster/
├── cmd/
│   ├── espbrew-cluster/   # Main CLI
│   └── espbrew/           # Alternative CLI
├── internal/
│   ├── cluster/           # Master/Worker, job queue, mDNS
│   ├── device/            # Device discovery, serial scanning
│   ├── flash/             # Flash operations (uses espflasher)
│   ├── http/              # HTTP API, WebSocket, dashboard
│   ├── dashboard/         # Embedded dashboard files
│   └── config/            # Configuration management
├── pkg/protocol/          # Cluster message types
└── go.mod
```

### Building

```bash
# Build
go build ./...

# Run tests
go test ./...

# Run hardware tests (requires ESP32 device)
go test -tags hardware ./...
```

## Hardware Support

Supports all ESP32 variants:
- ESP32 (original)
- ESP32-S2
- ESP32-S3
- ESP32-C2
- ESP32-C3
- ESP32-C5
- ESP32-C6
- ESP32-H2

## License

MIT
