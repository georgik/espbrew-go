# Flashing Implementation

This document describes the internal implementation of the flashing system, including fixes for common issues and platform-specific considerations.

## ESP32-S3 USB-JTAG/Serial Support

ESP32-S3, ESP32-C3, ESP32-C5, ESP32-C6, and ESP32-H2 chips support USB-JTAG/Serial mode, which presents itself as a USB CDC device (/dev/ttyACM* on Linux, /dev/cu.usb* on macOS). This requires different reset handling than classic DTR/RTS reset.

### USB CDC Port Detection

The system automatically detects USB CDC ports:

```
/dev/ttyACM*   # Linux USB CDC
/dev/cu.usb*    # macOS USB CDC
```

### Reset Modes

The flashing system supports multiple reset modes:

- **ResetDefault**: Classic DTR/RTS reset sequence
- **ResetUSBJTAG**: USB-JTAG reset only
- **ResetAuto**: Automatic detection (default)

### ResetAuto Behavior

For USB CDC ports, ResetAuto immediately uses USB-JTAG reset, skipping classic reset attempts that would fail. For non-USB CDC ports, it tries multiple strategies in sequence:

1. Classic reset (DTR/RTS)
2. USB-JTAG reset (if available)
3. No reset (device may already be in bootloader)

### Fast Mode Optimization

Fast mode reduces connection attempts and delays for rapid development iteration. For USB CDC ports, FastMode is automatically disabled because these devices re-enumerate after reset, requiring port reopen.

## Fixed Issues

### Context Cancellation Bug

**Problem**: In espflash.New(), a defer cancel() cancelled the child context immediately after the function returned, causing "context canceled" errors in subsequent operations.

**Solution**: Store the cancel function in the Flasher struct and call it in Close() instead of using defer.

**Impact**: Multi-part ELF flashing now works correctly. FlashImages() and subsequent attachFlash() calls no longer fail with context errors.

### USB CDC Port Re-enumeration

**Problem**: USB CDC devices re-enumerate after reset, making the old port handle stale. FastMode was skipping port reopen, causing sync failures.

**Solution**: Added isUSBPort() detection and automatically disable FastMode for USB CDC ports.

**Impact**: Flashing now works reliably for ESP32-S3/C3/C5/C6/H2 USB-JTAG/Serial devices.

### Reset Sequence for USB-JTAG Devices

**Problem**: ResetAuto was trying classic reset first for USB-JTAG devices, which doesn't work. This wasted attempts and caused timeouts.

**Solution**: For USB CDC ports, use USB-JTAG reset immediately in ResetAuto mode.

**Impact**: Connection is faster and more reliable for USB-JTAG/Serial devices.

## Multi-Part Flashing

When flashing ELF files, the system converts to ESP-IDF format and flashes multiple parts:

1. Bootloader (chip-specific offset, typically 0x0 or 0x1000)
2. Partition table (0x8000)
3. Application (0x10000)

Each part is flashed separately. The device must remain in bootloader mode between parts.

### Connection Persistence

The flasher maintains a single connection throughout the multi-part flash operation. The context lives for the entire flasher lifetime, not just during initial connection.

## Port Reopen Strategy

After a reset, USB CDC devices may briefly disappear. The reopenPort() function:

1. Closes the old port
2. Waits up to 3 seconds for re-enumeration
3. Reopens with the same baud rate settings
4. Creates a new connection object

For USB-JTAG reset, reopenPort() is always called because the reset causes USB disconnection.

## Connection Attempts

Default connection attempts vary by mode:

- Fast mode: 2 attempts
- Normal mode: 7 attempts
- ResetAuto mode: 9 attempts (allows for USB-JTAG retry every 3rd attempt)

## Flashing Flow

1. **Open serial port** - Open with configured baud rate
2. **Reset into bootloader** - Use appropriate reset method for device
3. **Sync with bootloader** - Send sync packets, wait for acknowledgment
4. **Detect chip** - Read chip identification
5. **Load stub loader** - Upload stub for advanced features
6. **Attach flash** - Configure SPI flash parameters
7. **Flash data** - Write firmware with progress tracking
8. **Verify** - MD5 verification (if stub loaded)
9. **Reset device** - Exit bootloader, run user code

## Error Recovery

### Sync Failures

When sync fails, the system:

1. Flushes input buffer
2. Retries with delay
3. Reopens port (if not in FastMode)
4. Tries next reset attempt

### Context Cancellation

All operations check for context cancellation and exit cleanly:

- Before reset operations
- Before sync attempts
- During data transfer loops

### Timeouts

- Connect attempts: Default 9, configurable via ConnectAttempts
- Port reopen: 3 seconds
- Individual operations: Use parent context timeout

## Testing

Flash functionality is tested via:

- Unit tests for port detection (isUSBPort)
- Integration tests for ELF conversion
- Virtual device tests (no hardware required)
- Hardware tests (require ESP ELF, skipped gracefully)

Run tests:

```bash
go test ./internal/flash/...
go test ./internal/espflash/...
```

## Stub Loaders

ESP stub loaders enable advanced flashing features:

- MD5 verification
- Compressed flashing
- Region erase
- Flash read operations

**Source:** git submodule from esp-rs/espflash repository

**Update procedure:**
```bash
# Update submodule to desired version
cd vendor/espflash && git checkout v3.1.0 && cd ../..

# Copy stubs to local directory
go run tools/update-stubs.go
```

**Format:** TOML files with base64-encoded binary data

**Embed:** Stubs are embedded in binary via go:embed, no runtime dependency on submodule
