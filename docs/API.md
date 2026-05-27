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

Data (base64 encoded):
```json
{
  "type": "data",
  "data": "SGVsbG8gZnJvbSBFU1AzMiE="
}
```

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
