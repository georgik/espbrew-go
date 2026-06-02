# Hash-Based Flash Detection

## Overview

This feature adds intelligent flash detection to significantly reduce flashing time by skipping reflashing of unchanged firmware regions. The client computes and sends hashes of currently flashed firmware regions (bootloader, partition table, application). The server compares these with the job's expected hashes and instructs the client to only send regions that need updating.

## Problem Statement

Currently, the flashing process always sends the complete firmware binary from client to server, even if regions like the bootloader or partition table have not changed. This is inefficient during:

- Iterative development where only the application partition changes
- Bulk device flashing where bootloader/partition table are identical across devices
- Recovery operations where only specific regions need updating

A typical 4MB firmware image takes significant time to transfer and flash, even when only 256KB of application code has changed.

## Solution

### Protocol Flow

1. **Pre-flash Query** (Client → Server):
   ```
   POST /api/v1/devices/{deviceId}/flash-status
   {
     "device_id": "esp-aa:bb:cc:dd:ee:ff",
     "regions": [
       {"name": "bootloader", "offset": 4096, "size": 28672, "md5": "..."},
       {"name": "partition-table", "offset": 32768, "size": 4096, "md5": "..."},
       {"name": "application", "offset": 65536, "size": 4194304, "md5": "..."}
     ]
   }
   ```

2. **Server Response**:
   ```
   {
     "status": "partial_update",
     "regions_needed": [
       {"name": "application", "offset": 65536, "size": 4194304, "md5": "..."}
     ],
     "regions_cached": [
       {"name": "bootloader", "reason": "hash_match"},
       {"name": "partition-table", "reason": "hash_match"}
     ],
     "job_id": "job-123",
     "message": "Partial update available"
   }
   ```

3. **Conditional Flash**:
   - Client sends only regions marked in `regions_needed`
   - Server processes as normal flash but skips cached regions

### Status Values

- **full_flash**: No regions match hashes, complete flash required
- **partial_update**: Some regions match, only send changed regions
- **skip_all**: All regions match hashes, no flash needed

## API Endpoints

### Query Flash Status

**Endpoint:** `POST /api/v1/devices/{deviceId}/flash-status`

**Request:**
```json
{
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "regions": [
    {"name": "bootloader", "offset": 4096, "size": 28672, "md5": "0123456789abcdef0123456789abcdef"}
  ]
}
```

**Response:**
```json
{
  "status": "partial_update",
  "regions_needed": [...],
  "regions_cached": [{"name": "bootloader", "reason": "hash_match"}],
  "job_id": "job-123",
  "message": "Partial update available"
}
```

### Get Job Hashes

**Endpoint:** `GET /api/v1/jobs/{jobId}/hashes`

**Response:**
```json
{
  "job_id": "job-123",
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "regions": [...],
  "created_at": "2026-06-02T12:00:00Z"
}
```

### Update Job Hashes

**Endpoint:** `PUT /api/v1/jobs/{jobId}/hashes`

**Request:**
```json
{
  "device_id": "esp-aa:bb:cc:dd:ee:ff",
  "regions": [
    {"name": "bootloader", "offset": 4096, "size": 28672, "md5": "..."}
  ]
}
```

### Delete Job Hashes

**Endpoint:** `DELETE /api/v1/jobs/{jobId}/hashes`

## ESP32 Flash Layouts

### ESP32-S3 (4MB)

| Region | Offset | Size | Description |
|--------|--------|------|-------------|
| Bootloader | 0x1000 | 0x7000 (28KB) | ROM bootloader |
| Partition Table | 0x8000 | 0x1000 (4KB) | Partition table |
| OTA Select | 0x9000 | 0x2000 (8KB) | OTA selection |
| Application | 0x10000 | 0x400000 (4MB) | Firmware application |

### ESP32 (4MB)

Same layout as ESP32-S3.

### ESP32-C3 (4MB)

Same layout as ESP32-S3.

## Implementation

### Data Structures

```go
type FlashRegionInfo struct {
    Name   string `json:"name"`   // Region identifier
    Offset uint32 `json:"offset"` // Flash offset in bytes
    Size   uint32 `json:"size"`   // Region size in bytes
    MD5    string `json:"md5"`    // MD5 hash (hex encoded)
}

type FlashStatusResponse struct {
    Status         string             `json:"status"`
    RegionsNeeded  []FlashRegionInfo  `json:"regions_needed,omitempty"`
    RegionsCached  []CachedRegionInfo `json:"regions_cached,omitempty"`
    JobID          string             `json:"job_id,omitempty"`
    Message        string             `json:"message,omitempty"`
}
```

### Hash Computation

```go
import "codeberg.org/georgik/espbrew-go/internal/flashhash"

// Compute MD5 for a region from memory
hash, err := flashhash.ComputeRegionMD5(data, offset, size)

// Compute MD5 for a region from file
hash, err := flashhash.ComputeRegionMD5FromFile(path, offset, size)

// Compute all region hashes
hashes, err := flashhash.ComputeAllRegionsMD5FromFile(path, regions)
```

### Region Comparison

```go
// Compare client regions with job regions
needed, cached := flashhash.CompareRegions(clientRegions, jobRegions)

// Find missing regions (in job but not on client)
missing := flashhash.MergeRegions(clientRegions, jobRegions)
```

## Storage

Flash hashes are stored in BoltDB under the `flash_hashes` bucket. Each job stores its expected region hashes for comparison during subsequent flashes.

### Schema

```
flash_hashes/
├── hash-{job_id} -> JobFlashHashes JSON
```

## Performance

Expected performance improvements:

- **Hash computation**: ~100MB/s on modern hardware
- **4MB image hashing**: <50ms
- **Network transfer savings**: Up to 90% for application-only changes
- **Flash time savings**: Proportional to regions skipped

Example for 4MB firmware with only application changed:
- Full flash: 4MB transfer + full flash time
- Optimized: ~256KB transfer + application flash only
- Savings: ~93% transfer time, ~90% flash time

## Security Considerations

- Hash collisions: MD5 is sufficient for firmware integrity (not cryptographic security)
- Client must validate server responses
- Server logs all partial flash operations for audit
- Hash comparison is deterministic and reproducible

## Backward Compatibility

Feature is opt-in. Existing flash workflows continue unchanged:
- Clients not sending hash queries get full flash behavior
- No breaking changes to existing API
- Server accepts both full and partial flashes

## Files

- `internal/flashhash/region.go` - Data structures and layouts
- `internal/flashhash/hash.go` - Hash computation utilities
- `internal/flashhash/region_test.go` - Region tests
- `internal/flashhash/hash_test.go` - Hash computation tests
- `internal/persistence/flash_hashes.go` - Hash storage
- `internal/persistence/flash_hashes_test.go` - Storage tests
- `internal/http/flash_status.go` - API endpoints
- `internal/cluster/peer_hash.go` - Peer protocol hash integration
- `internal/cluster/peer_hash_test.go` - Peer protocol tests
- `internal/cluster/peer.go` - Peer node implementation
- `internal/cluster/leader.go` - Leader node with hash processing
- `pkg/protocol/messages.go` - Protocol message definitions

## Usage Example

### Server-side (after flash completion)

```go
// Store job hashes after successful flash
hashes := &flashhash.JobFlashHashes{
    JobID:    job.ID,
    DeviceID: device.ID,
    Regions: []flashhash.FlashRegionInfo{
        {Name: "bootloader", Offset: 0x1000, Size: 0x7000, MD5: bootloaderHash},
        {Name: "application", Offset: 0x10000, Size: 0x400000, MD5: appHash},
    },
    CreatedAt: time.Now().Format(time.RFC3339),
}
store.SaveFlashHashes(hashes)
```

### Client-side (before flash)

```go
// 1. Compute current flash hashes
regions := flashhash.StandardESP32S3Layout4MB()
currentHashes, _ := flashhash.ComputeAllRegionsMD5FromFile("/dev/ttyUSB0", regions)

// 2. Query server status
req := flashhash.FlashStatusRequest{
    DeviceID: deviceID,
    Regions:  currentHashes,
}
resp := queryFlashStatus(deviceID, req)

// 3. Conditionally flash
if resp.Status == "skip_all" {
    log.Println("All regions match, skipping flash")
    return
}

if resp.Status == "partial_update" {
    log.Printf("Only flashing %d regions", len(resp.RegionsNeeded))
    regions = resp.RegionsNeeded
}

// Flash only needed regions
flashDevice(deviceID, regions)
```

## Testing

Run tests:

```bash
go test ./internal/flashhash/...
go test ./internal/persistence -run FlashHashes
```

Test coverage includes:
- Hash computation correctness
- Region validation
- Comparison logic
- Storage operations
- API endpoints

## Peer Protocol Integration

The hash-based flash feature integrates with the cluster peer protocol to enable distributed flash optimization across multiple nodes. Peers can query hash information from the leader to determine which regions need flashing.

### Message Types

The protocol defines the following message types for hash operations:

```go
// Message types
MsgFlashHashQuery    MessageType = "FlashHashQuery"
MsgFlashHashResponse MessageType = "FlashHashResponse"

// Request payload
type FlashHashQuery struct {
    DeviceID   string                      `json:"device_id"`
    JobID      string                      `json:"job_id,omitempty"`
    Regions    []flashhash.FlashRegionInfo `json:"regions"`
    ClientMode string                      `json:"client_mode"`
}

// Response payload
type FlashHashResponse struct {
    Status            string                       `json:"status"`
    RegionsNeeded     []flashhash.FlashRegionInfo  `json:"regions_needed,omitempty"`
    RegionsCached     []flashhash.CachedRegionInfo `json:"regions_cached,omitempty"`
    JobID             string                       `json:"job_id,omitempty"`
    Message           string                       `json:"message,omitempty"`
    RecommendedAction string                       `json:"recommended_action"`
}
```

### Peer Hash Query Flow

The peer-to-leader hash query follows this sequence:

1. **Peer computes device hashes**: The peer node reads current flash hashes from the device (or retrieves from cache)

2. **Peer sends query to leader**: The peer sends a `FlashHashQuery` message via HTTP POST to the leader's `/api/v1/devices/{deviceId}/flash-status` endpoint

3. **Leader processes query**: The leader compares the provided hashes against stored job hashes

4. **Leader returns optimization response**: The leader responds with regions that need flashing and regions that can be skipped

```go
// Peer-side query
response, err := cluster.QueryFlashHash(client, deviceID, currentHashes)
if err != nil {
    log.Warn().Err(err).Msg("Hash query failed, falling back to full flash")
    // Fall back to full flash
}
```

### Leader Hash Processing

The leader node processes hash queries from peers through the following steps:

1. **Receive hash data via heartbeat**: Devices include `FlashHashes` in their heartbeat payload, which the leader stores under `device-{deviceID}` job ID

2. **Compare with job hashes**: When a hash query arrives, the leader compares the peer-provided hashes with the expected job hashes

3. **Return optimization result**: The leader returns the appropriate status and region lists

```go
// Leader-side heartbeat processing
func (l *LeaderNode) UpdateHeartbeat(nodeID string, payload *protocol.HeartbeatPayload) {
    // Process devices with flash hash data
    for _, dev := range payload.Devices {
        if dev.FlashHashes != nil && dev.DeviceID != "" {
            l.processDeviceFlashHashes(dev.DeviceID, dev.FlashHashes)
        }
    }
}

// Leader stores device hashes
func (l *LeaderNode) processDeviceFlashHashes(deviceID string, hashes *protocol.DeviceFlashHashes) {
    jobHashes := &flashhash.JobFlashHashes{
        JobID:     "device-" + deviceID,
        DeviceID:  deviceID,
        Regions:   hashes.Regions,
        CreatedAt: hashes.UpdatedAt,
    }
    l.store.SaveFlashHashes(jobHashes)
}
```

### Code Examples

#### Peer Client Example

```go
import "codeberg.org/georgik/espbrew-go/internal/cluster"

// Create cluster client
client := cluster.NewClient("http://leader-node:8080")

// Compute current device hashes
regions := flashhash.StandardESP32S3Layout4MB()
currentHashes, err := cluster.ComputeDeviceHashes("/dev/ttyUSB0", "esp32s3")

// Query leader for optimization
response, err := cluster.QueryFlashHash(client, deviceID, currentHashes)
if err != nil {
    log.Warn().Err(err).Msg("Hash query failed, using full flash")
    // Fall back to full flash
}

// Process response
needed, cached, err := cluster.ProcessFlashHashResponse(response)

switch {
case len(needed) == 0 && len(cached) > 0:
    log.Info().Msg("All regions cached, skipping flash")
    return
case len(needed) < len(currentHashes):
    log.Printf("Partial update: flashing %d of %d regions", len(needed), len(currentHashes))
default:
    log.Info().Msg("Full flash required")
}

// Flash only needed regions
for _, region := range needed {
    flashRegion(devicePath, region)
}
```

#### Leader API Endpoint

```go
// HTTP handler on leader node
func (h *FlashAPI) HandleFlashStatus(w http.ResponseWriter, r *http.Request) {
    deviceID := mux.Vars(r)["deviceId"]

    var req flashhash.FlashStatusRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Get job hashes for comparison
    jobHashes, err := h.store.GetFlashHashes(req.JobID)
    if err != nil {
        // No hashes found, require full flash
        json.NewEncoder(w).Encode(flashhash.FlashStatusResponse{
            Status:    "full_flash",
            Message:   "No cached hashes found",
            JobID:     req.JobID,
        })
        return
    }

    // Compare regions
    needed, cached := flashhash.CompareRegions(req.Regions, jobHashes.Regions)

    status := "full_flash"
    if len(needed) == 0 && len(cached) > 0 {
        status = "skip_all"
    } else if len(cached) > 0 {
        status = "partial_update"
    }

    json.NewEncoder(w).Encode(flashhash.FlashStatusResponse{
        Status:        status,
        RegionsNeeded: needed,
        RegionsCached: cached,
        JobID:         req.JobID,
    })
}
```

#### Device Heartbeat with Hashes

```go
// Peer includes hash data in heartbeat
func (p *PeerNode) sendHeartbeat() {
    devices := make([]*protocol.DeviceInfo, 0)

    for path, dev := range p.state.Devices {
        // Compute and include flash hashes
        regions, err := cluster.ComputeDeviceHashes(path, dev.ChipType)
        if err == nil {
            dev.FlashHashes = &protocol.DeviceFlashHashes{
                DeviceID:  dev.DeviceID,
                Regions:   regions,
                UpdatedAt: time.Now().Format(time.RFC3339),
            }
        }
        devices = append(devices, dev)
    }

    payload := &protocol.HeartbeatPayload{
        NodeID:      p.id,
        DeviceCount: len(devices),
        Devices:     devices,
    }

    p.sendHeartbeatHTTP(payload)
}
```

### Hash Cache on Peers

Peers maintain a local cache of computed device hashes to avoid repeated device reads:

```go
type HashCache struct {
    hashes     []flashhash.FlashRegionInfo
    devicePath string
    chip       string
    expiry     time.Time
}

// Cache TTL
const defaultCacheTTL = 5 * time.Minute

// Invalidate cache after flash operations
func InvalidateHashCache(devicePath, chip string) {
    globalHashCache.Invalidate(devicePath, chip)
}
```

### Performance Considerations

- **Network overhead**: Hash query adds one HTTP round-trip before flashing
- **Cache hit rate**: High cache hit rates significantly reduce flash time
- **Hash computation**: Cached locally on peers with 5-minute TTL
- **Leader storage**: Stores device hashes under `device-{deviceID}` key for quick lookup

### Error Handling

Peers fall back to full flash operation if:

- Hash query fails (network error, timeout)
- Leader returns error response
- Hash computation fails on device
- Cache is stale and device read fails

```go
response, err := cluster.QueryFlashHash(client, deviceID, regions)
if err != nil {
    log.Warn().Err(err).Msg("Hash query failed, falling back to full flash")
    // Fall back to full flash
    flashAllRegions(devicePath, regions)
    return
}
```

### Integration Summary

The peer protocol hash extension provides:

1. **Distributed optimization**: Peers can query hash information without local storage
2. **Leader aggregation**: Leader collects device hashes from heartbeats and provides optimization recommendations
3. **Cache efficiency**: Local hash caching on peers reduces device reads
4. **Graceful degradation**: Automatic fallback to full flash on errors
5. **Scalability**: Single leader serves hash queries for multiple peers

## Files

- `internal/flashhash/region.go` - Data structures and layouts
- `internal/flashhash/hash.go` - Hash computation utilities
- `internal/flashhash/region_test.go` - Region tests
- `internal/flashhash/hash_test.go` - Hash computation tests
- `internal/persistence/flash_hashes.go` - Hash storage
- `internal/persistence/flash_hashes_test.go` - Storage tests
- `internal/http/flash_status.go` - API endpoints
- `internal/cluster/peer_hash.go` - Peer protocol hash integration
- `internal/cluster/peer_hash_test.go` - Peer protocol tests
- `internal/cluster/peer.go` - Peer node implementation
- `internal/cluster/leader.go` - Leader node with hash processing
- `pkg/protocol/messages.go` - Protocol message definitions
