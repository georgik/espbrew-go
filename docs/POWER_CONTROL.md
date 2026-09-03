# USB Hub Power Control Implementation

This document describes the internal implementation of ESPBrew's native USB hub power control feature on Linux.

## Architecture Overview

ESPBrew implements USB hub power control using two mechanisms:

1. **USBDEVFS ioctl** (primary) - Direct communication with USB device files via kernel ioctls
2. **sysfs write interface** (fallback) - Writing to `/sys/bus/usb/devices/*/port*/disable` files (kernel >= 6.0)

The implementation uses no external dependencies (no libusb, no cgo bindings), relying entirely on the Go standard library and Linux kernel interfaces.

## Hub Discovery

### listHubsLinux()

Discovers all USB hubs with per-port power switching capability:

1. Enumerates entries in `/sys/bus/usb/devices/`
2. Filters out interface directories (contain colon `:`)
3. Parses each entry as a potential hub using sysfs attributes:
   - `idVendor`, `idProduct` - Device identification
   - `bDeviceClass` - Must equal `0x09` (USB Hub class)
4. Identifies the hub interface and counts ports by examining port directories
5. **Filters out parent hubs** that have downstream sub-hubs on their ports

### Leaf Hub Filtering

The key innovation is filtering out "parent" hubs during discovery. In complex USB topologies:

```
Root Hub (1-5) -> Port 3 -> Sub-Hub (1-5.3) -> Port 3 -> ESP32 Device
```

Without filtering, espbrew would discover both `1-5` and `1-5.3`. Controlling the parent hub (`1-5`) doesn't work reliably because:
- The device connects through the sub-hub chain
- Power control on parent hubs may not propagate correctly to downstream devices

The filter uses `hasDownstreamHub()` which:
1. Reads entries in the hub's sysfs directory
2. Skips interface directories (contain colon)
3. Checks if any entry has a dot in its name (sub-hub format like `X-Y.Z`)
4. Verifies it's actually a hub by checking `bDeviceClass == 0x09`

Only "leaf" hubs (hubs with no downstream hub devices) are included for power control.

## Dual-Interface Hub Coordination

### linkDualHubs()

Modern USB 3.x hubs present as two separate hub devices:
- USB 2.0 hub - handles Full/High Speed traffic
- USB 3.x hub - handles SuperSpeed traffic

Both share physical ports but have independent power domains. For complete device disconnect, both must be powered simultaneously.

The linking algorithm finds counterpart hubs by:
1. Matching same vendor ID
2. Different SuperSpeed state (one USB 2.0, one USB 3.x)
3. Same hierarchy level (same number of dots in location)
4. Verifying they are dual-interface partners via sysfs relationships or known product ID pairs

### SetPortPowerDual()

Coordinates power operations across dual-interface hubs:

1. Links the hub with its counterpart if not already linked
2. Determines primary and secondary hubs (powers USB 2.0 first for compatibility)
3. Powers primary hub port
4. Waits 100ms to avoid race conditions
5. Powers secondary hub port
6. Rolls back primary if secondary fails

## Power Control Implementation

### setPortPowerIoctl() (Primary Path)

Uses USBDEVFS ioctl via `/dev/bus/usb/<bus>/<device>`:

1. Opens the hub device file by resolving location to bus/device numbers
2. Sends control transfer via `USBDEVFS_CONTROL` ioctl:
   - **Power ON**: SET_FEATURE request with `USB_PORT_FEAT_POWER` (value 8)
   - **Power OFF**: CLEAR_FEATURE request with `USB_PORT_FEAT_POWER`
3. For USB 3.0 hubs, waits 100ms after power off for link teardown
4. Verifies the power state actually changed via readback

### setPortPowerLinux() (Fallback Path)

Writes to sysfs `disable` file:
- `"1"` = disable port (power off)
- `"0"` = enable port (power on)

This path is available on Linux kernel >= 6.0 but is not wired up by default in the current implementation.

## Port Status Decoding

### getPortStatusIoctl()

Reads port status via USB GET_STATUS class request:

**USB 2.0 ports:**
- `wPortStatus` bits decoded for power, connection, enable, speed
- Speed: low (0x0200), high (0x0400), full (connected but no speed bit)

**USB 3.0 ports:**
- Link state decoded from `wPortStatus >> 5`
- Speed: U0/U1/U2/U3, Rx.Detect, compliance mode, etc.
- Power bit at different offset (0x0200 vs 0x0100 for USB 2.0)

## Platform Integration

### Linux (`sysfs_linux.go`)

Full implementation with:
- Hub discovery via sysfs enumeration
- Dual-hub linking and coordination
- USBDEVFS ioctl power control
- Port status reading via ioctl

### Non-Linux (`stub_other.go`)

Returns `ErrNotSupported` for all operations. Compiled with `//go:build !linux`.

## Error Handling

The implementation defines specific error types:
- `ErrHubNotFound` - Hub not found by location or vendor:product
- `ErrPortNotFound` - Port number out of range
- `ErrNotSupported` - Platform doesn't support power control
- `ErrPermissionDenied` - Insufficient permissions for USB device access
- `ErrKernelTooOld` - Kernel version < 6.0 (for sysfs path)

## Testing and Verification

### Expected Behavior

1. **Auto-detect** finds leaf hub where devices connect
2. **Power off** completely disconnects ESP32 (LEDs off, serial port gone)
3. **Power on** successfully reconnects device
4. **Power cycle** performs reliable cold boot with configurable delay

### Verification Commands

```bash
# List all discovered hubs
./espbrew power status

# Check specific hub status
./espbrew power status --location <hub-location>

# Verify with uhubctl (optional)
uhubctl -l <location> -p <port>

# List serial devices before/after
ls -la /dev/serial/by-id/
```

## Supported Hardware

### Tested Hubs

- **Rosonway RSH-A10** (vendor:product 0bda:0411)
  - USB 2.0 hub at location `1-5` (4 ports)
  - USB 3.x hub at location `4-2` (4 ports)
  - Sub-hubs at `1-5.3`, `4-2.3` where devices connect

### Known Compatible Chips

- Product ID pairs: `{5411, 0411}`, `{5413, 0413}`, `{5414, 0414}`

## Future Enhancements

Potential improvements to the power control implementation:

1. **Enable sysfs fallback path** - Wire up `setPortPowerLinux` as primary or fallback
2. **Extended hub discovery** - Support for additional dual-interface hub vendors
3. **Advanced topology handling** - Better support for multi-level USB hub chains
4. **Windows/macOS support** - Platform-specific implementations for non-Linux systems
5. **Real-time status monitoring** - Watch for device connection/disconnection events

## References

- Linux USBDEVFS documentation: `/usr/include/linux/usbdevice_fs.h`
- USB 2.0/3.0 specifications (Hub class descriptors)
- uhubctl source code: https://github.com/mvp/uhubctl (reference implementation using libusb)
