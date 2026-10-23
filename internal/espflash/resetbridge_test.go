package espflash

import (
	"context"
	"errors"
	"testing"
)

// TestResetBridgeForcesDTRRTSResetSkipsUSBJS verifies the ResetBridge path that
// the flash layer sets for USB-UART-bridge boards (CH340, CH9102, CP210x,
// FTDI) — e.g. the waveshare ESP32-S3 that enumerates as 1a86:55d3.
//
// Such a board has NO native USB-JTAG peripheral, so the bootloader reset must
// be driven purely with the atomic DTR/RTS sequence, and the USB-JTAG/Serial
// path (which closes+reopens the port on the assumption the device
// re-enumerated) must never run. With a mock connection that never syncs,
// connect() returns a SyncError; the side effects are what we assert on:
//
//   - closeCount stays 0: the only code path that closes the fd is reopenPort(),
//     which the USB-JTAG path calls. A zero count proves that path did not run.
//   - the DTR/RTS history shows the tight-reset bootloader transition.
//   - probeBootLog ran (boot-log verification step is present).
func TestResetBridgeForcesDTRRTSResetSkipsUSBJS(t *testing.T) {
	port := &recordingPort{}
	mc := &mockConnection{
		// Never syncs, so connect() exhausts its attempts and returns a
		// SyncError. The assertions below are on the reset side effects.
		syncFunc: func() (uint32, error) { return 0, errors.New("no bootloader sync") },
	}
	logger := &captureLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := &Flasher{
		port:    port,
		conn:    mc,
		portStr: "/dev/ttyACM5",
		opts: &FlasherOptions{
			ResetMode:   ResetAuto,
			ResetBridge: true,
			FastMode:    true, // on sync failure, skip the port reopen so
			// closeCount reflects ONLY the (absent) USB-JTAG path
			ConnectAttempts: 1,
			Logger:          logger,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	if err := f.connect(); err == nil {
		t.Log("connect() unexpectedly succeeded against the no-sync mock")
	}

	// 1. The USB-JTAG path must never have run. connectViaUSBJTAG() reconnects
	//    by calling reopenPort(), which closes the fd. A non-zero closeCount is
	//    the tell-tale sign that path executed on a board that never disconnects.
	if port.closeCount != 0 {
		t.Fatalf("USB-JTAG path ran unexpectedly: port closed %d time(s), want 0 (DTR/RTS reset only)", port.closeCount)
	}

	// 2. The atomic DTR/RTS (tight) reset must have driven the bootloader
	//    transition: GPIO0 (DTR) pulled LOW (true) together with EN (RTS)
	//    pulled LOW (false), exactly as unixTightReset does for a bridge.
	dtrTrueIdx := -1
	for i, v := range port.dtrCalls {
		if v && dtrTrueIdx == -1 {
			dtrTrueIdx = i
		}
	}
	rtsFalseIdx := -1
	for i, v := range port.rtsCalls {
		if !v && rtsFalseIdx == -1 {
			rtsFalseIdx = i
		}
	}
	if dtrTrueIdx == -1 || rtsFalseIdx == -1 {
		t.Fatalf("DTR/RTS reset never reached bootloader state (dtrTrueIdx=%d, rtsFalseIdx=%d): %v", dtrTrueIdx, rtsFalseIdx, port.calls)
	}

	// 3. probeBootLog must have been invoked (the boot-log verification step
	//    that confirms the chip actually entered download mode). With the
	//    recordingPort emitting no bytes the verdict is the empty-log message.
	if !logger.has("boot log") {
		t.Fatalf("expected probeBootLog to run, got: %v", logger.msgs)
	}
}

// TestResetBridgeOffStillTriesUSBJS is the contrast: with ResetBridge=false the
// board is treated as a native USB-JTAG device, so the auto path reaches the
// USB-JTAG/Serial reset (attempt%3==1) and reconnects by closing the fd. This
// guards against the ResetBridge flag being ignored.
//
// FastMode=true means sync failure does NOT reopen the port, so the single
// close we observe can only come from connectViaUSBJTAG's reopenPort.
// ConnectAttempts=2 lets attempt 1 (1%3==1) reach the USB-JTAG branch.
func TestResetBridgeOffStillTriesUSBJS(t *testing.T) {
	port := &recordingPort{}
	mc := &mockConnection{
		syncFunc: func() (uint32, error) { return 0, errors.New("no bootloader sync") },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := &Flasher{
		port:    port,
		conn:    mc,
		portStr: "/dev/ttyACM0",
		opts: &FlasherOptions{
			ResetMode:   ResetAuto,
			ResetBridge: false, // native USB-JTAG board
			FastMode:    true,  // sync failure skips reopen, so the ONLY close
			// comes from connectViaUSBJTAG's reopenPort
			ConnectAttempts: 2, // attempt 1 (1%3==1) reaches the USB-JTAG branch
			Logger:          &captureLogger{},
		},
		ctx:    ctx,
		cancel: cancel,
	}

	f.connect()

	// With ResetBridge off, the auto path reaches the USB-JTAG reset on attempt
	// 1, which reopens the port. This is exactly the disruptive reconnect we
	// suppress for a bridge board (see the ResetBridge test above).
	if port.closeCount == 0 {
		t.Fatalf("expected the USB-JTAG path to reopen the port (reopenPort), closeCount=0")
	}
}
