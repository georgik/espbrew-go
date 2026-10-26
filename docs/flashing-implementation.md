# Flashing Implementation

This document describes the internal implementation of the flashing system, including fixes for common issues and platform-specific considerations.

## Download-Mode Confirmation (Boot-Log Parsing)

Forcing the chip into bootloader mode is only half the job — we must confirm the
reset actually landed it in *download* mode and not let it fall through to the
user application. espflash (the Rust reference tool) reads the serial line
immediately after the reset and matches the pattern

```
boot:(0x<addr>)([\s\S]*waiting for download)?
```

- `boot:0x...` **without** `waiting for download` → the chip booted the user app
  (`WrongBootMode`); the reset missed the download window and we must retry.
- `boot:0x...` **with** `waiting for download` → the chip is in download mode and
  ready to take a flash.

Our tool has a lighter version of this in `probeBootLog()` / `readBootLog()`
(`internal/espflash/flasher.go`): after the reset it captures the serial output
and logs whether it saw `waiting for download`. The sync loop is the real gate —
not seeing the banner is not fatal — but a marginal reset now surfaces early as a
clear signal instead of a late, confusing SLIP timeout.

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

**Problem**: ResetAuto was trying classic reset first for USB-JTAG devices, which doesn't work. This wasted attempts and caused timeouts. Additionally, devices remained in download mode after flashing completed.

**Solution**: For USB CDC ports, use USB-JTAG reset immediately in ResetAuto mode. Implemented correct reset sequence that clears the download mode flag before triggering reset:

1. DTR=false (GPIO0=HIGH for normal boot)
2. RTS=false (clears download mode flag) ← **Critical step**
3. Sleep 100ms
4. RTS=true (triggers reset)
5. Sleep 100ms
6. RTS=false (exit reset)

**Impact**: Connection is faster and more reliable for USB-JTAG/Serial devices. Devices now properly boot to user application after flashing instead of remaining in download mode.

**Reference**: [ESP32-S3 Technical Reference Manual](https://documentation.espressif.com/projects/esp32s3/en/latest/esp32s3/usb-serial-jtag/usb-serial-jtag-console.html) - The USB-Serial/JTAG controller expects specific RTS/DTR sequences to enter/exit download mode.

### Transient Port Busy (EBUSY) on Open

**Problem**: `New()` opened the serial port exactly once. When the port was
briefly held by another caller — the startup boot-log probe
(`inventory.ProbeFromBootLog`), or the USB-CDC node itself during the split
second after the probe's DTR reset re-enumerates the device — the open failed
with `Serial port busy`. A flash that happened to land in that window failed
outright, and in a cluster the board could stay blocked until the next manual
retry.

**Solution**: `New()` now opens via `openPortWithRetries()` in
`internal/espflash/flasher.go`. It retries only while the error is a transient
busy condition — recognised as `*serial.PortError` with `code == PortBusy`
(`go.bug.st/serial` reports this as `"Serial port busy"`, with a
`syscall.EBUSY` fallback). Permission, no-such-device and similar errors fail
immediately, so the retry never masks a real access problem. The retry is
bounded by 8 attempts × 500ms (~4s) and is context-aware: a cancelled/expired
job stops retrying instead of spinning.

**Impact**: A flash no longer fails if the boot-log probe is running or if the
USB device detaches and re-enumerates mid-operation — the open waits out the
busy window and the flash proceeds, so the board is never left blocked.

## Reservation Lock-Coupling (Single Owner Across Flash + Monitor)

**Problem.** A device can be driven by two clients at once: a `flash` job and a
`monitor` session (or a re-flashing client). Each of these opens the same serial
port, and the OS grants an exclusive open — the second `serialheal.Open` fails
with `Serial port busy`. The cluster models this with a `DeviceRegistry` /
`DeviceLock` (states `available` / `reserved` / `busy` / `error`, keyed on the
`/dev/serial/by-id/…` path). A client calls `Reserve(path, owner)` before it
touches the port and `Release(path, owner)` when it is done.

The first attempt held the reservation from **both** ends — the client reserved
over HTTP *and* the server reserved when it opened the port. Two owners meant a
race: a `flash` retry could submit while the previous job still held the
reservation, and the server answered with `409 device not available`. A monitor
opened right after a flash could also win the race against a still-unreleased
flash job.

**Solution (lock coupling).** The reservation is now held by exactly **one**
owner for the whole lifetime of the OS serial handle, and released the instant
that handle closes. This mirrors the OS mutex itself: `serialheal.Open` is an
exclusive open, so the reservation must be coupled to it.

- **Flash job** (`internal/cluster/leader.go`, `EnqueueJobWithOffsetAndErase`):
  the server reserves with `owner = job.ID` right before flashing and releases it
  in `handleJobResults` when the job reaches a terminal state. A concurrent
  submit returns `409 device not available: <path>` (owner reported) instead of
  opening a second handle.
- **Monitor session** (`internal/monitor/stream.go`, `Start()`): the monitor
  reserves `path` → `id` *before* `serialheal.Open`, and releases it immediately
  if the open fails. It never holds the reservation without an open handle.
- **Client** (`cmd/espbrew/monitor.go`): the client-side HTTP reservation was
  **removed** — the client no longer reserves before connecting. The server
  session is the sole holder, so there is only ever one owner to reason about.

Because the reservation and the OS exclusive open are now the same lifecycle, two
clients can never both hold the device, and a monitor can attach the moment a
flash job releases the port.

### The 409 flash-retry race (client side)

Even with a single owner, a **retrying** client could still hit `409`: the
`flash` command re-submits the job on certain transient errors, but if the
previous submit is still running it still holds the reservation, so the retry's
`SubmitFlash` fails with `device not available` immediately.

**Solution** (`cmd/espbrew/flash.go`): the retry loop tracks the most recent
`jobID`. When a submit fails with `device not available` and there is a
previously submitted job, it polls `GetJob(jobID)` until that job reaches a
terminal state (`complete` / `failed` / `cancelled` / `timedout`) — which is
exactly when the server releases the reservation — then retries the submit
**once**. It never spins re-submitting against a still-held device, and it never
waits longer than `jobReleaseTimeout` (3 min), polling every `jobReleasePollInterval`
(2 s).

## Secure Download Mode (SDM) and the Wedged-Chip Scenario

### Secure Download Mode

Some chips ship with **Secure Download Mode** enabled. SDM is a hardware/efuse
state that restricts certain bootloader operations (notably register writes and
flash reads). When SDM is active:

- espflash prints `WARN Secure Download Mode is enabled on this chip` and
  disables `--verify` / `--skip` (flash reads are restricted).
- The stub loader is not used; flashing falls back to the ROM bootloader.
- Chip identification relies on `GetSecurityInfo`, and on older chips on a
  fallback `ReadReg(0x40001000)` magic-register read.

We detect SDM the same way. `detectChip()` (`internal/espflash/flasher.go`)
reads security info and sets `chipDef.SecureDownloadMode`; the P4 target uses it
to select the correct revision-aware bootloader (v0 vs v3).

### The wedged chip

The most alarming failure mode we have seen is a **stable, wedged chip**:

- The USB-JTAG/Serial peripheral still **enumerates** (`/dev/ttyACM0` present,
  `lsusb` shows the `303a:1001` device, no bus errors).
- The CPU, however, is hung and emits **no serial output at all** and **no SLIP
  response**. `monitor` shows nothing; `board-info` / `flash` hang on
  "Connecting…".
- `ttyACM0`'s mtime is **stable** (unlike the ~7–8 s re-enumeration churn our
  reset calls produce), ruling out a reboot loop.

**Root cause:** a partial/corrupt application at `0x10000` (e.g. a flash that was
interrupted) crashes the CPU on boot. The chip never recovers on its own and
refuses the SLIP handshake. The USB peripheral stays up, which is why a plain
power cycle does not clear it.

**Recovery:** the chip must be pulled into download mode *bypassing* the app and
then given a valid image. `espflash`'s `UsbJtagSerialReset` reliably forces
download mode (its debug log shows `Download Mode: true` right after the reset),
so a power-cycle followed immediately by an `espflash flash` (or `write-bin` to a
clean image) is the way back. A wedged CPU only clears on a hard EN reset or
power cycle — software resets alone are insufficient.

### Why espflash connects where our tool hung

We byte-compared the two reset implementations and they are equivalent, so the
gap is in the connect *flow*, not the reset pulses:

1. espflash uses `UsbJtagSerialReset` as the **primary** reset for native USB
   boards and syncs immediately from a **single** port handle.
2. It parses the boot log to confirm download mode before trusting the sync.
3. Chip identification (`detect_chip`) falls back to `ReadReg(0x40001000)` when
   `GetSecurityInfo` yields no chip id; on a wedged/SDM chip that read fails
   intermittently, which is where espflash's connect loop stalls.

Our `connectViaUSBJTAG()` reopens the port (`reopenPort()`) between the reset and
the sync loop. On a marginal chip that reopen can drop the connection right as
the chip is finishing its USB re-enumeration, so the sync misses. The lesson:
**reset once, sync several times from the same handle, and let the boot log tell
us whether the reset won** — rather than reopening and re-resetting on every
attempt.

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

1. **Open serial port** - Open with configured baud rate, retrying briefly if the
   port is momentarily busy (see [Transient Port Busy (EBUSY) on Open](#transient-port-busy-ebusy-on-open))
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
