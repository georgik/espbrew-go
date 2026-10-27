package espflash

import (
	"context"
	"errors"
	"testing"
)

// TestIsNativeUSBJTAGClassifiesFamilies locks in which chip families are routed
// to the native USB-JTAG/Serial reset path (usbJTAGSerialReset) on the first
// connect attempt instead of the USB-UART-bridge DTR/RTS reset.
//
// The S3/C3/C5/C6/H2 families have a native USB-JTAG/Serial peripheral and must
// be treated as native. The P4 has its own dedicated reset (forceP4Download),
// the legacy ESP32/ESP8266 families use the classic bridge reset, and the
// unknown/auto case falls back to the bridge strategy — all of these are NOT
// routed to the native fast path.
func TestIsNativeUSBJTAGClassifiesFamilies(t *testing.T) {
	native := []ChipType{
		ChipESP32S3, ChipESP32C3, ChipESP32C5, ChipESP32C6, ChipESP32H2,
	}
	bridge := []ChipType{
		ChipESP32, ChipESP8266, ChipESP32P4, ChipESP32S2, ChipESP32C2, ChipAuto,
	}

	for _, chip := range native {
		if !isNativeUSBJTAG(chip) {
			t.Errorf("isNativeUSBJTAG(%v) = false, want true", chip)
		}
	}
	for _, chip := range bridge {
		if isNativeUSBJTAG(chip) {
			t.Errorf("isNativeUSBJTAG(%v) = true, want false", chip)
		}
	}
}

// TestResetAutoNativeUSBConnectsOnFirstAttempt verifies that a native USB-JTAG
// board (ESP32-S3) takes the USB-JTAG reset path on the FIRST connect attempt,
// not attempt 1.
//
// With ConnectAttempts=1 only attempt 0 runs. If the native fast path fires,
// connectViaUSBJTAG() reopens the port once even though sync fails — so
// closeCount must be 1. A pre-fix implementation would have used the DTR/RTS
// bridge reset on attempt 0 and only reached the USB-JTAG path on attempt 1,
// which with a single attempt would leave closeCount at 0 and add a whole
// failed sync cycle before connecting (the "0% connecting" window).
func TestResetAutoNativeUSBConnectsOnFirstAttempt(t *testing.T) {
	port := &recordingPort{}
	mc := &mockConnection{
		syncFunc: func() (uint32, error) { return 0, errors.New("no bootloader sync") },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := &Flasher{
		port:    port,
		conn:    mc,
		portStr: "/dev/ttyespbrew-test-absent",
		opts: &FlasherOptions{
			ResetMode:   ResetAuto,
			ResetBridge: false,       // native USB-JTAG board
			ChipType:    ChipESP32S3, // forced, as the leader sends it
			FastMode:    true,        // sync failure skips the loop's reopen,
			// so the ONLY close comes from connectViaUSBJTAG's reopenPort
			ConnectAttempts: 1, // only attempt 0 runs
			Logger:          &captureLogger{},
		},
		ctx:    ctx,
		cancel: cancel,
	}

	f.connect()

	if port.closeCount != 1 {
		t.Fatalf("native USB-JTAG path did not run on attempt 0: port closed %d time(s), want 1", port.closeCount)
	}
}

// TestResetAutoBridgeStaysOnDTRRTS confirms a UART-bridge board keeps the
// bridge reset on the first attempt and never touches the native USB-JTAG
// path, even if its ChipType happens to be a native family. The ResetBridge
// flag takes precedence over the family classification.
func TestResetAutoBridgeStaysOnDTRRTS(t *testing.T) {
	port := &recordingPort{}
	mc := &mockConnection{
		syncFunc: func() (uint32, error) { return 0, errors.New("no bootloader sync") },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f := &Flasher{
		port:    port,
		conn:    mc,
		portStr: "/dev/ttyespbrew-test-absent",
		opts: &FlasherOptions{
			ResetMode:       ResetAuto,
			ResetBridge:     true, // UART-bridge board
			ChipType:        ChipESP32S3,
			FastMode:        true,
			ConnectAttempts: 1,
			Logger:          &captureLogger{},
		},
		ctx:    ctx,
		cancel: cancel,
	}

	f.connect()

	if port.closeCount != 0 {
		t.Fatalf("bridge board must not run the native USB-JTAG path: port closed %d time(s), want 0", port.closeCount)
	}
}
