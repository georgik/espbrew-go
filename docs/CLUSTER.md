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

### Linux

On Linux, ESPBrew uses stable device identifiers via `/dev/serial/by-id/` symlinks for reliable device tracking across reconnections:

- Primary path: `/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:30:F9:5A:A3:A0-if00`
- Reference path: `/dev/ttyACM0` (displayed as `real_path` in UI)
- Device identity persists across USB reconnections
- Multiple devices distinguished by serial number

Example device listing:
```
Path:     /dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:30:F9:5A:A3:A0-if00
RealPath: /dev/ttyACM0
DeviceID: (assigned after probe)
Status:   available
```

### macOS

On macOS, devices are discovered at paths like `/dev/ttyUSB0`, `/dev/ttyACM0`, or `/dev/cu.usbserial-xxx`. These paths are used consistently throughout the cluster.

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

### Device Filtering

When multiple devices are connected, you can filter which device to flash based on hardware properties or user-defined tags. All specified filters must match for a device to be selected.

```bash
# Flash only ESP32-S3-BOX boards
./espbrew --cluster http://leader:8080 flash --filter-board "ESP32-S3-BOX" firmware.bin

# Flash devices with "production" tag
./espbrew --cluster http://leader:8080 flash --filter-tag "production" firmware.bin

# Flash ESP32-S3 devices with both "test" and "s3" tags
./espbrew --cluster http://leader:8080 flash --filter-tag "test" --filter-tag "s3" firmware.bin

# Combine filters (ESP32-S3 chip + "dev" tag)
./espbrew --cluster http://leader:8080 flash --filter-chip "ESP32-S3" --filter-tag "dev" firmware.bin
```

**Filter Criteria:**

| Flag | Description | Example |
|------|-------------|---------|
| `--filter-board` | Board model from inventory | `ESP32-S3-BOX`, `ESP32-S3-BOX-3` |
| `--filter-tag` | User-defined tags (repeatable, all must match) | `production`, `test`, `dev` |
| `--filter-chip` | Chip type | `ESP32`, `ESP32-S3`, `ESP32-C3` |

**Filtering Behavior:**

1. ESPBrew queries the cluster API for available devices and their metadata
2. Devices are matched against all specified filter criteria (AND logic)
3. The first available matching device is selected for flashing
4. If no devices match, the command fails with an error message

**Setting Device Properties:**

Device board models and tags can be configured via the web dashboard or API:

```bash
# Via web UI: http://localhost:8080 → Devices → Edit device
# Via API: PATCH /api/v1/devices/{device_id} with board_model and tags fields
```

**Use Cases:**

- Production Deployment: Tag production devices with "prod" and use `--filter-tag "prod"` to target only production units
- Board Variants: Use `--filter-board` when different board types require different firmware variants
- Testing Environments: Tag test devices with "test" to prevent accidentally flashing production hardware
- Hardware Requirements: Filter by chip type when firmware is compiled for specific ESP variants

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

## Device Management

List and manage devices across the cluster:

```bash
# List all cluster devices with detailed information
./espbrew --cluster http://leader:8080 device list

# List devices in JSON format for scripting
./espbrew --cluster http://leader:8080 device list --json

# Delete a device record (clears cached device information)
./espbrew --cluster http://leader:8080 device delete /dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:30:F9:5A:A3:A0-if00

# Delete by device ID
./espbrew --cluster http://leader:8080 device delete esp-30:30:f9:5a:a3:a0
```

**Device Record Fields:**

- `path`: Stable device identifier (by-id on Linux)
- `real_path`: Actual device node (e.g., /dev/ttyACM0)
- `device_id`: Unique identifier assigned after probe
- `chip_type`: Detected chip type (ESP32, ESP32-S3, etc.)
- `status`: Connection status (available, busy, offline)
- `node_id`: Cluster node hosting the device

**Manual Device Editing:**

For devices that cannot be probed (port busy, incompatible device), manually configure via API:

```bash
# Edit device without probe (by path or device_id)
curl -X PATCH http://leader:8080/api/v1/devices/{path_or_id} \
  -H "Content-Type: application/json" \
  -d '{
    "chip_type": "ESP32-S3",
    "mac_address": "30:30:f9:5a:a3:a0",
    "description": "Production unit"
  }'
```

This creates a persistent device record even when probe fails.

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

## Job Execution Flow

The cluster implements a distributed job execution system where the leader routes jobs to the appropriate node based on device ownership.

### Step-by-Step Process

1. **Client submits job to leader**
   ```
   POST /api/v1/jobs
   {
     "firmware": "firmware.bin",
     "device_path": "/dev/ttyUSB0"
   }
   ```
   - Job is added to the leader's queue
   - Job receives unique ID and status "pending"

2. **Leader dispatches job**
   - Dispatcher checks device ownership via `DeviceNodeID` field
   - If device is local: executes directly
   - If device is on peer: dispatches via HTTP

3. **Peer receives job assignment**
   ```
   POST /api/v1/jobs/assign
   {
     "job_id": "abc-123",
     "device_path": "/dev/ttyUSB0",
     "type": "flash"
   }
   ```
   - Peer validates device ownership
   - Device status changes to "busy"
   - Execution goroutine starts

4. **Peer reports progress**
   ```
   POST /api/v1/nodes/{node_id}/jobs/{job_id}/progress
   {
     "job_id": "abc-123",
     "status": "running",
     "progress": 50,
     "node_id": "peer-1"
   }
   ```
   - Leader updates job progress in queue
   - Clients can poll `/api/v1/jobs/{id}` for status

5. **Job completion**
   - Peer sends final status: `completed` or `failed`
   - Leader marks job complete/failed
   - Device status changes back to "available"
   - Error message stored if failed

### Error Handling

- **Peer offline**: Job marked as failed, device released
- **Device unavailable**: Peer rejects job (409), leader retries
- **Execution timeout**: Job marked as timed out after 10 minutes
- **Network failure**: Progress updates may be lost, job eventually times out

### Client Polling

Clients poll for job status:
```bash
# Check job status
curl http://leader:8080/api/v1/jobs/{job_id}

# Response
{
  "id": "abc-123",
  "status": "running",
  "progress": 75,
  "device_path": "/dev/ttyUSB0",
  "device_node": "peer-1"
}
```

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

## Observability

### Prometheus Metrics

All cluster nodes expose Prometheus metrics on the `/metrics` endpoint:

```bash
curl http://localhost:8080/metrics
```

**Available Metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `espbrew_cluster_heartbeat_success_total` | Counter | node_id, direction | Total successful heartbeats |
| `espbrew_cluster_heartbeat_latency_seconds` | Histogram | node_id, direction | Heartbeat latency distribution |
| `espbrew_cluster_node_count` | Gauge | - | Current number of nodes |
| `espbrew_cluster_node_up` | Gauge | node_id, role | Node availability (1=up, 0=down) |
| `espbrew_cluster_jobs_queued_total` | Counter | - | Total jobs queued |
| `espbrew_cluster_jobs_started_total` | Counter | - | Total jobs started |
| `espbrew_cluster_jobs_completed_total` | Counter | status | Total jobs completed by status |
| `espbrew_cluster_job_duration_seconds` | Histogram | status | Job execution duration |
| `espbrew_cluster_queue_size` | Gauge | - | Current queue depth |
| `espbrew_cluster_device_count` | Gauge | backend | Device count by backend type |
| `espbrew_cluster_device_available` | Gauge | node_id | Available devices per node |
| `espbrew_cluster_device_busy` | Gauge | node_id | Busy devices per node |
| `espbrew_cluster_device_offline` | Gauge | node_id | Offline devices per node |
| `espbrew_cluster_peer_rejoin_total` | Counter | node_id | Peer rejoins after timeout |
| `espbrew_cluster_peer_recovery_success_total` | Counter | node_id | Successful peer recoveries |
| `espbrew_cluster_peer_recovery_failure_total` | Counter | node_id, reason | Failed peer recoveries |
| `espbrew_cluster_peer_timeout_total` | Counter | node_id | Peer timeout events |

**Grafana Dashboard Example:**

```promql
# Cluster node availability
espbrew_cluster_node_up

# Job completion rate (last 5m)
rate(espbrew_cluster_jobs_completed_total[5m])

# Average job duration
rate(espbrew_cluster_job_duration_seconds_sum[5m]) / rate(espbrew_cluster_job_duration_seconds_count[5m])

# Queue depth over time
espbrew_cluster_queue_size
```

## Static Peer Configuration

For multi-subnet deployments or environments without mDNS, configure static peers:

**Config file (`~/.espbrew/config.toml`):**

```toml
role = "leader"
http_port = 8080

[[static_peers]]
id = "peer-station-1"
address = "192.168.1.100"
port = 8081

[[static_peers]]
id = "peer-station-2"
address = "192.168.1.101"
port = 8082
```

**Fields:**

- `id`: Unique identifier for the peer
- `address`: IP address or hostname
- `port`: HTTP port where peer listens

The leader will attempt to connect to static peers on startup and periodically retry unhealthy peers. Static peers coexist with mDNS discovery - peers discovered via mDNS are also registered.

**Health Monitoring:**

Static peer health is tracked via metrics:

```bash
curl http://localhost:8080/metrics | grep peer_health
```

Unhealthy peers are retried every 30 seconds.
