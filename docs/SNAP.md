# ESPBrew Snap - Flash, Monitor, Capture

## Overview

The `snap` command combines three essential operations into a single streamlined workflow:

1. **Flash** firmware (optional, with hash-based optimization)
2. **Monitor** serial output for boot verification
3. **Capture** camera image for visual verification

This command is designed for iterative development, CI/CD pipelines, and automated testing where you need quick feedback on firmware changes.

### Key Benefits

- **Single Command**: One command replaces manual flash + monitor + capture workflow
- **Hash Optimization**: Automatically skips flashing unchanged firmware regions
- **Auto-Detection**: Finds devices, firmware, and project context automatically
- **Cluster Support**: Works with local and remote cluster deployments
- **Flexible Output**: Console, JSON, or file-based results

## Command Reference

### Syntax

```bash
espbrew snap [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--cluster` | string | (empty) | Cluster URL for remote snap operations |
| `--device` | string | (empty) | Device selection by ID, alias, or MAC from inventory |
| `-p, --port` | string | (auto) | Serial port (auto-detects ESP device if empty) |
| `-f, --firmware` | string | (auto) | Firmware .bin file to flash before capture |
| `--duration` | int | 10 | Capture duration in seconds |
| `--baud-rate` | int | 115200 | Serial baud rate |
| `--camera` | string | (auto) | Camera device identifier (empty for auto-select first available) |
| `-o, --output` | string | (empty) | Output file path for captured image |
| `--force-flash` | bool | false | Force flash even if firmware hash matches |
| `--skip-flash` | bool | false | Skip flashing step entirely |
| `--no-capture` | bool | false | Skip image capture (flash + monitor only) |
| `--no-monitor` | bool | false | Skip serial monitor after flash |
| `--save-dir` | string | (empty) | Directory to save captured images |
| `--leader` | string | (empty) | Leader address for cluster mode (deprecated: use --cluster) |
| `--job-id` | string | (empty) | Job ID for resuming operations |

### Device Selection

The `--device` flag accepts multiple identifier formats:

- **Device ID**: `esp-aa:bb:cc:dd:ee:ff` (MAC-based)
- **MAC Address**: `aa:bb:cc:dd:ee:ff`
- **Alias**: Any custom alias assigned in the inventory
- **Path**: `/dev/ttyUSB0` (direct port, use `--port` instead)

## Usage Examples

### Local Mode

#### Basic snap with auto-detection

```bash
# Auto-detects device and firmware, monitors for 10 seconds (default)
espbrew snap
```

#### Specify device and firmware

```bash
espbrew snap --device /dev/ttyUSB0 --firmware build/app.bin
```

#### Use device from inventory

```bash
espbrew snap --device esp-aa:bb:cc:dd:ee:ff --firmware build/app.bin
```

#### Quick verification (5 seconds)

```bash
espbrew snap --duration 5
```

#### Flash and monitor only (no camera capture)

```bash
espbrew snap --no-capture --duration 10
```

#### Monitor existing firmware (skip flash, no camera)

```bash
espbrew snap --skip-flash --no-capture --duration 15
```

#### Force flash even if hashes match

```bash
espbrew snap --force-flash
```

#### Save captured image to specific directory

```bash
espbrew snap --save-dir ./snapshots
```

#### Custom baud rate

```bash
espbrew snap --baud-rate 921600
```

### Cluster Mode

#### Snap via cluster leader

```bash
espbrew snap --cluster http://leader:8080 --device esp-aa:bb:cc:dd:ee:ff
```

#### Snap with firmware upload

```bash
espbrew snap --cluster http://leader:8080 \
            --device esp-aa:bb:cc:dd:ee:ff \
            --firmware build/app.bin \
            --duration 20
```

#### Skip flash in cluster mode

```bash
espbrew snap --cluster http://leader:8080 \
            --device esp-aa:bb:cc:dd:ee:ff \
            --skip-flash
```

## Output Format

### Console Output

```bash
$ espbrew snap
INFO Auto-detected ESP device: /dev/ttyUSB0
INFO Auto-detected firmware: build/app.bin
INFO Flashing firmware to /dev/ttyUSB0
[========================================] 100% Flashing complete
INFO Starting monitor for capture (duration: 10s)
=== Serial Output ===
Boot complete. Ready.
```

### Output Formats

The snap command supports multiple output formats via the `--output` flag:

#### Text Format (Default)

Human-readable output with sections for device, flash, monitor, and capture information:

```bash
$ espbrew snap
INFO Auto-detected ESP device: /dev/ttyUSB0
INFO Auto-detected firmware: build/app.bin
INFO Flashing firmware to /dev/ttyUSB0
[========================================] 100% Flashing complete
INFO Starting monitor for capture (duration: 10s)

--- Snap Result ---
Snap ID: snap-20240602-123456
Status: success
Device: /dev/ttyUSB0 (esp32-s3)
Flashed: yes (hash_mismatch)
Duration: 28453ms
Logs: 42 entries
Image: captured (45678 bytes)

=== Serial Output ===
Boot complete. Ready.
```

#### JSON Format

Machine-readable JSON output for programmatic parsing:

```bash
espbrew snap --output json
espbrew snap --output result.json
```

JSON structure:
```json
{
  "snap_id": "snap-20240602-123456",
  "timestamp": "2024-06-02T12:34:56Z",
  "duration_ms": 28453,
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
  "monitor_duration": 30,
  "monitor_baud": 115200,
  "log_entry_count": 42,
  "capture_enabled": true,
  "camera_id": "esp32-cam",
  "image_captured": true,
  "image_format": "jpeg",
  "image_size": 45678,
  "logs": "Boot complete. Ready.",
  "image": "base64_encoded_jpeg_data..."
}
```

#### Compact Format

Single-line summary for quick status checks:

```bash
$ espbrew snap --output compact
snap-20240602-123456 success 28453ms /dev/ttyUSB0 esp32-s3 build/app.bin monitor:42 capture:45678
```

### File Saving

Use `--save-dir` to save captured images and metadata:

```bash
espbrew snap --save-dir ./snapshots
```

Creates:
- `./snapshots/snap-{id}.jpg` - Captured image
- `./snapshots/snap-{id}.json` - Result metadata

Example output:
```
Saved image: ./snapshots/snap-20240602-123456.jpg
Saved metadata: ./snapshots/snap-20240602-123456.json
```

## Integration with FlashHash

The snap command integrates with the hash-based flash detection system for optimized flashing during iterative development.

### How It Works

In cluster mode, the snap workflow is separated into three distinct steps:

1. **Hash Check**: Query cluster to compare firmware hash with device flash
2. **Flash (if needed)**: Only flash when hashes differ or `--force-flash` is set
3. **Snap**: Monitor serial output and capture image (no flashing)

This separation prevents the snap operation from attempting to flash after a previous flash completed, avoiding "port busy" errors.

### Cluster Workflow

```
espbrew snap --cluster http://leader:8080 --device esp-id --firmware app.bin

Step 1: POST /api/v1/devices/{id}/flash-hash
        → Check if device needs flashing

Step 2: POST /api/v1/flash (if hash mismatch or force-flash)
        → Upload firmware and submit flash job
        → Stream progress until complete

Step 3: POST /api/v1/devices/snap?device_id={id}
        → Monitor serial output
        → Capture camera image
        → Returns snap result
```

### Local Workflow

For local operations (without `--cluster`), the snap command performs all operations sequentially on the local machine.

### Performance Benefits

For iterative development with unchanged bootloader/partitions:

- **Full flash + monitor + capture**: ~25s
- **Hash skip + monitor + capture**: ~12s
- **Monitor + capture only** (`--skip-flash`): ~10s
- **Flash only** (`--no-capture`): ~22s

The hash check adds ~100-200ms in cluster mode but saves ~15s when firmware is unchanged.

### Manual Control

Use `--force-flash` to bypass hash optimization and force a full flash:

```bash
espbrew snap --force-flash
```

Use `--skip-flash` to skip flashing entirely:

```bash
espbrew snap --skip-flash
```

## Performance Notes

### Duration Guidelines

| Duration | Use Case |
|----------|----------|
| 5s | Quick boot check, minimal output |
| 10s | Default, standard boot + app startup |
| 20s | Full initialization, network connection |
| 30s+ | Extended monitoring, debugging |

### Baud Rate Impact

Higher baud rates enable faster serial monitoring:

| Baud Rate | Use Case |
|-----------|----------|
| 115200 | Default, reliable |
| 230400 | Fast boot logs |
| 460800 | High-speed logging |
| 921600 | Maximum speed |

### Cluster Overhead

Cluster mode adds network latency:

- Firmware upload: +1-2s per MB
- Hash query: +100-200ms
- Progress streaming: +50-100ms

Local mode is faster for single-device operations.

## Troubleshooting

### Device Not Found

**Error**: `--port required or no ESP devices found`

**Solutions**:
- Connect device and verify with `espbrew devices`
- Specify port explicitly: `--port /dev/ttyUSB0`
- Check device permissions (add user to `dialout` group on Linux)

### Firmware Not Found

**Error**: `please specify --firmware or --skip-flash`

**Solutions**:
- Run from project build directory for auto-detection
- Specify firmware: `--firmware build/app.bin`
- Skip flash: `--skip-flash`

### Flash Failure

**Error**: `flash failed: ...`

**Solutions**:
- Verify device is in download mode (auto-reset on most boards)
- Try manual reset: hold BOOT button, press RESET, release both
- Check baud rate: `--baud-rate 115200` for reliable flash
- Force re-flash: `--force-flash`

### No Serial Output

**Symptom**: Monitor runs but no logs appear

**Solutions**:
- Verify baud rate matches firmware configuration
- Increase duration: `--duration 60`
- Check device is actually booting (LED activity)
- Use `espbrew monitor` directly for testing

### Camera Not Found

**Error**: `camera not found`

**Solutions**:
- List cameras: `espbrew capture --list`
- Skip camera: `--no-capture`
- Check camera permissions on macOS/Linux

**Note**: USB cameras often appear as multiple devices (index0, index1, etc.). The camera list is filtered to show only the primary interface (index0) for each camera, sorted alphabetically by name. Use `espbrew capture --list` to see available cameras.

### Cluster Connection Failed

**Error**: `cluster connection failed`

**Solutions**:
- Verify cluster URL: `http://leader:8080`
- Check cluster status: `curl http://leader:8080/api/v1/status`
- Ensure device is registered in inventory
- Use device ID instead of path: `--device esp-aa:bb:cc:dd:ee:ff`

### Hash Query Failed

**Warning**: `hash query failed, falling back to full flash`

**Solutions**:
- Network error or cluster unavailable
- Device not in inventory
- Hash computation failed (continues with full flash)
- Use `--force-flash` to avoid query

### Permission Denied

**Error**: `permission denied: /dev/ttyUSB0`

**Solutions**:
- Linux: `sudo usermod -a -G dialout $USER` (then log out/in)
- Or use: `sudo espbrew snap` (not recommended for regular use)

## Workflow Examples

### Iterative Development

```bash
# Initial flash and verify
espbrew snap

# Make code changes
# ...

# Rebuild only (app binary changed)
idf.py build

# Snap skips bootloader flash, only flashes app
espbrew snap  # ~10s instead of ~30s
```

### CI/CD Pipeline

```bash
#!/bin/bash
set -e

# Build firmware
idf.py build

# Verify device boots correctly
espbrew snap --duration 10 --no-capture

# Check logs for success pattern
# (Pipeline continues if snap succeeded)
```

### Bulk Device Testing

```bash
# Test all devices in inventory
for device in $(espbrew inventory list --json | jq -r '.[].device_id'); do
    espbrew snap --cluster http://leader:8080 --device "$device" --duration 15
done
```

### Regression Testing

```bash
# Capture baseline
espbrew snap --save-dir ./baseline

# Make changes
# ...

# Compare with baseline
espbrew snap --save-dir ./test

# (Manual comparison or automated image diff)
```

## Cluster Snap API

When using cluster mode, the snap command communicates with the cluster leader via HTTP API. The leader routes snap requests to the appropriate peer node that owns the target device.

### API Endpoints

#### Execute Snap

**Endpoint:** `POST /api/v1/devices/snap?device_id={deviceId}`

**Request:**
```json
{
  "duration": 10,
  "camera_id": "",
  "skip_flash": true,
  "skip_capture": false,
  "skip_monitor": false
}
```

**Response:**
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
    "flashed": true,
    "log_entry_count": 42,
    "image_captured": true,
    "image_size": 45678
  },
  "logs": "Boot complete. Ready.",
  "image": "base64_encoded_jpeg_data..."
}
```

#### List Device Snaps

**Endpoint:** `GET /api/v1/devices/{deviceId}/snaps`

**Response:**
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

#### Get Snap Result

**Endpoint:** `GET /api/v1/snaps/{snapId}`

**Response:** Same as Execute Snap response

#### Check Flash Hash

**Endpoint:** `POST /api/v1/devices/{deviceId}/flash-hash`

**Request:**
```json
{
  "firmware": "build/app.bin",
  "chip": "esp32s3"
}
```

**Response:**
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

### Error Responses

**Device not found (404):**
```json
{
  "error": "device not found: esp-aa:bb:cc:dd:ee:ff"
}
```

**Snap execution failed (500):**
```json
{
  "snap_id": "snap-20240602-123456",
  "status": "failed",
  "error": "flash operation timed out"
}
```

## See Also

- [Hash-Based Flash Detection](HASH_BASED_FLASH.md) - Flash optimization details
- [Cluster Usage](CLUSTER.md) - Multi-node setup
- [HTTP API Reference](API.md) - REST and WebSocket endpoints
- [Image Mapping](IMAGE_MAPPING.md) - Device mapping and screenshot extraction
