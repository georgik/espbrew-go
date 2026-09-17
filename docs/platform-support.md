## Windows Support

ESPBrew provides full support for Windows with automatic COM port detection and monitoring.

### COM Port Detection

On Windows, ESPBrew automatically detects COM ports (COM1, COM2, etc.) used by ESP32 devices. The application identifies USB serial devices by common patterns including:
- USB modem and serial device identifiers
- Common USB-to-serial chip manufacturers (SLAB, CP21, FTDI, CH340)
- COM port prefixes

### Device Discovery

```bash
# List all serial devices on Windows
espbrew.exe devices

# List only ESP devices
espbrew.exe devices --esp
```

### Flashing on Windows

```bash
# Flash to auto-detected COM port
espbrew.exe flash firmware.bin

# Flash to specific COM port
espbrew.exe flash firmware.bin -p COM5

# Flash with chip specification
espbrew.exe flash firmware.bin -p COM5 --chip esp32-s3
```

### Serial Monitoring on Windows

```bash
# Monitor auto-detected COM port
espbrew.exe monitor

# Monitor specific COM port
espbrew.exe monitor -p COM5

# Monitor with reset to capture boot logs
espbrew.exe monitor -p COM5 --reset
```

### Web Interface on Windows

The web dashboard provides full monitoring capabilities for Windows COM ports:

```
http://localhost:8080
```

The serial monitor interface automatically handles Windows COM port paths without the `/dev/` prefix used on Unix systems.

## Linux Support

ESPBrew provides full support for Linux with stable device paths via `/dev/serial/by-id` and USB hub power control integration.

### Device Discovery

On Linux, ESPBrew automatically discovers USB serial devices using stable `/dev/serial/by-id/` symlinks. This provides persistent device identification across reconnections and reboots.

```bash
# List all serial devices on Linux
./espbrew devices

# List only ESP devices
./espbrew devices --esp

# View detailed device information
./espbrew devices --json
```

### Stable Device Paths

ESPBrew resolves temporary ports (`/dev/ttyUSB0`, `/dev/ttyACM0`) to stable symlinks:
```
/dev/ttyUSB0 → /dev/serial/by-id/usb-Espressif_ESP32-S3-DevKitC-1_1234567890-if00
```

This ensures device records persist across reconnections and system reboots.

### Device Permissions

Access to USB serial devices on Linux typically requires group membership or device-specific permissions.

**Check current access:**
```bash
# Check dialout group membership (for serial ports)
groups | grep dialout

# Check device permissions
ls -la /dev/serial/by-id/
```

**Add user to dialout group:**
```bash
# Add user to dialout group for serial port access
sudo usermod -aG dialout $USER

# Relogin required for group membership to take effect
# Or test immediately:
newgrp dialout
```

**Important**: After adding yourself to a group, you must **log out and log back in** for the change to take effect. Simply restarting your terminal is not enough — the group membership is set at login time.

### Native USB Hub Power Control

ESPBrew includes native USB hub power control support on Linux (kernel >= 6.0), enabling automated device reset cycles for cold boot testing without external dependencies.

**Requirements:**
- Linux kernel >= 6.0
- USB hub with per-port power switching (e.g., Rosonway RSH-A10, 0BDA:0411)

**Dual-Interface Hub Architecture:**

Modern USB 3.x hubs present as two separate hub devices to the system:
- USB 2.0 hub - handles USB 2.0 traffic
- USB 3.x hub - handles SuperSpeed traffic

For complete power control, both interfaces must be powered simultaneously. ESPBrew automatically detects dual-interface hubs and coordinates power operations across both interfaces.

**Automatic Hub Discovery:**

ESPBrew automatically discovers leaf hubs (hubs with no downstream hub devices) where ESP32 devices are actually connected. This ensures correct power control even when devices are connected through intermediate sub-hubs in the USB topology.

When you specify a hub location, ESPBrew will:
1. Discover all available hubs with per-port power switching
2. Filter to leaf hubs (hubs without downstream hubs on their ports)
3. Automatically pair dual-interface hubs (USB 2.0 + USB 3.x) for coordinated power control

**Commands:**
```bash
# Auto-detect supported hub
./espbrew power auto-detect

# Show hub and port status
./espbrew power status

# Power control (specify leaf hub location from auto-detect or status output)
./espbrew power on <port> --location <leaf-hub-location>
./espbrew power off <port> --location <leaf-hub-location>
./espbrew power cycle <port> --location <leaf-hub-location>

# Power cycle with custom delay
./espbrew power cycle <port> --location <leaf-hub-location> --delay 3s
```

**Example: Rosonway RSH-A10 (10-port hub)**

The Rosonway RSH-A10 presents as multiple hubs in the USB topology. ESPBrew automatically identifies and controls the leaf hubs where devices are connected:

```bash
# Auto-detect finds the correct leaf hub
./espbrew power auto-detect
# Output: Found hub at location: 4-2.3 (USB 3.0)

# Show status to see all discovered hubs
./espbrew power status
# Lists all hubs with their port statuses

# Power control (auto-detect identifies the correct hub)
./espbrew power off 3 --location 4-2.3
./espbrew power on 3 --location 4-2.3

# Power cycle with 3 second delay
./espbrew power cycle 3 --location 4-2.3 --delay 3s
```

ESPBrew automatically pairs the USB 2.0 and USB 3.x leaf hubs and powers both simultaneously for complete device disconnect.

**Permissions:**

The native implementation requires write access to USB device files. Create a udev rule:

```bash
# Add udev rule for USB hub access
sudo tee /etc/udev/rules.d/99-espbrew-hub.rules <<EOF
SUBSYSTEM=="usb", ATTR{idVendor}=="0bda", ATTR{idProduct}=="0411", TAG+="uaccess"
EOF

# Reload rules
sudo udevadm control --reload-rules
sudo udevadm trigger
```

**Kernel Compatibility:**

If running on kernel < 6.0, the power commands will return an error indicating kernel upgrade is required. As an alternative, use the `uhubctl`.

For detailed implementation information, see [docs/power-control.md](docs/power-control.md).

### Flashing on Linux

```bash
# Flash to auto-detected device
./espbrew flash firmware.bin

# Flash to specific device path
./espbrew flash firmware.bin -p /dev/serial/by-id/usb-Espressif_...

# Flash with chip specification
./espbrew flash firmware.bin --chip esp32-s3
```

### Serial Monitoring on Linux

```bash
# Monitor auto-detected device
./espbrew monitor

# Monitor specific device
./espbrew monitor -p /dev/serial/by-id/usb-Espressif_...

# Monitor with reset to capture boot logs
./espbrew monitor -p /dev/serial/by-id/usb-Espressif_... --reset
```

### Web Interface on Linux

The web dashboard provides full monitoring capabilities:

```
http://localhost:8080
```

The serial monitor interface automatically handles Linux device paths including `/dev/serial/by-id/` symlinks.

