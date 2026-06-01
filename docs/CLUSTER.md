# ESPBrew Cluster Usage

## Starting a Cluster

### Standalone Mode (Single Machine)

```bash
./espbrew cluster --role standalone --port 8080
```

Standalone mode runs both leader and peer functionality on a single node. This is useful for:
- Testing the cluster locally
- Managing multiple devices on one machine
- Small deployments without distributed needs

### Leader Mode (Cluster Coordinator)

```bash
./espbrew cluster --role leader --port 8080
```

The leader node:
- Coordinates all cluster operations
- Manages the job queue
- Handles device reservations
- Runs the web dashboard
- Discovers and aggregates devices from local and peer nodes
- Distributes work to peer nodes

### Peer Mode (Joins Cluster)

```bash
./espbrew cluster --role peer --leader <leader-ip>:8080 --port 8081
```

Peer nodes:
- Register with the leader via mDNS or explicit address
- Report local devices to leader
- Execute flash jobs assigned by leader
- Send progress updates back

## Cluster Options

| Flag | Description | Default |
|------|-------------|---------|
| `--role` | Node role: leader, peer, standalone | standalone |
| `--port` | HTTP port | 8080 |
| `--bind` | Bind address | 0.0.0.0 |
| `--leader` | Leader address (for peers) | - |
| `--node-id` | Unique node identifier | hostname |
| `--workers` | Number of flash workers | 2 |
| `--no-mdns` | Disable mDNS discovery | false |
| `--log-level` | Log level: debug, info, warn, error | info |
| `-c, --config` | Config file path | - |

## Platform Support

### Linux/macOS

On Linux and macOS, devices are discovered at paths like `/dev/ttyUSB0`, `/dev/ttyACM0`, or `/dev/cu.usbserial-xxx`. These paths are used consistently throughout the cluster.

### Windows

On Windows, ESPBrew automatically detects and manages COM ports (COM1, COM2, etc.). The application handles platform-specific path differences transparently:

- Device detection uses COM port naming
- Cluster communication uses COM port identifiers
- Web interface displays COM ports without Unix-style prefixes
- Serial monitoring automatically uses correct Windows serial port paths

Windows-specific examples:
```bash
# Start cluster on Windows
espbrew.exe cluster --role leader --port 8080

# Flash to specific COM port
espbrew.exe flash firmware.bin -p COM5

# Monitor COM port
espbrew.exe monitor -p COM5
```

The web dashboard at `http://localhost:8080` displays COM ports correctly and provides full monitoring functionality for Windows systems.

## Remote Flashing

Connect to a cluster from any machine:

```bash
# List available devices on cluster
./espbrew --cluster http://leader:8080 devices

# Flash to first available device (auto-select)
./espbrew --cluster http://leader:8080 flash firmware.bin

# Flash to specific device
./espbrew --cluster http://leader:8080 flash firmware.bin -p /dev/ttyUSB0

# Flash with progress bar and monitor after
./espbrew --cluster http://leader:8080 flash firmware.bin --monitor
```

## Remote Monitor

Monitor device serial output remotely:

```bash
# Monitor first available device
./espbrew --cluster http://leader:8080 monitor

# Monitor specific device
./espbrew --cluster http://leader:8080 monitor -p /dev/ttyUSB0

# Monitor with reset to capture boot logs
./espbrew --cluster http://leader:8080 monitor --reset

# Exit on pattern (useful for CI/CD)
./espbrew --cluster http://leader:8080 monitor --exit-on "System ready"
```

### Web Serial Monitor

Browser-based monitoring at `http://leader:8080/monitor`:

- Real-time output via WebSocket
- ANSI color rendering (ESP-IDF logs display with colors)
- Device dropdown with auto-refresh
- Baud rate selection, reset control, pattern matching
- Log download (plain text)
- Pre-select device: `/monitor?device=/dev/ttyUSB0`

Monitor buttons in dashboard device list for quick access.

## Device Reservation

Devices are automatically reserved during operations:
- Flash jobs reserve devices for the duration
- Monitor sessions reserve devices until closed
- Reservation prevents concurrent access conflicts

## Web Dashboard

Access the cluster dashboard at:
```
http://<node-ip>:8080/
```

The dashboard shows:
- Cluster nodes and their status
- Available devices per node
- Active and queued jobs
- Real-time job progress

## mDNS Discovery

Nodes on the same network automatically discover each other via mDNS:
- Leaders advertise themselves as `_espbrew-leader._tcp`
- Peers find leaders automatically if not explicitly set
- Disable with `--no-mdns` for networks without mDNS

## Configuration File

```toml
# ~/.espbrew.toml or /etc/espbrew/config.toml
cluster_name = "espbrew-cluster"
role = "leader"
bind_address = "0.0.0.0"
http_port = 8080
leader_address = ""  # For peer nodes
heartbeat_interval = "5s"
node_timeout = "30s"
log_level = "info"
```

## Example: Multi-Node Setup

```bash
# Terminal 1: Start leader with custom node ID
./espbrew cluster --role leader --port 8080 --node-id "build-server"

# Terminal 2: Start peer 1 with custom node ID
./espbrew cluster --role peer --leader localhost:8080 --port 8081 --node-id "esp-station-1"

# Terminal 3: Start peer 2 with custom node ID
./espbrew cluster --role peer --leader localhost:8080 --port 8082 --node-id "esp-station-2"

# Terminal 4: Flash to cluster (auto-selects available device)
./espbrew --cluster http://localhost:8080 flash firmware.bin
```

**Node names in the dashboard:** Without `--node-id`, nodes use the system hostname. With `--node-id`, you can set descriptive names for easier identification.

## Device Persistence

Device records persist in the embedded database and survive cluster restarts. Each device record stores:

- `device_id`: Unique identifier (ESP-<MAC> format)
- `mac_address`: Hardware MAC address from boot log probe
- `chip_type`: Detected chip variant (esp32, esp32s3, etc.)
- `chip_rev`: Chip revision (e.g., "1.1")
- `flash_size`: Detected flash size
- `psram_size`: Detected PSRAM size
- `last_path`: Most recent connection path (e.g., /dev/ttyUSB0)
- `node_id`: Cluster node where device was last seen
- `first_seen` / `last_seen`: Timestamps for tracking
- `aliases`: Custom device names
- `tags`: User-defined labels
- `disabled`: Administrative disable flag
- `protected`: Flash read-only protection flag

### Device Rediscovery Behavior

When devices are unplugged and reconnected, the cluster automatically handles rediscovery:

```
Initial connection: /dev/ttyUSB0 → Probe → Store (device_id: esp-aa:bb:cc:dd:ee:ff)
Device unplugged: Removed from memory (record kept in database)
Device reconnected: /dev/ttyUSB0 → Restore from database → No duplicate
```

**Key behaviors:**

- **Same Path**: Device reconnecting on same port (e.g., /dev/ttyUSB0) automatically restores existing record - no duplicate created
- **Different Path**: Device on different port (e.g., /dev/ttyUSB0 → /dev/ttyUSB1) creates new entry until probed and matched by MAC
- **Disabled State**: Devices disabled via web UI remain disabled after reconnection
- **Startup Restore**: Cluster loads previously seen devices on startup, marking them as offline until connected

### Database Location

Device records stored in:
```
~/.espbrew/devices.json   # JSON database (legacy)
~/.espbrew/devices.db     # BoltDB database (current)
```

### Deleting Device Records

To remove a device record permanently (allows fresh discovery on next connection):

```bash
# Via web UI: http://localhost:8080 → Devices → Delete device
# Via API: DELETE /api/v1/devices/{device_id}
```
