# Camera Identification Strategy

## Current Implementation

Cameras are identified using UUIDs from pion/mediadevices library, with separate device path information for platform-specific operations.

## Data Structure

```go
type CameraInfo struct {
    ID   string // UUID from pion mediadevices
    Name string // Device label from system
    Path string // Platform-specific device path (e.g. /dev/video0 on Linux)
}
```

## Identification by Platform

### Linux (V4L2)
- **ID**: UUID from pion mediadevices (e.g. "61beb4c3-142a-4de5-bfa6-b55f3cb63577")
- **Path**: V4L2 device node (e.g. "/dev/video0")
- **Discovery**: pion/mediadevices with v4l2 backend
- **Controls**: go4vl library using Path field

### macOS (AVFoundation)
- **ID**: Platform-specific UUID
- **Path**: Empty or platform identifier
- **Discovery**: pion/mediadevices with avfoundation backend
- **Controls**: Not supported

### Windows (DirectShow)
- **ID**: Platform-specific identifier
- **Path**: Device path or empty
- **Discovery**: pion/mediadevices with directshow backend
- **Controls**: Not supported

## API Behavior

**Discovery Response**:
```json
{
  "cameras": [
    {
      "id": "61beb4c3-142a-4de5-bfa6-b55f3cb63577",
      "name": "usb-046d_Brio_100_2437APG0Y788-video-index0;video4",
      "path": "/dev/video4",
      "backend": "v4l2"
    }
  ]
}
```

**Controls Request**: `GET /api/v1/camera/{cameraId}/controls`
- `cameraId` = UUID from discovery
- Server uses internal `getCameraPathByID()` to resolve UUID to device path

**Settings Storage**: Keyed by camera ID (UUID)

## Path Extraction

On Linux, device path is extracted from pion's device label using pattern matching:
- Label format: `usb-046d_Brio_100_2437APG0Y788-video-index0;video4`
- Extract `;videoN` suffix to get device node number
- Construct path: `/dev/videoN`

Primary vs metadata device filtering:
- Even-numbered devices (/dev/video0, /dev/video2) are primary video devices
- Odd-numbered devices (/dev/video1, /dev/video3) are metadata devices
- Discovery returns only primary devices

## Cross-Platform Comparison

Similar to device identification:
- **Devices**: Identified by MAC address + metadata (chip_type, path)
- **Cameras**: Identified by UUID + metadata (name, path)

Both provide:
- Stable identifiers across hotplug events
- Platform-specific path for operations
- Discovery for dynamic device detection
