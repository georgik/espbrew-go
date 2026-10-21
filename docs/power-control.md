# USB Hub Power Control Implementation

This document describes the internal implementation of ESPBrew's native USB hub power control feature on Linux.

## Architecture Overview

ESPBrew implements USB hub power control using two mechanisms:

1. **sysfs write interface** (primary) - Writing to `/sys/bus/usb/devices/<if>/\<location\>-portN/disable` files (kernel >= 6.0). This is the path that works on **parent hubs** (hubs with downstream sub-hubs), which is what real deployments need.
2. **USBDEVFS ioctl** (fallback) - Direct communication with USB device files via kernel ioctls. Used only when the kernel is too old for the sysfs `disable` file (`ErrKernelTooOld`).

The implementation uses no external dependencies (no libusb, no cgo bindings, **no uhubctl**), relying entirely on the Go standard library and Linux kernel interfaces.

### Which path is actually used

`setPortPower` is wired to `setPortPowerSysfsOrIoctl` (see `internal/powercontrol/sysfs_linux.go`):

```go
func setPortPowerSysfsOrIoctl(hub *Hub, port int, on bool) error {
    if err := setPortPowerLinux(hub, port, on); err != nil {
        if errors.Is(err, ErrKernelTooOld) {
            return setPortPowerIoctl(hub, port, on) // legacy kernels without the disable file
        }
        return err
    }
    return nil
}
```

sysfs is tried first because it needs **no `/dev/bus/usb` access** and works on parent hubs.
The ioctl path needs the `usbfs` filesystem mounted (it is **not** on the production host kernel
`7.0.0-1019-nvidia`, where `/proc/filesystems` has no `usbfs` line) and can only reach leaf hubs.

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

Only "leaf" hubs (hubs with no downstream hub devices) are included for power control **discovery**.

### Driving parent hubs

The discovery filter above is deliberately **not** a hard restriction on driving. A board can sit
on a **parent hub** (a hub that itself has downstream sub-hubs) — e.g. the staging board sits on
`1-6` (a `0bda:5411` hub) at port 1, exposing the CH9102 at `1-6.1`. Driving the parent hub's port
is exactly what powers the board off and on.

`FindHubByLocation` handles this:

1. It first checks the enumerated leaf-hub list (`listHubsLinux`).
2. If the location is a parent hub (not in the leaf cache), it resolves it with the sysfs
   fallback (`hubParseFallback`), which reads the hub's ports directly from `/sys/bus/usb/devices`.

`SetPortPowerDual` then drives it directly:

```go
func (c *controller) SetPortPowerDual(hub *Hub, port int, on bool) error {
    // Only link dual-interface partners that we actually enumerated.
    if hubInCache != nil && hubInCache(hub.Location) && hub.Counterpart == nil {
        linkDualHubs(hub)
    }
    // ...drive primary (and secondary, if present) port...
}
```

For a parent hub that was **not** enumerated (so it has no `Counterpart`), `linkDualHubs` is
skipped and the single parent port is driven directly. This is the path the staging deployment
uses, and it is why the sysfs interface is the primary mechanism — the ioctl path cannot address
parent hubs at all.

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

### setPortPowerLinux() (Primary Path)

Writes to sysfs `disable` files. The path is per-interface:

```
/sys/bus/usb/devices/<interfaceDir>/<location>-portN/disable
```

e.g. `/sys/bus/usb/devices/1-6:1.0/1-6-port1/disable` for the parent hub `1-6`, port 1.

- `"1"` = disable the port (power off)
- `"0"` = enable the port (power on)

Available on Linux kernel >= 6.0. This is the **primary** path and the only one that works on
parent hubs. Note that the write can be blocked at the kernel level (see *Deployment* below).

### setPortPowerIoctl() (Fallback Path)

Uses USBDEVFS ioctl via `/dev/bus/usb/<bus>/<device>`. Only reached when the sysfs write fails with
`ErrKernelTooOld` (kernel < 6.0, so the `disable` file does not exist). Requires the `usbfs`
filesystem to be mounted — it is **not** on the production host kernel, so this path is effectively
dead there and sysfs is the only option.

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
- Hub discovery via sysfs enumeration (leaf hubs)
- Parent-hub resolution via `hubParseFallback`
- Dual-hub linking and coordination (only for enumerated hubs)
- **Hybrid** power control: `setPortPowerSysfsOrIoctl` tries sysfs first, falls back to ioctl
  only on `ErrKernelTooOld`. `init()` wires `setPortPower = setPortPowerSysfsOrIoctl`.
- Port status reading via ioctl (`getPortStatusIoctl`)

### Deployment (k3s / container)

Driving the sysfs `disable` file from inside a container has **two** independent blockers, both of
which must be cleared for the power control to work in a pod:

1. **`/sys` must be mounted read-write.** The default Kubernetes mount of `/sys` is read-only, so
   the write fails with `EACCES`. Add a `hostPath` volume mounting the host `/sys` at `/sys`
   (`type: Directory`, read-write).
2. **The kernel blocks the write unless the container is privileged.** Even a read-write `/sys`
   mount plus `CAP_SYS_ADMIN` returns `-EPERM` for the `disable` file — the host kernel (7.0.x)
   enforces sysfs write-protection independent of capabilities. The write succeeds only when the
   container runs with `securityContext.privileged: true`.

Staging is a test/dev instance, so `privileged: true` is acceptable there. Production uses the
`esp.dev/serial-port` device-plugin (not hub power control) and does **not** need this.

```yaml
# deployment-staging.yaml
volumeMounts:
  - name: sys
    mountPath: /sys          # host /sys, read-write
volumes:
  - name: sys
    hostPath: { path: /sys, type: Directory }
container:
  securityContext:
    privileged: true         # required: kernel blocks disable-file writes otherwise
```

### Non-Linux (`stub_other.go`)

Returns `ErrNotSupported` for all operations. Compiled with `//go:build !linux`.

## Power-Managed Device Auto Sleep/Wake

On top of the raw hub power control above, espbrew can treat selected boards as
**power-managed**: it powers them off when idle and powers them back on right
before an operation. This is configured per-device in `espbrew.toml` and driven
by the `internal/devicesleep` package.

### Configuration

Add `usb_location` and `usb_port` to a `[[devices]]` entry. `usb_location` is the
sysfs hub location string (e.g. `1-2`) that supplies power to the board;
`usb_port` is the 1-based port number on that hub the board is plugged into.

```toml
[[devices]]
path = "/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_98:88:E0:D4:D2:58-if00"
id       = "esp-98:88:E0:D4:D2:58-if00"
alias    = "esp32-c3-lcdkit"
chip     = "ESP32-C3"
usb_location = "1-2"
usb_port   = 3
description = "ESP32-C3 LcdKit on a powered hub"
```

A board is power-managed only when `usb_location` is set (see
`config.DeviceConfig.PowerManaged` / `PowerLocation`). The pair fully specifies
where the board sits in the USB tree, which is what espbrew needs to switch its
power.

### Behavior

- **Sleeping when absent.** If a power-managed board is not present in the live
  USB tree, espbrew marks it `sleeping` (status `sleeping` in the API) and keeps
  a record so it can still be addressed by alias.
- **Wake before use.** When an operation (flash/monitor/erase) targets a
  sleeping board, espbrew powers the hub port on and waits `WakeDelay`
  (default **1 s**) for the board to re-enumerate before starting.
- **Auto power-down.** While an operation runs the board is flagged `busy` and is
  never powered off. After the operation finishes the idle timer starts; if the
  board is unused for `IdleTimeout` (default **2 min**), espbrew powers the port
  off and returns the board to `sleeping`.

This enables cold-start testing (a board that is only ever rebooted never
exposes hidden peripheral state) and power saving when many boards are
connected.

### Implementation

- `internal/devicesleep` owns the state machine (sleep/wake/busy/idle timer) and
  talks to the outside world only through the `Controller` interface, a slice of
  `powercontrol.PowerController`. The real controller drives the hub on Linux;
  tests inject an in-memory mock, so the whole behaviour is covered without a
  physical hub.
- The leader (`internal/cluster/leader_sleep.go`) reconciles board state against
  the live USB tree, wakes boards before a job, and runs a background loop that
  powers down idle boards and mirrors the result onto the live device records.

### Tunables

All timings live in `devicesleep.Config` (`DefaultConfig`): `WakeDelay`
(1 s), `IdleTimeout` (2 m), `SweepPeriod` (5 s, how often the background loop
checks). Tests override these via `LeaderConfig.SleepConfig` to run fast.

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
# List serial devices before/after a cycle (board should disappear then reappear)
ls -la /dev/serial/by-id/

# Confirm the hub port was actually toggled in sysfs
# (parent hub 1-6, port 1): the value flips between 0 and 1
cat /sys/bus/usb/devices/1-6:1.0/1-6-port1/disable
```

The authoritative proof that a power cycle rebooted the board is a **fresh USB enumeration**: the
board's `devnum` in `/sys/bus/usb/devices/1-6.1/uevent` changes after each cycle, and the
`/dev/serial/by-id` node is removed and recreated.

## Supported Hardware

### Tested Hubs

- **Rosonway RSH-A10** (vendor:product 0bda:0411)
  - USB 2.0 hub at location `1-5` (4 ports)
  - USB 3.x hub at location `4-2` (4 ports)
  - Sub-hubs at `1-5.3`, `4-2.3` where devices connect

### Tested Parent Hub

- **Generic USB 2.0 hub** (vendor:product `0bda:5411`)
  - Parent hub at location `1-6` (has downstream sub-hubs), port 1
  - Drives the **waveshare CH9102** board (`1a86:55d3`, exposed as `1-6.1`)
  - This is the staging power-control target and the case that required the sysfs-primary,
    parent-hub-driving path described above.

### Known Compatible Chips

- Product ID pairs: `{5411, 0411}`, `{5413, 0413}`, `{5414, 0414}`

## Future Enhancements

Potential improvements to the power control implementation:

1. **Extended hub discovery** - Support for additional dual-interface hub vendors
2. **Advanced topology handling** - Better support for multi-level USB hub chains
3. **Platform parity** - Windows/macOS implementations for non-Linux systems
4. **Real-time status monitoring** - Watch for device connection/disconnection events

> Note: the sysfs path is now the primary mechanism and uhubctl is no longer a dependency, so the
> "enable sysfs fallback" item from earlier drafts is complete.

## References

- Linux USBDEVFS documentation: `/usr/include/linux/usbdevice_fs.h`
- USB 2.0/3.0 specifications (Hub class descriptors)
- uhubctl source code: https://github.com/mvp/uhubctl (reference implementation using libusb)
