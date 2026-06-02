# Image Mapping Feature

## Overview

The Image Mapping feature allows users to associate physical device locations within camera captures using bounding boxes. This enables automatic device identification during image capture workflows, supporting automated visual verification of flashed devices.

## Use Case

In production setups where multiple ESP devices are positioned under a single camera, the system needs to:

1. Identify which portion of a captured image corresponds to each device
2. Extract device-specific screenshots for verification
3. Support automated workflows: flash → monitor → capture → verify

Example scenario:
- 4 ESP32-S3 boards fixed on a test jig
- Single overhead camera captures all boards
- User defines bounding boxes for each board position
- After flashing, system extracts individual board screenshots
- Verification confirms expected display output per device

## Architecture

### Data Model

```go
// BoundingBox represents normalized coordinates (0.0-1.0) relative to image dimensions
type BoundingBox struct {
    X      float64 `json:"x"`      // Top-left X (0.0-1.0)
    Y      float64 `json:"y"`      // Top-left Y (0.0-1.0)
    Width  float64 `json:"width"`  // Width (0.0-1.0)
    Height float64 `json:"height"` // Height (0.0-1.0)
}

// ImageAdjustment stores per-region image enhancement settings
type ImageAdjustment struct {
    Brightness int `json:"brightness"` // -100 to 100, 0 = no change
    Contrast   int `json:"contrast"`   // -100 to 100, 0 = no change
    Saturation int `json:"saturation"` // -100 to 100, 0 = no change
}

// DeviceBoundingBoxMapping maps a device to its location in a camera view
type DeviceBoundingBoxMapping struct {
    ID                 string           `json:"id"`
    DeviceID           string           `json:"device_id"`           // Device reference
    CameraID           string           `json:"camera_id"`           // Camera reference (can change)
    CameraName         string           `json:"camera_name"`         // Stable camera identifier
    Bounds             BoundingBox      `json:"bounds"`              // Normalized box
    CalibrationVersion int              `json:"calibration_version"` // Camera position version
    Adjustment         ImageAdjustment  `json:"adjustment"`          // Per-region image enhancement
    CreatedAt          time.Time        `json:"created_at"`
    UpdatedAt          time.Time        `json:"updated_at"`
}

// CameraCalibration stores camera position data
type CameraCalibration struct {
    ID              string    `json:"id"`
    CameraID        string    `json:"camera_id"`
    Version         int       `json:"version"`         // Increment on position change
    Description     string    `json:"description"`     // Human-readable position name
    ReferenceImage  string    `json:"reference_image"` // Optional reference path
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}
```

### Storage

**Persistence:**
- BoltDB buckets: `camera_calibrations`, `bounding_boxes`
- Indexes: camera_id (for querying all boxes in camera), device_id (reverse lookup)
- Survives restarts

**Screenshot Storage:**
```
~/.espbrew/screenshots/
├── {device_id}/
│   ├── {timestamp}.jpg
│   └── thumbnails/
│       └── {timestamp}_thumb.jpg
└── screenshots.json (metadata)
```

### Coordinate System

Normalized (0.0-1.0) coordinates:
- Resolution-independent (works with 640x480, 1920x1080, 4K)
- Industry standard (YOLO, COCO format)
- Conversion: `pixelX = x * imageWidth`, `pixelY = y * imageHeight`

## API Endpoints

### Mapping Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/cameras/{id}/boxes` | GET | Get all bounding boxes for camera |
| `/api/v1/cameras/{id}/calibration` | GET | Get camera calibration |
| `/api/v1/cameras/{id}/calibration` | POST | Create new calibration version |
| `/api/v1/bounding_boxes` | POST | Create bounding box mapping |
| `/api/v1/bounding_boxes/{id}` | PUT | Update bounding box |
| `/api/v1/bounding_boxes/{id}` | DELETE | Delete bounding box |

### Create Mapping

**Request:**
```json
POST /api/v1/bounding_boxes
{
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "camera_id": "cam-001",
  "bounds": {
    "x": 0.1,
    "y": 0.2,
    "width": 0.3,
    "height": 0.4
  }
}
```

**Response:**
```json
{
  "id": "bbox-123",
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "camera_id": "cam-001",
  "bounds": {...},
  "created_at": "2026-06-01T12:00:00Z"
}
```

### Camera Boxes

**Response:**
```json
GET /api/v1/cameras/cam-001/boxes
{
  "camera_id": "cam-001",
  "calibration": {
    "version": 1,
    "description": "Position 1"
  },
  "mappings": [
    {
      "id": "bbox-1",
      "device_id": "esp-aa:bb:cc:dd:ee:ff",
      "bounds": {...},
      "device": {
        "device_id": "esp-aa:bb:cc:dd:ee:ff",
        "chip_type": "ESP32-S3",
        "aliases": ["devkit-1"]
      }
    }
  ]
}
```

## Web UI

### Bounding Box Editor

**Components:**
1. **Image Selection** - Gallery of recent captures with "Map Devices" button
2. **Canvas Workspace** - Image with overlay drawing canvas
3. **Device Palette** - Draggable device chips for association
4. **Mappings List** - Current boxes with device assignments

**Interactions:**
- Drag on canvas to create bounding box
- Drag device chip onto box to assign
- Right-click/long-press for context menu
- Visual states: unmapped (blue), mapped (green), selected (highlighted)

**Features:**
- Touch/mouse unified input
- Real-time coordinate display
- Box editing (resize, move, delete)
- Export/import configurations

### Device Detail Enhancement

Add to device detail modal:
- "Set Reference Image" section
- Current bounding box preview
- List of all mappings for this device
- Edit/delete controls

## CLI Commands

### Mapping Management

```bash
# List all mappings for device
espbrew mapping list --device-id esp-aa:bb:cc:dd:ee:ff

# Create or update bounding box mapping
espbrew mapping set --device-id esp-aa:bb:cc:dd:ee:ff --camera cam-001 --bounds 0.1,0.2,0.3,0.4

# With image adjustments (per-device enhancement)
espbrew mapping set --device-id esp-aa:bb:cc:dd:ee:ff --camera cam-001 --bounds 0.1,0.2,0.3,0.4 \
  --adjust-brightness 20 --adjust-contrast 10 --adjust-saturation -5

# Delete a mapping
espbrew mapping remove --id bbox-123

# Export mappings to JSON
espbrew mapping export --device-id esp-aa:bb:cc:dd:ee:ff --output mappings.json

# Import mappings from JSON
espbrew mapping import mappings.json
```

### Capture and Verify

```bash
# Capture with device-specific extraction
espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff

# With specific camera
espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff --camera-id cam-001

# With custom output
espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff --output /tmp/verify.jpg

# With custom dimensions
espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff --width 1920 --height 1080
```

**Exit Codes:**
- `0`: Success
- `1`: No bounding box found for device
- `2`: Capture failed
- `3`: Extraction failed

**Output Location:** `~/.espbrew/verify/<device>-<timestamp>.jpg` (or custom path via `--output`)

## Workflow Integration

### Flash and Verify

```bash
espbrew workflow flash-and-verify firmware.bin \
  --device esp-aa:bb:cc:dd:ee:ff \
  --exit-on "System ready" \
  --capture-after \
  --verify-expect-text "HMI initialized"
```

**Output (JSON):**
```json
{
  "status": "success",
  "workflow_id": "wf-123",
  "flash": {
    "status": "completed",
    "duration_ms": 4500
  },
  "monitor": {
    "status": "success",
    "matched_pattern": "System ready"
  },
  "capture": {
    "status": "success",
    "screenshot_path": "~/.espbrew/screenshots/esp-aa:bb:cc:dd:ee:ff/20260601-120000.jpg"
  },
  "verification": {
    "status": "passed",
    "method": "ocr",
    "confidence": 0.95
  }
}
```

## Implementation Status

### Phase 1: Backend Foundation COMPLETE
- **Persistence layer** (`internal/persistence/bbox.go`, `mapping.go`)
  - BoundingBox with normalized coordinates (0-1)
  - DeviceBoundingBoxMapping CRUD operations
  - CameraCalibration with versioning
  - BoltDB buckets: `bounding_boxes`, `calibrations`
  - Indexes for camera_id and device_id lookups
- **API endpoints** (`internal/http/mapping.go`)
  - GET `/api/v1/cameras/{id}/boxes` - List all boxes for camera
  - GET `/api/v1/cameras/{id}/calibration` - Get calibration info
  - POST `/api/v1/cameras/{id}/calibration` - Create new calibration version
  - POST `/api/v1/bounding_boxes` - Create mapping
  - PUT `/api/v1/bounding_boxes/{id}` - Update mapping
  - DELETE `/api/v1/bounding_boxes/{id}` - Delete mapping

### Phase 2: Web UI COMPLETE
- **Canvas bounding box editor** (`internal/dashboard/static/js/bbox-editor.js`)
  - BoundingBoxEditor class with drawing, selection, edit modes
  - Unified pointer events (mouse + touch)
  - Coordinate conversion (pixels ↔ normalized)
  - Visual states: unmapped (blue), mapped (green), selected (yellow)
  - Events: boxCreated, boxSelected, boxModified, boxDeleted
- **UI integration** (`internal/dashboard/static/index.html`)
  - "Device Mapping" tab in navigation
  - Mapping editor modal with gallery, canvas, device palette
  - Device detail modal enhancements (bounding box section)

### Phase 3: CLI & Automation COMPLETE
- **Mapping commands** (`cmd/espbrew/mapping.go`)
  - `mapping list --device-id <id>` - List mappings for device
  - `mapping set --device-id <id> --camera <cam> --bounds <x,y,w,h>` - Create/update mapping
  - `mapping remove --id <bbox-id>` - Delete mapping
  - `mapping export --device-id <id> --output <file>` - Export to JSON
  - `mapping import <file>` - Import from JSON
- **Capture verify** (`cmd/espbrew/capture_verify.go`)
  - `capture verify --device-id <id>` - Capture and extract device region
  - Flags: `--camera-id`, `--output`, `--width`, `--height`
  - Exit codes: 0 (success), 1 (no box), 2 (capture failed), 3 (extraction failed)

### Phase 4: Testing & Polish COMPLETE
- **Unit tests** (`internal/persistence/bbox_test.go`, `mapping_test.go`, `internal/http/mapping_test.go`)
  - BoundingBox validation tests (12 test cases)
  - Coordinate conversion and round-trip tests
  - Mapping CRUD operations tests
  - Camera/device index query tests
  - Calibration version management tests
  - API handler tests (create, update, delete, list)
- **Code quality** - fmt and vet clean
- **URL encoding fix** - Camera IDs in API calls now use `encodeURIComponent()` to handle special characters like `/dev/video0`

### Phase 5: Image Enhancement COMPLETE
- **Per-region adjustments** - Brightness/contrast/saturation per device box
  - ImageAdjustment struct with validation (-100 to 100 range)
  - Stored with DeviceBoundingBoxMapping
  - Post-process when extracting region (internal/camera/postprocess.go)
  - CLI: `--adjust-brightness`, `--adjust-contrast`, `--adjust-saturation`
  - API: PUT /api/v1/bounding_boxes/{id} accepts adjustment

### Phase 6: Device-Specific Screenshots COMPLETE
- **Auto-extraction on capture** - Device subimages automatically extracted after capture
  - Queries mappings for camera
  - Extracts each device region using bounding boxes
  - Applies adjustments if configured
  - Saves device subimages to capture subdirectory
- **Storage structure:**
  ```
  ~/.espbrew/captures/
  ├── YYYY-MM-DD/
  │   ├── cam-abcd-YYYYMMDD-HHMMSS.jpg       # Full capture
  │   ├── cam-abcd-YYYYMMDD-HHMMSS.json      # Device captures metadata
  │   └── cam-abcd-YYYYMMDD-HHMMSS/
  │       ├── device-12345.jpg               # Device subimage
  │       └── device-67890.jpg               # Another device
  ```
- **API Endpoints:**
  - `GET /api/v1/captures/{captureId}/devices` - List device captures for specific capture
  - `GET /api/v1/devices/{deviceId}/captures` - List all captures for a device
- **Manual extraction:** ExtractFromCaptureFile() for on-demand extraction

### Phase 7: UI Enhancements COMPLETE
- **Device gallery thumbnails** - Device-specific subimages shown in capture gallery
  - Automatic loading of device captures per capture
  - Thumbnail display with device count badge
  - Click to view full device subimage in modal
  - Empty state handling when no device captures available
  - URL-encoded paths for proper routing
- **Loading states** - Async device capture loading with visual feedback

### Phase 8: Bug Fixes COMPLETE
- **URL encoding fix** - Camera IDs in API calls now use `encodeURIComponent()` to handle special characters like `/dev/video0`
- **Device capture serving fix** - Fixed JSON field name mismatch (`subimage_path` vs `subimage`) in device gallery
- **Persistence tests** - Added `TestBoundingBoxPersistenceAcrossReopen` to verify mappings survive database closure
- **Extraction bug fix** - Fixed `ExtractAndAdjust` to always extract region first, then apply adjustments. Previously, when adjustments were zero, it returned full image instead of subimage
- **Logging improvements** - Added debug logging for bounding box creation and retrieval
- **Black image bug fix** - Fixed `ApplyAdjustments` to create result image with origin at (0,0) instead of using source bounds. Previously, extracting from non-origin bounds (e.g., (45,45)-(55,55)) created an image with those bounds, causing `Set(0,0, pixel)` to write outside valid bounds, resulting in black images
- **Test coverage** - Added `TestExtractAndAdjust_NonOriginBounds` and `TestExtractAndAdjust_CornerExtraction` to prevent regression

### Phase 9: Duplicate Prevention COMPLETE
- **Unique device-camera constraint** - Creating a bounding box for same device+camera now updates existing mapping instead of creating duplicate
- **Upsert behavior** - `POST /api/v1/bounding_boxes` checks for existing mapping and updates if found
- **API change** - Create endpoint returns `200 OK` with updated mapping when duplicate detected, instead of `201 Created`
- **Persistence layer** - Added `GetBoundingBoxForDeviceAndCamera(deviceID, cameraID)` for efficient lookup
- **Test coverage** - Added `TestGetBoundingBoxForDeviceAndCamera` and `TestUniqueDeviceCameraMapping`

### Phase 10: CLI Integration Extensions COMPLETE
- **Device capture listing command** - `espbrew capture list --device-id <id>` - Lists all device-specific captures
- **Device capture retrieval** - `espbrew capture get <capture-id> --device-id <id>` - Retrieves device subimage
- **Generic capture listing** - `espbrew capture list` - Lists all camera captures
- **Files:**
  - `cmd/espbrew/capture_list.go` - Capture listing commands
  - `cmd/espbrew/capture_get.go` - Device capture retrieval command

### Phase 11: Camera Name-Based Lookup COMPLETE
- **Stable camera identifier** - Added `camera_name` field to `DeviceBoundingBoxMapping` for persistent identification across restarts
- **Fallback lookup logic** - `ListBoundingBoxesForCamera` now attempts name-based lookup if camera ID lookup fails
- **Automatic ID migration** - When mappings are found by name, their `camera_id` is updated to the current value
- **Problem solved** - Camera DeviceIDs from pion/mediadevices can change between restarts; camera names remain stable
- **API changes:**
  - `POST /api/v1/bounding_boxes` accepts optional `camera_name` field
  - Response includes `camera_name` in mapping objects
- **Frontend changes:**
  - Bounding box editor sends `camera_name` when creating mappings
  - Mapping editor preserves `camera_name` from loaded mappings

### Phase 12: Future Enhancements PLANNED
- **Auto-detection hooks** - Placeholder for ML/CV integration
- **Enhanced error handling** - Better user feedback for edge cases
- **Advanced verification** - OCR and template matching for device output validation

## Implementation Notes

### Changes from Original Specification

1. **Screenshot Storage:** Uses `~/.espbrew/verify/` instead of `~/.espbrew/screenshots/`
2. **CLI Flag Names:**
   - `mapping export` uses `--output` instead of `--format`
   - `mapping set` uses `--bounds <x,y,w,h>` comma-separated format
3. **Command Structure:** `capture verify` is a subcommand under `capture`, not standalone
4. **Workflow Command:** `espbrew workflow flash-and-verify` deferred to future implementation
5. **Verification Methods:** Only basic capture/extract implemented; OCR and template matching are future features

### Files Created/Modified

**New Files:**
- `internal/persistence/bbox.go` - BoundingBox struct and utilities
- `internal/persistence/mapping.go` - Mapping and calibration CRUD
- `internal/http/mapping.go` - API handlers
- `internal/dashboard/static/js/bbox-editor.js` - Canvas editor component
- `internal/camera/extract.go` - Device subimage extraction logic
- `cmd/espbrew/mapping.go` - Mapping CLI commands
- `cmd/espbrew/capture_verify.go` - Capture verify command
- `cmd/espbrew/capture_list.go` - Capture listing commands
- `cmd/espbrew/capture_get.go` - Device capture retrieval command

**Modified Files:**
- `internal/persistence/buckets.go` - Added bounding_boxes and calibrations buckets
- `internal/persistence/codec.go` - Added decoders for new types
- `internal/http/server.go` - Registered mapping routes
- `internal/dashboard/static/index.html` - Added mapping UI components

### Building and Running

```bash
# Build the project
go build ./cmd/espbrew

# Test mapping commands
./espbrew mapping list --device-id esp-aa:bb:cc:dd:ee:ff
./espbrew capture verify --device-id esp-aa:bb:cc:dd:ee:ff

# Access web UI
# Navigate to http://localhost:8080 and click "Device Mapping" tab
```

## Error Handling

| Error | Handling |
|-------|----------|
| Invalid bounds (x+w > 1) | Return 400, validation message |
| Camera position changed | Calibration version mismatch warning |
| Capture file deleted | Skip with warning, return partial results |
| Device offline | Skip extraction, continue with other devices |
| OCR/OpenCV unavailable | Graceful degradation, log warning |

## Security Considerations

- Path sanitization for screenshot storage
- Bounds validation (0-1 range check)
- Camera access controls (existing authentication)
- Device authorization checks

## Testing Strategy

**Unit Tests (Implemented):**
- `internal/persistence/bbox_test.go` - BoundingBox validation, coordinate conversion, round-trip precision
- `internal/persistence/mapping_test.go` - CRUD operations, indexing, calibration versioning
- `internal/http/mapping_test.go` - API handler tests (create, update, delete, list, calibration)

**Integration Tests (Future):**
- Full capture → extract → verify flow
- Multi-device screenshot extraction
- Calibration version handling

**Manual Tests:**
- Canvas drawing on mobile/desktop
- Touch gesture handling
- Camera position change scenario
