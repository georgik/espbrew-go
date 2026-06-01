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
    "state": "available",
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
      "id": "cam-abc123",
      "name": "FaceTime HD Camera",
      "backend": "darwin",
      "node_id": "node-1",
      "status": "available"
    }
  ],
  "count": 1
}
```

### Capture Image

```
POST /api/v1/cameras/capture
Content-Type: application/json
```

Request:
```json
{
  "camera_id": "cam-abc123",
  "width": 1280,
  "height": 720,
  "format": "jpg",
  "quality": 85
}
```

All fields are optional. Defaults: first available camera, 1280x720, jpg, quality 85.

Response:
```json
{
  "status": "success",
  "camera_id": "cam-abc123",
  "path": "/captures/2026-05-27/cam-abc123-20260527-123456.jpg",
  "timestamp": 1716816246
}
```

### List Captures

```
GET /api/v1/captures
```

Response:
```json
{
  "captures": [
    {
      "path": "/captures/2026-05-27/cam-abc123-001.jpg",
      "filename": "cam-abc123-001.jpg",
      "camera_id": "cam-abc123",
      "camera_name": "FaceTime HD Camera",
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
