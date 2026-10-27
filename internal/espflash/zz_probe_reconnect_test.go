package espflash

import (
	"context"
	"testing"
	"time"
)

// TestZzProbeReconnect probes whether the chip can be reached from its current
// (post-app-boot) state, and which reset strategy wins. It runs one connect()
// per ResetMode and reports the successful attempt index. This is the recovery
// probe: if USB-JTAG wins while DTR/RTS (ResetAuto attempt 0) loses, that
// tells us the native USB-JTAG reset must be the PRIMARY strategy.
func TestZzProbeReconnect(t *testing.T) {
	requireHardware(t)
	modes := []struct {
		name string
		mode ResetMode
	}{
		{"ResetAuto", ResetAuto},
		{"ResetUSBJTAG", ResetUSBJTAG},
		{"ResetDefault", ResetDefault},
	}
	for _, m := range modes {
		LastConnectAttempts = nil
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		f, err := New(ctx, hardwarePort(), &FlasherOptions{
			BaudRate: 115200, ResetMode: m.mode, ConnectAttempts: 9, Logger: &captureLogger{},
		})
		if err != nil {
			t.Logf("%s: connect FAILED after all attempts: %v", m.name, err)
			cancel()
			time.Sleep(200 * time.Millisecond)
			continue
		}
		attempt := f.LastConnectAttempt
		_ = f.Close()
		cancel()
		t.Logf("%s: CONNECTED on attempt %d (LastConnectAttempts=%v)", m.name, attempt, LastConnectAttempts)
		time.Sleep(200 * time.Millisecond)
	}
}
