# ESPBrew Cluster

ESP32 cluster flashing tool written in Go. Manages multiple ESP32 devices across multiple machines with web-based dashboard and CLI tools.

## Demo

Try ESPBrew in your browser without installation:

**[Live Demo](https://georgik.github.io/espbrew-go/?demo=true)** - WebAssembly interface running in demo mode

The demo showcases the full WASM UI with mock data for devices, cameras, captures, and serial monitoring.

## Features

ESPBrew is a cluster flashing tool for ESP32. A leader node runs a web dashboard and a job queue; peer nodes report the devices attached to their machines; and the CLI flashes and monitors from anywhere on the network. It runs on Windows, Linux, and macOS.

**Cluster flashing.** Run a single standalone node, or a leader with peers spread across machines. Devices from every node feed one job queue, so you flash to a device without tracking which machine it is on. Node discovery is automatic over mDNS, progress streams in real time over WebSocket, and serial ports are locked so two jobs never collide.

**Flexible flashing.** Flash a single image or a full multi-image (bootloader + partitions + app) in one command. ESP-IDF, TinyGo, and Rust no_std projects are auto-detected and their flash paths filled in; `flash_args` is read straight from an ESP-IDF build directory. Chip-aware offsets handle the rest, and the flasher tolerates a busy port — it retries the brief window when the boot-log probe or a USB re-enumeration holds the node — so a flash is never blocked by that overlap.

**Device management.** Device identity is defined explicitly in an `espbrew.toml` mapping (path to `device_id`, chip, alias, and description) rather than left to automatic probing, so a board is never probed behind a flash job and a busy port never stalls flashing. On-device USB serial is still discovered, but auto-probe stays off; instead an on-demand, non-blocking discovery pass (`device discover`, the `discover` endpoint) identifies unconfigured ports within a fixed timeout, records the ones you choose, and turns itself off again. Records persist across restarts and can be viewed, edited, aliased, tagged, or deleted from the CLI or dashboard. A device can be administratively disabled, or marked read-only (protected) so it can still be monitored.

**Monitoring and cameras.** Live serial monitoring works locally or over the cluster, with exit-on-pattern matching and boot-log capture. Connected cameras are discoverable and capturable, and the `snap` command flashes, monitors serial, and captures a camera frame in a single step. Native USB-hub power control cycles ports for cold-boot resets.

**Web dashboard and simulation.** The WASM dashboard shows real-time device and job status and manages devices, with a browser-only demo mode. Wokwi simulator backends let you flash, monitor, and snap against virtual devices with no hardware attached.

## Quick Start

```bash
# Clone repository
git clone https://github.com/georgik/espbrew-go.git
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

### macOS: cgo and C compiler on PATH

The server uses cgo (via `pion/mediadevices` for the camera feature), so building on macOS requires a working C toolchain with the macOS SDK headers. Go resolves its C compiler (`clang`) from `PATH`. If another tool shadows Apple's `/usr/bin/clang`, the build fails with errors like:

```
# runtime/cgo
fatal error: 'stdlib.h' file not found
```

This commonly happens when [Swiftly](https://swiftly.dev) (or any bundled compiler) is installed and its `bin` directory appears before `/usr/bin` in `PATH`. Its clang lacks the macOS SDK headers. Fix by ensuring Apple's clang is used:

```bash
command -v clang   # should print /usr/bin/clang, not ~/.swiftly/bin/clang
```

If it points elsewhere, either remove that toolchain from `PATH` (check `.zshrc`/`.zprofile`) or override the compiler for a single build:

```bash
CC=/usr/bin/clang go build -o espbrew ./cmd/espbrew
```

## Container Image

A ready-to-use container image is published to the GitHub Container Registry. It
bundles the pre-built release binary and runs espbrew in cluster leader mode,
exposing port 8080. Without a device the image default (root) is fine, but to
mount a serial device you must run the container as your **host user** via
`--userns=keep-id` — see below.

```bash
# Bare cluster only — no hardware attached inside the container:
podman run --rm -p 8080:8080 ghcr.io/georgik/espbrew-go:latest
docker run --rm -p 8080:8080 ghcr.io/georgik/espbrew-go:latest
```

### Running with a serial device attached

To let espbrew see and flash an ESP32 connected to your host, run the container as
your host user via `--userns=keep-id` and mount the device node. This is the same
recipe `orchestrion shell` produces:

```bash
# Linux — device node is usually /dev/ttyACM0 (USB CDC) or /dev/ttyUSB0 (FTDI/CH340/CP210x)
# NOTE: `-e HOME=/tmp` is required under keep-id (see the "State directory" note
# below) until the image is rebuilt with a writable state dir.
podman run --rm -p 8081:8080 --userns=keep-id --group-add=keep-groups --device=/dev/ttyACM0:rwm -e HOME=/tmp ghcr.io/georgik/espbrew-go:latest
docker run --rm -p 8081:8080 --userns=keep-id --group-add=keep-groups --device=/dev/ttyACM0:rwm -e HOME=/tmp ghcr.io/georgik/espbrew-go:latest
```

- `--userns=keep-id` runs the container as your current host user so file
  ownership is preserved. Do **not** hard-code `--user=1000:1000` unless your
  uid/gid are exactly 1000; `keep-id` maps to whatever your user actually is.
- `--group-add=keep-groups` carries your host user's supplementary groups
  (harmless and recommended, but see the critical note below).
- `--device=/dev/ttyACM0:rwm` injects the node at the same path inside the
  container; `:rwm` grants read/write/mmap for flashing.

**Host prerequisite — the device must be world-accessible.** Because the
container runs in a rootless user namespace, the host `dialout` group cannot be
represented inside it, so the node is remapped to `nobody:nogroup` and its group
permission bits are **not** honoured. `--group-add=keep-groups` alone does **not**
let espbrew open the device. On Linux you must make the node world-readable /
writable (`0666`) with a small `udev` rule (for example
`SUBSYSTEM=="tty", MODE="0666"`). See
[container.md](docs/container.md) for the exact rule and why.

For diagnostics you can run an interactive `/bin/sh` shell instead of espbrew —
add `-it --entrypoint /bin/sh` to the command above.

For full details, the host `udev` prerequisite, verification, and troubleshooting,
see [container.md](docs/container.md).

See [docs/ci_setup.md](docs/ci_setup.md) for details on how the image is built
and published (including the manual workflow trigger).

## Documentation

Detailed documentation lives in the [docs/](docs) directory.

| File | Description |
|------|-------------|
| [installation.md](docs/installation.md) | Build and install espbrew, configure environment variables |
| [platform-support.md](docs/platform-support.md) | Windows and Linux specifics: COM ports, stable device paths, permissions, USB hub power control |
| [container.md](docs/container.md) | Running the image: device mounting (`--device …:rwm`), host permissions, verification |
| [simulator.md](docs/simulator.md) | Simulator backends for testing without physical hardware |
| [web-interface.md](docs/web-interface.md) | WASM dashboard, web serial monitor, technical details |
| [cli-commands.md](docs/cli-commands.md) | Reference for all CLI commands |
| [hardware-support.md](docs/hardware-support.md) | Supported chips, file type detection, project detection, ESP-IDF integration, bootloader management |
| [camera-support.md](docs/camera-support.md) | Camera discovery, capture, and image mapping |
| [development.md](docs/development.md) | Building, linting, testing, and end-to-end tests |

Additional reference documents:

- [cluster.md](docs/cluster.md) - Multi-node cluster setup and remote operations
- [snap.md](docs/snap.md) - Flash, monitor, and capture workflow
- [api.md](docs/api.md) - REST and WebSocket API reference
- [power_control.md](docs/power_control.md) - USB hub power control details
- [image_mapping.md](docs/image_mapping.md) - Device-to-camera mapping

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

## License

MIT
