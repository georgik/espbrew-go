# ESPBrew HTTP API Reference

## Base URL

All endpoints are prefixed with `/api/v1`

## Authentication

Currently no authentication. Use firewall/network isolation for security.

## Endpoints

### Health Check

```
GET /health
```

Response:
```json
{
  "status": "healthy",
  "time": "2026-05-22T10:00:00Z"
}
```

### Cluster Status

```
GET /api/v1/status
```

Response:
```json
{
  "nodes_count": 3,
  "devices_count": 6,
  "jobs_count": 2,
  "role": "leader",
  "queue_size": 1
}
```

### List Nodes

```
GET /api/v1/nodes
```

Response:
```json
[
  {
    "id": "node-abc123",
    "role": "leader",
    "address": "192.168.1.10",
    "port": 8080,
    "last_seen": "2026-05-22T10:00:00Z"
  }
]
```

### List Devices

```
GET /api/v1/devices
```

Response:
```json
[
  {
    "path": "/dev/ttyUSB0",
    "vid": "0x4348",
    "pid": "0x0028",
    "status": "available",
    "node_id": "node-abc123"
  }
]
```

Device states:
- `available` - Device is free to use
- `busy` - Device is in use
- `reserved` - Device is reserved for a client
- `offline` - Device is not currently connected
- `disabled` - Device is administratively disabled

Device response fields:
- `path` - Current connection path (empty if offline)
- `device_id` - Unique device identifier
- `chip_type` - Detected chip variant
- `board_model` - Board model from inventory (if set)
- `status` - Connection status
- `connected` - Boolean indicating if device is currently connected
- `vid` / `pid` - Vendor/product IDs (if connected)
- `serial` - MAC address (if available)
- `aliases` - Custom device names
- `tags` - User-defined labels
- `disabled` - Boolean indicating if device is administratively disabled
- `disabled_reason` - Reason for disabling (if disabled)
- `disabled_by` - Identifier of user/client who disabled the device
- `disabled_at` - Timestamp when device was disabled
- `protected` - Boolean indicating if device is protected (flash read-only mode)
- `protected_reason` - Reason for protection (if protected)
- `protected_by` - Identifier of user/client who protected the device
- `protected_at` - Timestamp when device was protected

### Device Detail

```
GET /api/v1/devices/{id}
```

Retrieves detailed information about a specific device. The device can be looked up by:
- Device ID (ESP-<MAC>)
- MAC address
- Alias
- Last connection path

Response:
```json
{
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "mac_address": "aa:bb:cc:dd:ee:ff",
  "chip_type": "ESP32-S3",
  "chip_rev": "1.0",
  "flash_size": 8388608,
  "psram_size": 8388608,
  "psram_type": "QSPI",
  "board_model": "ESP32-S3-DevKitC-1",
  "description": "Development board",
  "first_seen": "2026-05-01T10:00:00Z",
  "last_seen": "2026-05-28T15:30:00Z",
  "last_path": "/dev/ttyUSB0",
  "node_id": "node-abc123",
  "aliases": ["devkit-s3", "station-1"],
  "tags": ["dev", "testing"],
  "connected": true,
  "current_path": "/dev/ttyUSB0",
  "status": "available"
}
```

### Update Device

```
PUT /api/v1/devices/{id}
PATCH /api/v1/devices/{id}
Content-Type: application/json
```

Updates device information. Only provided fields are updated. Empty fields are ignored.

Request:
```json
{
  "mac_address": "aa:bb:cc:dd:ee:ff",
  "chip_type": "ESP32-S3",
  "chip_rev": "1.0",
  "flash_size": 8388608,
  "psram_size": 8388608,
  "psram_type": "QSPI",
  "board_model": "ESP32-S3-DevKitC-1",
  "description": "Development board",
  "aliases": ["devkit-s3"],
  "tags": ["dev", "testing"]
}
```

Response:
```json
{
  "status": "updated",
  "device_id": "esp-aa:bb:cc:dd:ee:ff"
}
```

### Delete Device

```
DELETE /api/v1/devices/{id}
```

Removes a device from inventory. Also removes the device from in-memory cluster state if currently connected.

Response:
```json
{
  "status": "deleted",
  "device_id": "esp-aa:bb:cc:dd:ee:ff"
}
```

Note: This only removes the device record from inventory. It does not affect the physical device or prevent it from being re-registered later.

### Disable Device

```
PUT /api/v1/devices/{id}/disable
POST /api/v1/devices/{id}/disable
Content-Type: application/json
```

Administratively disables a device to prevent it from being used for flashing or monitoring. The disabled state persists across cluster restarts.

Request body (optional):
```json
{
  "reason": "Device under maintenance",
  "client_id": "admin-user"
}
```

Response (200 OK):
```json
{
  "status": "disabled",
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "path": "/dev/ttyUSB0"
}
```

Response (404 Not Found):
```json
{
  "error": "device not found"
}
```

Response (409 Conflict):
```json
{
  "error": "cannot disable device that is currently in use"
}
```

Notes:
- Disabled devices cannot be flashed or reserved
- The device can be identified by device ID, path, or serial number
- Disabled state is persisted and survives cluster restarts
- A device cannot be disabled if it is currently busy or reserved

### Enable Device

```
PUT /api/v1/devices/{id}/enable
POST /api/v1/devices/{id}/enable
Content-Type: application/json
```

Re-enables a previously disabled device.

Request body (optional):
```json
{
  "client_id": "admin-user"
}
```

Response (200 OK):
```json
{
  "status": "enabled",
  "device_id": "esp-aa:bb:cc:dd:ee:ff"
}
```

Response (404 Not Found):
```json
{
  "error": "device not found"
}
```

### Protect Device

```
PUT /api/v1/devices/{id}/protect
POST /api/v1/devices/{id}/protect
Content-Type: application/json
```

Protects a device from flash and erase operations while allowing serial monitoring and read operations. The protected state persists across cluster restarts.

Request body (optional):
```json
{
  "reason": "Production device - firmware must not be changed",
  "client_id": "admin-user"
}
```

Response (200 OK):
```json
{
  "status": "protected",
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "path": "/dev/ttyUSB0"
}
```

Response (404 Not Found):
```json
{
  "error": "device not found"
}
```

Notes:
- Protected devices cannot be flashed or erased
- Serial monitoring and read operations remain available
- Flash operations return 403 Forbidden with appropriate error message
- The device can be identified by device ID, path, or serial number
- Protected state is persisted and survives cluster restarts
- Protection is useful for production devices that must retain their firmware

### Unprotect Device

```
PUT /api/v1/devices/{id}/unprotect
POST /api/v1/devices/{id}/unprotect
Content-Type: application/json
```

Removes protection from a device, restoring flash and erase capabilities.

Request body (optional):
```json
{
  "client_id": "admin-user"
}
```

Response (200 OK):
```json
{
  "status": "unprotected",
  "device_id": "esp-aa:bb:cc:dd:ee:ff"
}
```

Response (404 Not Found):
```json
{
  "error": "device not found"
}
```

### Add Device (Manual)

```
POST /api/v1/devices
Content-Type: application/json
```

Manually adds a device to inventory for devices that cannot be auto-detected or probed.

Request:
```json
{
  "path": "/dev/ttyUSB0",
  "mac_address": "aa:bb:cc:dd:ee:ff",
  "chip_type": "ESP32-S3",
  "chip_rev": "1.0",
  "flash_size": 8388608,
  "psram_size": 8388608,
  "psram_type": "QSPI",
  "board_model": "ESP32-S3-DevKitC-1",
  "description": "Development board",
  "aliases": ["devkit-s3"],
  "tags": ["dev"]
}
```

Required field: `chip_type`. All other fields are optional.

Response:
```json
{
  "status": "created",
  "device_id": "esp-aa:bb:cc:dd:ee:ff"
}
```

### Probe Device

```
POST /api/v1/devices/probe
Content-Type: application/json
```

Probes a device to read its boot log and extract device information. Device must be in bootloader mode.

Request:
```json
{
  "path": "/dev/ttyUSB0"
}
```

Response:
```json
{
  "status": "probed",
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "chip_type": "ESP32-S3",
  "path": "/dev/ttyUSB0"
}
```

Note: To put ESP32 devices in bootloader mode:
1. Hold BOOT button
2. Press RESET button
3. Release BOOT button

### Reserve Device

```
POST /api/v1/devices/{path}/reserve
```

Request:
```json
{
  "client_id": "cli-session-123",
  "ttl": 300
}
```

Response (200 OK):
```json
{
  "status": "reserved",
  "device": "/dev/ttyUSB0",
  "client_id": "cli-session-123"
}
```

Response (409 Conflict):
```json
{
  "error": "Device already reserved by: cli-session-456"
}
```

### Release Device

```
DELETE /api/v1/devices/{path}/reserve
```

Request:
```json
{
  "client_id": "cli-session-123"
}
```

Response:
```json
{
  "status": "released",
  "device": "/dev/ttyUSB0"
}
```

### List Boards

```
GET /api/v1/boards
```

Returns a grouped view of connected ESP boards. Multiple ports belonging to the same physical board (identified by MAC address) are grouped together. This endpoint is useful for:

- Identifying boards with multiple connection options (UART + USB Serial/JTAG)
- Determining the recommended port for flashing and monitoring
- Automation tools that need to understand port relationships

Response:
```json
{
  "targets": [
    {
      "target": "ESP32-C5",
      "mac": "30:ed:a0:e4:6a:d0",
      "uart": [
        {
          "type": "usb_serial_jtag",
          "port": "/dev/cu.usbmodem1201"
        },
        {
          "type": "uart",
          "port": "/dev/cu.usbserial-110"
        }
      ],
      "jtag": [],
      "recommended": {
        "flash_port": "/dev/cu.usbmodem1201",
        "monitor_port": "/dev/cu.usbmodem1201",
        "reason": "first_identified_port"
      }
    },
    {
      "target": "ESP32",
      "mac": "84:0d:8e:18:8a:d0",
      "uart": [
        {
          "type": "uart",
          "port": "/dev/ttyUSB0"
        }
      ],
      "jtag": [],
      "recommended": {
        "flash_port": "/dev/ttyUSB0",
        "monitor_port": "/dev/ttyUSB0",
        "reason": "only_identified_port"
      }
    }
  ],
  "unidentified_ports": [],
  "metadata": {
    "platform": "linux",
    "scan_time": "2026-06-04T15:30:00Z"
  }
}
```

Port types:
- `uart` - External UART bridge (CP2102, FT2232, etc.)
- `usb_serial_jtag` - Native USB Serial/JTAG interface

Recommendation reasons:
- `usb_serial_jtag_preferred` - USB Serial/JTAG port selected for reliability
- `only_identified_port` - Single port available
- `first_identified_port` - First port that successfully identified the chip
- `fallback_first_port` - No identified ports, using first available

### Backend Configuration

#### Get Device Backend Config

```
GET /api/v1/devices/{id}/backend
```

Retrieves the backend configuration for a device.

Response:
```json
{
  "backend": "wokwi",
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "backend_config": {
    "chip_type": "ESP32",
    "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
  }
}
```

Backend types:
- `physical` - Real hardware via serial port
- `wokwi` - Wokwi simulator
- `qemu` - QEMU emulator (future)

#### Set Device Backend Config

```
PUT /api/v1/devices/{id}/backend
PATCH /api/v1/devices/{id}/backend
Content-Type: application/json
```

Sets the backend configuration for a device.

Request:
```json
{
  "backend": "wokwi",
  "backend_config": {
    "chip_type": "ESP32",
    "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
  }
}
```

Wokwi backend config:
- `chip_type` - ESP32 chip variant (ESP32, ESP32-S2, ESP32-S3, ESP32-C3, ESP32-C6)
- `diagram_json` - Wokwi diagram.json content
- `api_token` - Wokwi API token (optional). If set, uses Wokwi WebSocket API instead of wokwi-cli

QEMU backend config (future):
- `machine_type` - Machine type (esp32, esp32s3, etc.)
- `memory_size` - Memory size in MB

Physical backend has no additional config.

#### Create Virtual Device

```
POST /api/v1/devices/virtual
Content-Type: application/json
```

Creates a new virtual device (simulator).

Request:
```json
{
  "device_id": "wokwi-esp32-test",
  "chip_type": "ESP32",
  "description": "Test Wokwi device",
  "backend": "wokwi",
  "backend_config": {
    "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
  }
}
```

If `diagram_json` is not provided, a default diagram will be generated based on the chip type.

Response:
```json
{
  "device_id": "wokwi-esp32-test",
  "chip_type": "ESP32",
  "backend": "wokwi",
  "backend_config": {
    "wokwi": {
      "chip_type": "ESP32",
      "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
    }
  }
}
```

#### List Virtual Devices

```
GET /api/v1/devices/virtual
```

Lists all virtual devices (simulators).

Response:
```json
[
  {
    "device_id": "wokwi-esp32-test",
    "chip_type": "ESP32",
    "backend": "wokwi",
    "backend_config": {
      "wokwi": {
        "chip_type": "ESP32",
        "diagram_json": "{\"version\":1,\"parts\":[{\"type\":\"esp32-devkitC\",\"id\":\"chip\"}]}"
      }
    }
  }
]
```

#### Delete Virtual Device

```
DELETE /api/v1/devices/virtual/{id}
```

Deletes a virtual device. Only virtual devices (wokwi, qemu) can be deleted through this endpoint.

Response: `204 No Content`

### List Jobs

```
GET /api/v1/jobs
```

Response:
```json
[
  {
    "id": "job-abc123",
    "device_path": "/dev/ttyUSB0",
    "status": "running",
    "progress": 45,
    "created_at": "2026-05-22T10:00:00Z"
  }
]
```

Job statuses:
- `pending` - Job is queued
- `assigned` - Job assigned to worker
- `running` - Job is executing
- `completed` - Job finished successfully
- `failed` - Job failed with error
- `cancelled` - Job was cancelled by user
- `timeout` - Job exceeded time limit

### Get Job

```
GET /api/v1/jobs/{id}
```

Response:
```json
{
  "id": "job-abc123",
  "device_path": "/dev/ttyUSB0",
  "status": "completed",
  "progress": 100,
  "created_at": "2026-05-22T10:00:00Z",
  "completed_at": "2026-05-22T10:00:05Z"
}
```

### Cancel Job

```
DELETE /api/v1/jobs/{id}
```

Cancels a pending or running job. Releases the device reservation.

Response (200 OK):
```json
{
  "status": "cancelled",
  "job_id": "job-abc123"
}
```

Response (404 Not Found):
```json
{
  "error": "job not found: job-abc123"
}
```

Response (409 Conflict):
```json
{
  "error": "cannot cancel job in state: completed"
}
```

### Upload Firmware

```
POST /api/v1/flash/upload
Content-Type: multipart/form-data
```

Form fields:
- `firmware`: Binary file (max 32MB)

Response:
```json
{
  "file_id": "abc123-def456",
  "size": 137920
}
```

### Submit Flash Job

```
POST /api/v1/flash
Content-Type: application/json
```

Request:
```json
{
  "device_path": "/dev/ttyUSB0",
  "file_id": "abc123-def456",
  "client_id": "espbrew-cli"
}
```

Response (200 OK):
```json
{
  "job_id": "job-xyz789",
  "status": "pending",
  "device_path": "/dev/ttyUSB0"
}
```

### Submit Erase Job

```
POST /api/v1/flash/erase
Content-Type: application/json
```

Request (full erase):
```json
{
  "device_path": "/dev/ttyUSB0",
  "erase_all": true,
  "client_id": "espbrew-cli"
}
```

Request (region erase):
```json
{
  "device_path": "/dev/ttyUSB0",
  "address": 65536,
  "size": 4096,
  "client_id": "espbrew-cli"
}
```

Fields:
- `device_path` (required): Serial port path
- `erase_all` (optional): Erase entire flash (default: false)
- `address` (optional): Start address for region erase (hex or decimal)
- `size` (optional): Size in bytes for region erase (hex or decimal)
- `client_id` (optional): Client identifier for tracking

Either `erase_all: true` or both `address` and `size` must be provided for region erase.

Response (200 OK):
```json
{
  "job_id": "erase-xyz789",
  "status": "pending",
  "device_path": "/dev/ttyUSB0"
}
```

Error responses:
- `400 Bad Request` - Missing device_path or invalid parameters
- `403 Forbidden` - Device is disabled
- `409 Conflict` - Device not found or unavailable

### Flash Progress WebSocket

```
WS /api/v1/flash/{job_id}/progress
```

Messages from server:

Init (sent on connect):
```json
{
  "type": "init",
  "job_id": "job-xyz789",
  "progress": 0,
  "status": "pending"
}
```

Progress update:
```json
{
  "type": "progress",
  "job_id": "job-xyz789",
  "progress": 45,
  "status": "running"
}
```

Complete:
```json
{
  "type": "complete",
  "job_id": "job-xyz789",
  "status": "completed"
}
```

Error:
```json
{
  "type": "complete",
  "job_id": "job-xyz789",
  "status": "failed",
  "error": "Connection lost"
}
```

### Read Flash Memory

Submits a job to read flash memory from a device. The read operation executes asynchronously and the data is available for download once complete.

```
POST /api/v1/flash/read
Content-Type: application/json
```

Request:
```json
{
  "device_path": "/dev/ttyUSB0",
  "address": 65536,
  "size": 1048576,
  "chip": "esp32s3",
  "client_id": "cli-session-123"
}
```

Fields:
- `device_path` (required): Serial port path
- `address` (required): Flash address to read from
- `size` (required): Number of bytes to read (max 16MB)
- `chip` (optional): Chip type for auto-detection
- `client_id` (optional): Client identifier for tracking

Response:
```json
{
  "job_id": "read-abc123",
  "status": "pending",
  "device_path": "/dev/ttyUSB0",
  "size": 1048576
}
```

### Read Flash Status

Checks the status of a read job and retrieves download URL when complete.

```
GET /api/v1/flash/read/{job_id}
```

Response (pending/running):
```json
{
  "job_id": "read-abc123",
  "status": "running",
  "device_path": "/dev/ttyUSB0",
  "size": 1048576,
  "download_url": null,
  "error": null
}
```

Response (completed):
```json
{
  "job_id": "read-abc123",
  "status": "completed",
  "device_path": "/dev/ttyUSB0",
  "size": 1048576,
  "download_url": "/api/v1/flash/download/read-abc123",
  "error": null
}
```

Response (failed):
```json
{
  "job_id": "read-abc123",
  "status": "failed",
  "device_path": "/dev/ttyUSB0",
  "size": 0,
  "download_url": null,
  "error": "Connection lost"
}
```

### Download Read Data

Downloads the flash read data for a completed job.

```
GET /api/v1/flash/download/{job_id}
```

Response: Binary data (`application/octet-stream`)

Headers:
- `Content-Type: application/octet-stream`
- `Content-Disposition: attachment; filename="flash-read-{job_id}.bin"`
- `Content-Length: {actual_size}`

Example curl usage:
```bash
# Submit read job
curl -X POST http://localhost:8080/api/v1/flash/read \
  -H "Content-Type: application/json" \
  -d '{"device_path":"/dev/ttyUSB0","address":65536,"size":1048576}'

# Check status
curl http://localhost:8080/api/v1/flash/read/read-abc123

# Download data
curl http://localhost:8080/api/v1/flash/download/read-abc123 -o flash_dump.bin
```

### Monitor WebSocket

```
WS /api/v1/monitor/{port}?baud=115200&exit_on=pattern
```

Query parameters:
- `baud`: Baud rate (default: 115200)
- `exit_on`: Exit pattern (success)
- `exit_on_error`: Exit pattern (failure)

Messages from server:

Start:
```json
{
  "type": "monitor_start",
  "port": "/dev/ttyUSB0",
  "baud": 115200
}
```

Data (plain text string):
```json
{
  "type": "data",
  "data": "Hello from ESP32!"
}
```

**Web UI:** Access at `/monitor` for browser-based serial monitoring with ANSI color rendering, device selection, and log controls.

Reset complete:
```json
{
  "type": "reset_complete"
}
```

Exit (pattern matched):
```json
{
  "type": "exit",
  "message": "Exit pattern matched"
}
```

Error:
```json
{
  "type": "error",
  "message": "Port not found"
}
```

Messages to server:

Send data to device:
```json
{
  "type": "data",
  "data": "help\n"
}
```

Reset device:
```json
{
  "type": "reset"
}
```

Close connection:
```json
{
  "type": "close"
}
```

### Cluster Status WebSocket

```
WS /api/v1/ws
```

Broadcasts state updates for dashboard clients.

State update:
```json
{
  "type": "state_update",
  "nodes": 3,
  "devices": 6,
  "jobs": 2,
  "queue_size": 1
}
```

### List Cameras

```
GET /api/v1/cameras
```

Response:
```json
{
  "cameras": [
    {
      "id": "61beb4c3-142a-4de5-bfa6-b55f3cb63577",
      "name": "usb-046d_Brio_100_2437APG0Y788-video-index0;video4",
      "path": "/dev/video4",
      "backend": "v4l2",
      "node_id": "themerin",
      "status": "available"
    }
  ],
  "count": 1
}
```

Camera fields:
- `id` - Unique camera identifier (UUID from pion mediadevices)
- `name` - Device label from system
- `path` - Platform-specific device path (e.g. /dev/video4 on Linux)
- `backend` - Platform backend: v4l2, avfoundation, directshow
- `node_id` - Cluster node where camera is attached
- `status` - Camera status: available, busy, offline

### Capture Image

```
POST /api/v1/cameras/capture
Content-Type: application/json
```

Request:
```json
{
  "camera_id": "61beb4c3-142a-4de5-bfa6-b55f3cb63577",
  "width": 1280,
  "height": 720,
  "format": "jpg",
  "quality": 85,
  "preview": false
}
```

Parameters:
- `camera_id` (string, optional): Unique camera identifier (UUID from pion mediadevices)
- `width` (integer, optional): Image width in pixels (default: 1280)
- `height` (integer, optional): Image height in pixels (default: 720)
- `format` (string, optional): Image format - jpg or png (default: jpg)
- `quality` (integer, optional): JPEG quality 1-100 (default: 85)
- `preview` (boolean, optional): If true, returns image without saving to gallery (default: false)

All fields are optional. Defaults: first available camera, 1280x720, jpg, quality 85, save to gallery.

Response:
```json
{
  "status": "success",
  "camera_id": "61beb4c3-142a-4de5-bfa6-b55f3cb63577",
  "path": "/captures/2026-06-05/cam-61beb4c3-20260605-120000.jpg",
  "timestamp": 1716816246
}
```

Filename format: `cam-{camera_id_short}-{timestamp}.{ext}` where `camera_id_short` is the first 12 characters of the camera ID with slashes replaced by dashes (e.g. `/dev/video0` becomes `-dev-video0`).

### List Captures

```
GET /api/v1/captures
```

Response:
```json
{
  "captures": [
    {
      "path": "/captures/2026-06-05/cam-61beb4c3-20260605-120000.jpg",
      "filename": "cam-61beb4c3-20260605-120000.jpg",
      "camera_id": "61beb4c3-142a-4de5-bfa6-b55f3cb63577",
      "camera_name": "usb-046d_Brio_100_2437APG0Y788-video-index0;video4",
      "timestamp": 1716816246,
      "size": 183500
    }
  ],
  "count": 1
}
```

### Get Capture Image

```
GET /captures/{path}
```

Returns the image file with proper Content-Type header.

Path format: `YYYY-MM-DD/cam-{id}-{timestamp}.jpg`

Example: `GET /captures/2026-05-27/cam-abc123-001.jpg`

### Delete Capture

```
DELETE /captures/{path}
```

Deletes a captured image. Path is validated to prevent directory traversal - only files within the captures directory can be deleted, and only image files (.jpg, .jpeg, .png) are allowed.

Response:
```json
{
  "status": "deleted",
  "path": "2026-05-27/cam-abc123-001.jpg"
}
```

Error responses:
- `400 Bad Request` - Invalid or unsafe path
- `404 Not Found` - File doesn't exist

### Get Device Captures

```
GET /api/v1/captures/{captureId}/devices
```

Returns device-specific subimages extracted from a full capture. When device bounding box mappings exist for a camera, the system automatically extracts individual device views from each capture.

Path format: `YYYY-MM-DD/cam-{id}-{timestamp}.jpg`

Example: `GET /api/v1/captures/2026-06-05/cam-abc123-20260605-120000.jpg/devices`

Response:
```json
{
  "capture_id": "2026-06-05/cam-abc123-20260605-120000.jpg",
  "device_captures": [
    {
      "device_id": "esp-aa:bb:cc:dd:ee:ff",
      "bounds": {
        "x": 100,
        "y": 50,
        "width": 320,
        "height": 240
      },
      "subimage_path": "2026-06-05/cam-abc123-20260605-120000/device-esp-aabbccddeeff.jpg",
      "adjustment": {
        "rotation": 0,
        "mirror_x": false,
        "mirror_y": false
      },
      "generated_at": "2026-06-05T12:00:01Z"
    }
  ],
  "count": 1
}
```

Device capture fields:
- `device_id` - Device identifier from mapping
- `bounds` - Bounding box coordinates used for extraction
- `subimage_path` - Relative path to device subimage (serve via /captures/)
- `adjustment` - Image adjustments applied during extraction
- `generated_at` - Timestamp when subimage was created

### Check Flash Hash

```
POST /devices/{deviceId}/flash-hash
```

Checks if the device's current firmware matches the specified firmware file by comparing MD5 hashes of the application region. Use this before flashing to determine if flashing is necessary.

Request body:
```json
{
  "firmware": "build/app.bin",
  "chip": "esp32s3"
}
```

Parameters:
- `firmware` (string, required): Path to firmware file to compare
- `chip` (string, optional): Chip type. Default: esp32s3

Response:
```json
{
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "match": false,
  "device_hash": "abc123...",
  "firmware_hash": "def456...",
  "flash_required": true,
  "status": "checked"
}
```

Parameters:
- `device_id`: Device identifier
- `match`: true if hashes match (no flash needed), false otherwise
- `device_hash`: MD5 hash of device's application region (first 64KB at 0x10000)
- `firmware_hash`: MD5 hash of firmware file's application region
- `flash_required`: true if flashing is needed, false if hashes match
- `status`: Operation status ("checked", "error")

Error responses:
- `400 Bad Request` - Invalid request body or missing firmware path
- `404 Not Found` - Device not found
- `500 Internal Server Error` - Hash computation failed

### Execute Snap

```
POST /devices/snap?device_id={deviceId}
```

Executes a snap operation on the specified device. Snap performs serial monitoring and camera capture. Flashing is handled separately via the hash check and flash endpoints.

Query parameters:
- `device_id` (string, required): Device identifier (path, ID, or alias)

Request body:
```json
{
  "duration": 10,
  "camera_id": "",
  "skip_flash": true,
  "skip_capture": false,
  "skip_monitor": false
}
```

Parameters:
- `duration` (integer, optional): Monitor/capture duration in seconds. Default: 10
- `camera_id` (string, optional): Camera device identifier. Default: auto-select first available
- `skip_flash` (boolean, optional): Skip flashing (should be true for snap-only). Default: true
- `skip_capture` (boolean, optional): Skip camera capture. Default: false
- `skip_monitor` (boolean, optional): Skip serial monitoring. Default: false

Note: The snap endpoint only performs monitor+capture operations. Use the flash hash check endpoint before snap to determine if flashing is needed.

Response:
```json
{
  "snap_id": "snap-20240602-123456",
  "status": "success",
  "metadata": {
    "snap_id": "snap-20240602-123456",
    "timestamp": "2024-06-02T12:34:56Z",
    "duration_ms": 10234,
    "status": "success",
    "device_path": "/dev/ttyUSB0",
    "device_chip": "esp32-s3",
    "flash_enabled": true,
    "flash_firmware": "build/app.bin",
    "flash_offset": 0,
    "flashed": true,
    "flash_skipped": false,
    "flash_hash_before": "abc123...",
    "monitor_enabled": true,
    "monitor_duration": 10,
    "monitor_baud": 115200,
    "log_entry_count": 42,
    "capture_enabled": true,
    "camera_id": "esp32-cam",
    "image_captured": true,
    "image_format": "jpeg",
    "image_size": 45678
  },
  "logs": "Boot complete. Ready.",
  "image": "base64_encoded_jpeg_data..."
}
```

Status values:
- `success`: All operations completed successfully
- `partial`: Some operations failed (e.g., capture failed but monitor succeeded)
- `failed`: Critical operation failed

Error responses:
- `400 Bad Request` - Invalid request body or parameters
- `404 Not Found` - Device not found
- `500 Internal Server Error` - Snap execution failed

### List Device Snaps

```
GET /devices/{deviceId}/snaps
```

Lists all snap operations for a specific device.

Response:
```json
{
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "snaps": [
    {
      "snap_id": "snap-20240602-123456",
      "timestamp": "2024-06-02T12:34:56Z",
      "status": "success",
      "duration_ms": 10234
    }
  ]
}
```

### Get Snap Result

```
GET /snaps/{snapId}
```

Retrieves the result of a specific snap operation.

Response: Same as Execute Snap response.

Error responses:
- `404 Not Found` - Snap ID not found

### Camera Settings

#### List Camera Settings

```
GET /api/v1/camera/settings
```

Lists all camera settings stored in the database.

Response:
```json
{
  "settings": [
    {
      "camera_id": "/dev/video0",
      "name": "Logitech C615",
      "brightness": 128,
      "contrast": 32,
      "saturation": 32,
      "sharpness": 22,
      "gain": 0,
      "focus": 85,
      "exposure": 300,
      "white_balance": 4000,
      "auto_exposure": false,
      "auto_focus": false,
      "auto_white_balance": false,
      "created_at": "2026-06-04T10:00:00Z",
      "updated_at": "2026-06-04T10:00:00Z"
    }
  ],
  "count": 1
}
```

#### Get Camera Settings

```
GET /api/v1/camera/settings/{cameraId}
```

Retrieves settings for a specific camera.

Response:
```json
{
  "settings": {
    "camera_id": "/dev/video0",
    "name": "Logitech C615",
    "brightness": 128,
    "contrast": 32,
    "saturation": 32,
    "sharpness": 22,
    "gain": 0,
    "focus": 85,
    "exposure": 300,
    "white_balance": 4000,
    "auto_exposure": false,
    "auto_focus": false,
    "auto_white_balance": false
  },
  "controls_available": true,
  "platform": "v4l2"
}
```

#### Create Camera Settings

```
POST /api/v1/camera/settings
Content-Type: application/json
```

Request body:
```json
{
  "camera_id": "/dev/video0",
  "name": "Logitech C615",
  "brightness": 128,
  "contrast": 32,
  "saturation": 32,
  "sharpness": 22,
  "gain": 0,
  "focus": 85,
  "exposure": 300,
  "white_balance": 4000,
  "auto_exposure": false,
  "auto_focus": false,
  "auto_white_balance": false
}
```

Parameters:
- `camera_id` (string, required): Unique camera identifier
- `name` (string, optional): Human-readable name for the camera
- `brightness` (integer, optional): Brightness value (0-255)
- `contrast` (integer, optional): Contrast value (0-255)
- `saturation` (integer, optional): Saturation value (0-255)
- `sharpness` (integer, optional): Sharpness value (0-255)
- `gain` (integer, optional): Gain value (0-255)
- `focus` (integer, optional): Focus distance (0-255)
- `exposure` (integer, optional): Manual exposure value
- `white_balance` (integer, optional): White balance temperature
- `auto_exposure` (boolean, optional): Enable auto exposure
- `auto_focus` (boolean, optional): Enable auto focus
- `auto_white_balance` (boolean, optional): Enable auto white balance

Response:
```json
{
  "status": "created",
  "camera_id": "/dev/video0",
  "settings": {...}
}
```

#### Update Camera Settings

```
PUT /api/v1/camera/settings/{cameraId}
PATCH /api/v1/camera/settings/{cameraId}
```

Updates camera settings. PUT replaces all settings, PATCH updates only provided fields.

Request body: Same as Create Camera Settings.

Response:
```json
{
  "status": "updated",
  "camera_id": "/dev/video0",
  "settings": {...}
}
```

#### Delete Camera Settings

```
DELETE /api/v1/camera/settings/{cameraId}
```

Removes camera settings for the specified camera.

Response:
```json
{
  "status": "deleted",
  "camera_id": "/dev/video0"
}
```

#### Apply Camera Settings

```
POST /api/v1/camera/settings/{cameraId}/apply
```

Applies stored settings to the physical camera device. Only available on Linux with V4L2 support.

If no settings exist for the camera, default settings are automatically created with:
- brightness: 128, contrast: 128, saturation: 128, sharpness: 128
- gain: 0, focus: 0, exposure: 0, white_balance: 0
- auto_exposure: true, auto_focus: true, auto_white_balance: true

Response:
```json
{
  "status": "applied",
  "camera_id": "/dev/video0",
  "settings": {...},
  "current": {
    "brightness": 128,
    "contrast": 32,
    "focus": 85
  },
  "platform": "v4l2"
}
```

If camera controls are not available on the platform:
```json
{
  "status": "skipped",
  "message": "Camera controls not available on this platform",
  "platform": "darwin"
}
```

#### Discover Cameras

```
GET /api/v1/camera/discover
```

Lists available cameras on the system.

Response:
```json
{
  "cameras": [
    {
      "id": "/dev/video0",
      "name": "Logitech C615",
      "backend": "v4l2",
      "formats": [
        {"width": 1280, "height": 720, "pixel_format": "MJPG"}
      ]
    }
  ],
  "count": 1,
  "controls_available": true,
  "platform": "v4l2"
}
```

#### Get Camera Controls

```
GET /api/v1/camera/{cameraId}/controls
```

Queries available controls and current settings for a camera.

Response:
```json
{
  "current": {
    "brightness": 128,
    "contrast": 32,
    "saturation": 32,
    "sharpness": 22
  },
  "available": true,
  "platform": "v4l2",
  "display_preset": {
    "brightness": 80,
    "contrast": 140,
    "sharpness": 150,
    "saturation": 90,
    "exposure": 300
  },
  "focus_presets": {
    "close": 200,
    "display": 85,
    "far": 30
  }
}
```

## Wokwi Simulator Integration

ESPBrew supports running firmware on Wokwi simulator as an alternative to physical hardware. This is useful for testing and development without requiring actual devices.

### Device Paths

Virtual Wokwi devices use URI-style paths:
- `wokwi:esp32-s3` - ESP32-S3 simulator
- `wokwi:esp32-c3` - ESP32-C3 simulator

### Backend Configuration

Wokwi devices can be configured in two modes:

#### CLI Mode (default)
Uses `wokwi-cli` subprocess for simulation:
```json
{
  "device_id": "wokwi:esp32-s3",
  "backend": "wokwi",
  "backend_config": {
    "chip_type": "ESP32-S3",
    "diagram_json": "{...}"
  }
}
```

#### API Mode
Uses Wokwi WebSocket API for better performance and features:
```json
{
  "device_id": "wokwi:esp32-s3",
  "backend": "wokwi",
  "backend_config": {
    "chip_type": "ESP32-S3",
    "diagram_json": "{...}",
    "api_token": "your-wokwi-api-token"
  }
}
```

Get your API token from https://wokwi.com/dashboard/ci

### API Mode Benefits

- Faster startup (no subprocess spawning)
- Real-time serial output via WebSocket events
- Structured error responses
- Future support for GPIO, display capture, logic analyzer

### Default Virtual Devices

ESPBrew automatically creates these Wokwi devices on startup:
- `wokwi:esp32-s3` - ESP32-S3 with default board
- `wokwi:esp32-c3` - ESP32-C3 with default board

## Error Responses

All error responses follow this format:

```json
{
  "error": "Error message here"
}
```

Common HTTP status codes:
- `400 Bad Request` - Invalid parameters
- `404 Not Found` - Resource doesn't exist
- `409 Conflict` - Device/job unavailable
- `500 Internal Server Error` - Server error
- `501 Not Implemented` - Feature not available
