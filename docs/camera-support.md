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

### Cluster capture (k3s and container)

When espbrew runs as a cluster node in a container or k3s pod, a camera attached
to that host can be captured remotely with `snap --cluster`. Two things must be in
place:

1. **The device node must be openable inside the container.** A plain `/dev`
   mount makes the node *visible*, but the container's cgroup v2 device
   controller still denies `open()` with `EPERM`.
   - **Bare container:** make the host node world-accessible (`0666`) with a
     `udev` rule, e.g. `SUBSYSTEM=="video4linux", MODE="0666"`. See
     [docs/container.md](container.md).
   - **k3s pod:** run the espbrew camera Device Plugin, which grants the
     device-cgroup allow rule when the pod requests the `esp.dev/camera`
     resource. This is set up by [deploy/README.md](../deploy/README.md).
2. **`fswebcam` must be installed** — Linux capture goes through it
   (`internal/camera/capture_legacy.go`). The `Containerfile.k8s` image ships it;
   for a bare container install it on the host or in your own image.

Once the camera is discoverable on the node, capture it from anywhere:

```bash
espbrew snap --cluster http://leader:8080 --device esp-aa:bb:cc:dd:ee:ff   # captures the node's camera
```

### Device cropping via `espbrew.toml`

For a board fixed under a camera, declare the camera and a bounding box in
`espbrew.toml`. On startup the leader persists this device→camera mapping, and
`snap` crops the frame to it automatically — no UI calibration required.

```toml
[[devices]]
path  = "/dev/serial/by-id/usb-…-if00"
id    = "esp-30:30:F9:5A:8F:D4-if00"
alias = "esp32-s3-box-3"
chip  = "ESP32-S3"

camera = "cam-usb-046d_Brio_100_2437APG0Y788"   # from `espbrew cameras`
[devices.camera_box]
x      = 0.3671875      # normalized 0.0–1.0, resolution-independent
y      = 0.26634114583333335
width  = 0.2453125
height = 0.2375
```

```bash
espbrew snap --cluster http://leader:8080 --filter-alias esp32-s3-box-3
```

How the pieces connect:

- The crop is keyed on the device `id` and the camera. The `id` here **must match**
  the identity the leader assigns to the board — the same `id` the CLI resolves
  from `--filter-alias`. Because `espbrew.toml` is authoritative for device
  identity, the runtime device id equals this configured `id`, so the crop lookup
  always hits the mapping.
- `camera` may be the discovered camera ID (e.g. `cam-usb-…`) **or** its stable
  Name. Camera IDs from `pion/mediadevices` can change between restarts; the name
  is stable, so prefer the name when the ID looks ephemeral. List cameras with
  `espbrew cameras`.
- The box is normalized (`0.0–1.0`), so it is independent of camera resolution.
  For a 640×480 camera, the box above yields a **157×114** JPEG cropped to the
  board instead of the full 640×480 frame.

Find the region by drawing a box around the board in the dashboard's bounding-box
editor (or `espbrew mapping set`) — it reports the normalized `x, y, width,
height`. The full mapping model, REST API, and CLI are in
[docs/image-mapping.md](image-mapping.md).

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

ESPBrew can map physical device locations within camera captures using bounding boxes, enabling automated device-specific screenshot extraction. See [Image Mapping Documentation](image-mapping.md) for details.

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

