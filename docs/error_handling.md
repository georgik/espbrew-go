# Error Handling and Recovery

## Overview

ESPBrew Cluster includes comprehensive error handling and recovery mechanisms to ensure reliable operation even in the face of network issues, device failures, or unexpected conditions.

## Job Timeout

Jobs that exceed the time limit are automatically marked as timed out and resources are released.

### Configuration

Default timeout: 10 minutes

To change the timeout, modify the `DefaultJobTimeout` constant in `internal/cluster/queue.go`:

```go
const DefaultJobTimeout = 10 * time.Minute
```

### Behavior

When a job times out:
1. Job status changes to `timeout`
2. Device reservation is released
3. Device state returns to `available`
4. Error message: "Job timed out"

## Job Cancellation

Jobs can be cancelled before or during execution.

### Via API

```bash
curl -X DELETE http://localhost:8080/api/v1/jobs/{job_id}
```

Response:
```json
{
  "status": "cancelled",
  "job_id": "job-abc123"
}
```

### Via CLI

```bash
# Cancel a specific job (future feature)
./espbrew job cancel {job_id}
```

### Cancellation Rules

- Jobs in state `pending` or `assigned` can be cancelled
- Jobs in state `running` will be marked but executor must handle cleanup
- Jobs in terminal states (`completed`, `failed`, `cancelled`, `timeout`) cannot be cancelled
- Device is released when job is cancelled

## Connection Retry

The cluster client automatically retries failed requests to handle transient network issues.

### Retry Configuration

Default behavior:
- Maximum retries: 3
- Retry delay: 1 second (exponential backoff)
- Retries on: HTTP 500+, HTTP 429 (Too Many Requests)

To customize:

```go
client := cluster.NewClient("http://leader:8080")
client.SetRetryPolicy(5, 2*time.Second) // 5 retries, 2 second delay
client.SetTimeout(30 * time.Second)     // 30 second timeout
```

### Retryable Errors

- Network connection failures
- HTTP 500 (Internal Server Error)
- HTTP 502 (Bad Gateway)
- HTTP 503 (Service Unavailable)
- HTTP 504 (Gateway Timeout)
- HTTP 429 (Too Many Requests)

### Non-Retryable Errors

- HTTP 400 (Bad Request) - Invalid parameters
- HTTP 404 (Not Found) - Resource doesn't exist
- HTTP 409 (Conflict) - Device/job unavailable

## Stale Reservation Cleanup

Device reservations that exceed the time limit are automatically cleaned up.

### Configuration

Default stale timeout: 30 minutes

Cleanup runs:
- On startup (after 10 seconds)
- Every 60 seconds thereafter

### Behavior

When a stale reservation is cleaned:
1. Device state changes to `available`
2. Reservation owner is cleared
3. Log entry records the previous owner

## Old Job Cleanup

Completed jobs older than the threshold are automatically removed from memory.

### Configuration

Default job TTL: 24 hours

### Behavior

Jobs in any terminal state are cleaned:
- `completed`
- `failed`
- `cancelled`
- `timeout`

## Unprobed Devices and Fallback IDs

When a device is detected but automatic probe fails (e.g., due to USB issues, non-ESP device, or unresponsive device), the system creates a fallback device ID to allow manual configuration.

### Detection

Devices that fail probe are identified by:
- Device ID starting with `unprobed-` prefix (e.g., `unprobed-10c4:ea60`)
- Chip type set to `Unknown`
- Warning icon displayed in the web UI

### Fallback ID Format

```
unprobed-<VID>:<PID>
```

Example: `unprobed-10c4:ea60`

The VID and PID are extracted from the USB device descriptor.

### User Actions

For unprobed devices, the web UI provides:
- **Edit button**: Configure device manually
- **Warning button**: Shows probe failure message

To recover an unprobed device:
1. Click Edit on the device
2. Select appropriate Chip Type
3. Save to update device configuration

### Persistence

Fallback devices are persisted across restarts, allowing:
- Device identification after reconnection
- Manual configuration at any time
- Tracking of problematic devices

### Common Causes

1. **USB hardware issues**: Flaky connection, damaged cable
2. **Device not responding**: Device stuck in bootloader, powered off
3. **Non-ESP device**: USB serial adapter without ESP connected
4. **Permission denied**: User lacks device access permissions
5. **Driver issues**: Missing or incorrect USB drivers

### Logs

```
WARN Device probe failed, created fallback ID for manual configuration path=/dev/ttyUSB0 fallback_id=unprobed-10c4:ea60
```

## Orphaned Device Recovery

Devices marked as `busy` without an active job are automatically recovered.

### Detection

During maintenance runs, the system:
1. Lists all active jobs
2. Finds devices marked `busy`
3. Releases devices without corresponding active jobs

### Recovery

```go
// Automatic in maintenance loop
func (m *MasterNode) cleanupOrphanedDevices()
```

## WebSocket Reconnection

WebSocket connections for progress monitoring and serial monitoring should be re-established on disconnect.

### Client-Side Reconnection

```go
for attempt := 0; attempt <= maxRetries; attempt++ {
    conn, err := websocket.Dial(url)
    if err == nil {
        break
    }
    time.Sleep(retryDelay * time.Duration(attempt))
}
```

### Graceful Disconnect

Normal WebSocket closure codes:
- `1000` (Normal Closure)
- `1001` (Going Away)

These should not trigger reconnection.

## Error Responses

All API errors follow a consistent format:

```json
{
  "error": "Descriptive error message"
}
```

### HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| 400  | Bad Request | Invalid parameters |
| 404  | Not Found | Job or device doesn't exist |
| 409  | Conflict | Device already reserved |
| 500  | Internal Error | Server-side failure |
| 501  | Not Implemented | Feature unavailable |
| 503  | Service Unavailable | Temporarily overloaded (retryable) |

## Client Error Handling

### Example: Robust Flash Submission

```go
client := cluster.NewClient("http://leader:8080")

// Configure retry policy for unreliable networks
client.SetRetryPolicy(5, 2*time.Second)
client.SetTimeout(30 * time.Second)

// Upload with retry
uploadResp, err := client.UploadFirmware("firmware.bin")
if err != nil {
    log.Fatal().Err(err).Msg("Upload failed after retries")
}

// Submit job
flashResp, err := client.SubmitFlash(cluster.FlashSubmitRequest{
    DevicePath: "/dev/ttyUSB0",
    FileID:     uploadResp.FileID,
})
if err != nil {
    if strings.Contains(err.Error(), "device not available") {
        // Handle device unavailable
        log.Info().Msg("No devices available, waiting...")
        time.Sleep(5 * time.Second)
    } else {
        log.Fatal().Err(err).Msg("Flash submission failed")
    }
}
```

## Logging

All error conditions are logged with appropriate levels:

- `Debug`: Retries, state transitions
- `Info`: Job completion, device state changes
- `Warn`: Stale reservations, orphaned devices
- `Error`: Job failures, timeout, critical errors

Example log entries:

```
WARN Cleaned up stale device reservation path=/dev/ttyUSB0 prev_owner=client-123
WARN Releasing orphaned busy device path=/dev/ttyUSB1
ERROR Job timed out job_id=abc-123
INFO Job completed successfully job_id=def-456
```

## Monitoring Error States

### Dashboard Indicators

The web dashboard shows:
- Failed jobs (red)
- Timed out jobs (yellow)
- Cancelled jobs (gray)

### Health Check

```bash
curl http://localhost:8080/health
```

Returns cluster health status for monitoring systems.

## Best Practices

1. **Always handle errors** from API calls
2. **Use retry logic** for network operations
3. **Set appropriate timeouts** for your use case
4. **Monitor logs** for error patterns
5. **Clean up resources** by releasing reservations
6. **Check device state** before operations
7. **Implement graceful shutdown** in long-running processes
