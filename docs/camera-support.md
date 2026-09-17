## Camera Support

ESPBrew can discover and capture images from connected cameras, useful for HMI demos, automated testing, and remote monitoring.

### Platform Support

Camera support uses native platform APIs:

| Platform | Backend | Installation |
|----------|---------|--------------|
| macOS    | AVFoundation | Built-in (no external tools required) |
| Linux    | V4L2/fswebcam | `sudo apt install fswebcam` |
| Windows  | DirectShow | Planned |

### Camera Discovery

List available cameras with friendly names and backend information:

```bash
espbrew cameras
```

Output example:
```
Found 2 camera(s):

1. FaceTime HD Camera
   ID:     EAB7A68F-EC2B-4487-AADF-D8A91C1CB782
   Backend: avfoundation
   Path:   EAB7A68F-EC2B-4487-AADF-D8A91C1CB782

2. UVC CAM1
   ID:     0x1134000303a8000
   Backend: avfoundation
   Path:   0x1134000303a8000
```

Camera discovery uses pion/mediadevices library with native platform backends:
- **macOS**: AVFoundation for device enumeration and capture (no external tools)
- **Linux**: V4L2 for device discovery, fswebcam for capture
- **Windows**: DirectShow (planned)

### ESP32-S3-EYE as USB Camera

The ESP32-S3-EYE development board can be used as a USB camera when running appropriate firmware. This enables using ESP32 hardware as part of your camera setup for HMI testing, device monitoring, and automated capture workflows.

**Hardware Setup:**

- ESP32-S3-EYE devkit (includes built-in camera module and USB support)
- Documentation: [ESP32-S3-EYE Getting Started Guide](https://github.com/espressif/esp-who/blob/master/docs/en/get-started/ESP32-S3-EYE_Getting_Started_Guide.md)

**Firmware:**

Use the USB webcam firmware from Espressif's IoT solutions:

- Repository: [esp-iot-solution](https://github.com/espressif/esp-iot-solution)
- Firmware path: `examples/usb/device/usb_webcam`
- Build and flash using ESP-IDF

```bash
# Clone repository
git clone https://github.com/espressif/esp-iot-solution.git
cd esp-iot-solution/examples/usb/device/usb_webcam

# Build with ESP-IDF
idf.py set-target esp32s3
idf.py build

# Flash to ESP32-S3-EYE
idf.py flash
```

**Using ESP32-S3-EYE with ESPBrew:**

After flashing the webcam firmware:
1. Connect ESP32-S3-EYE via USB
2. It appears as a standard USB camera (UVC device)
3. Use `espbrew cameras` to verify detection
4. Capture images normally

```bash
espbrew cameras                    # Should show ESP32-S3-EYE as UVC device
espbrew capture --camera-id <ID>   # Capture from ESP32-S3-EYE
```

**Advantages:**
- Low-cost camera solution using ESP32 hardware
- Direct USB connection, no network setup required
- Integrates with existing ESPBrew workflows
- Useful for multi-device testing setups

### Capture

Capture images to `~/.espbrew/captures/` with timestamped filenames:

```bash
espbrew capture                                          # Use defaults (1280x720, JPEG 85%)
espbrew capture --width 1920 --height 1080 --quality 90  # Specify parameters
espbrew capture --camera-id 0x1134000303a8000          # Specific camera (use ID from cameras command)
espbrew capture output.jpg                              # Custom output path
```

Captured images are organized by date:
```
~/.espbrew/captures/
├── 2026-05-27/
│   ├── cam-abc123-001.jpg
│   ├── cam-abc123-002.jpg
│   └── metadata.json
```

### Storage

- **Location**: `~/.espbrew/captures/`
- **Organization**: Daily directories with metadata.json
- **Formats**: JPEG (default), PNG
- **Metadata**: Camera ID, timestamp, dimensions, file size

### Managing Captures

List and delete captured images via CLI:

```bash
espbrew captures list                    # List all captures with size and date
espbrew captures delete --all            # Delete all (prompts for confirmation)
espbrew captures delete --older-than 7d  # Delete captures older than 7 days
espbrew captures delete "2026-05-*"      # Delete by pattern
```

### Web Dashboard

The ESPBrew dashboard includes camera controls and capture gallery:

- **Camera Tab**: View available cameras, trigger captures with custom resolution
- **Gallery Tab**: Browse captured images with modal viewer
- **Delete**: Remove individual captures via web UI

Access at `http://localhost:8080` when cluster is running.

### Use Cases

- **HMI Testing**: Capture screenshots of GUI demos for verification
- **AI Observation**: Feed camera images to AI for automated testing
- **Remote Monitoring**: Capture and review images from cluster nodes
- **Documentation**: Generate visual records of device states

### Image Mapping and Device Screenshots

ESPBrew can map physical device locations within camera captures using bounding boxes, enabling automated device-specific screenshot extraction. See [Image Mapping Documentation](docs/image-mapping.md) for details.

**Features:**
- **Bounding Box Editor**: Web UI for drawing device regions on camera captures
- **Device Mapping**: Associate device IDs with camera regions
- **Auto-Extraction**: Automatically extract device subimages after capture
- **Per-Device Adjustments**: Configure brightness, contrast, saturation per device
- **Device Gallery**: View device-specific screenshots in web UI
- **Storage**: Device subimages stored alongside full captures

**CLI Commands:**
```bash
# List device mappings
espbrew mapping list --device-id esp-aa:bb:cc:dd:ee:ff

# Create or update mapping
espbrew mapping set --device-id esp-aa:bb:cc:dd:ee:ff --camera /dev/video0 --bounds 0.1,0.2,0.3,0.4

# Capture with device extraction
espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff --camera /dev/video0

# Export/import mappings
espbrew mapping export --device-id esp-aa:bb:cc:dd:ee:ff --output mappings.json
espbrew mapping import mappings.json
```

**Web Interface:**
- Device Mapping tab with canvas-based bounding box editor
- Device gallery thumbnails with click-to-view
- Camera calibration version tracking

