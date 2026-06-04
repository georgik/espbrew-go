# Camera Identification Strategy

## Problem

Currently, cameras are identified inconsistently across the application:
- Discovery uses pion's device IDs (UUID-like: `usb-046d_HD_Webcam_C615_C574F460-video-index0`)
- V4L2 controls expect device paths (`/dev/video0`)
- Storage and API use these IDs interchangeably, causing failures

## Current State

```go
type CameraInfo struct {
    ID   string // pion UUID: "usb-046d_HD_Webcam_C615_C574F460-video-index0"
    Path string // V4L2 path: "/dev/video0"
}
```

**Issues:**
- UI passes `ID` (UUID) to API endpoints
- API tries to use UUID as device path for V4L2 controls
- V4L2 rejects UUID: `stat usb-...: no such file or directory`
- Camera controls and preview don't work

## Desired State

### Camera Identification Rules

1. **Linux V4L2**: Use device path `/dev/video0` as primary identifier
2. **macOS AVFoundation**: Use device UUID (platform-specific)
3. **Windows DirectShow**: Use device path or UUID (platform-specific)

### Storage Key Strategy

**Camera settings storage key**: Use the platform-specific device path/ID
- Linux: `/dev/video0`
- macOS: `0x1a2b3c4d...` (UUID)
- Windows: `@device:pnp://...` (path)

**Display name**: Keep human-readable name separate
- `CameraInfo.Name`: "HD Webcam C615" (for UI display)

### API Behavior

**Discovery Response**:
```json
{
  "cameras": [
    {
      "id": "/dev/video0",
      "name": "HD Webcam C615",
      "path": "/dev/video0",
      "backend": "v4l2"
    }
  ]
}
```

**Controls Request**: `GET /api/v1/camera/{id}/controls`
- `id` = `/dev/video0` (directly usable by V4L2)

**Settings Storage**: Keyed by `id` (`/dev/video0`)

## Implementation Plan

1. **Modify CameraInfo struct**: Make `ID` contain the usable device identifier
2. **Update discovery**: Set `ID` to device path on Linux
3. **Update UI**: Use `id` for all API calls (already correct)
4. **Add alias mapping**: For backward compatibility with existing settings

### Backward Compatibility

Existing settings stored with UUID keys need migration:
1. On startup, scan settings for UUID keys
2. Map UUIDs to current device paths
3. Migrate settings to new keys
4. Keep UUID->path mapping for future reference

## Cross-Platform Comparison

Similar to device identification:
- **Devices**: Identified by path (`/dev/ttyUSB0`) + metadata (chip_type)
- **Cameras**: Identified by path (`/dev/video0`) + metadata (name, backend)

Both should:
- Use stable device path as primary key
- Store metadata separately
- Provide discovery for hotplug support
