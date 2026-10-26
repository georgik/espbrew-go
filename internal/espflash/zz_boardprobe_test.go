package espflash

import (
	"context"
	"testing"
	"time"
)

// TestZzConnectSweep measures the real connect() success rate for each reset
// strategy. connect() succeeds only if the chip enters download mode and syncs,
// so this directly measures which reset makes flashing possible.
func TestZzConnectSweep(t *testing.T) {
	const reps = 3
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 2
	base.BaudRate = 115200

	configs := map[string]*FlasherOptions{
		"Auto+Bridge=false (current P4 default)": func() *FlasherOptions {
			o := *base
			o.ResetMode = ResetAuto
			o.ResetBridge = false
			return &o
		}(),
		"USBJTAG": func() *FlasherOptions {
			o := *base
			o.ResetMode = ResetUSBJTAG
			o.ResetBridge = false
			return &o
		}(),
		"Default(classic)": func() *FlasherOptions {
			o := *base
			o.ResetMode = ResetDefault
			o.ResetBridge = false
			return &o
		}(),
	}

	for name, opts := range configs {
		ok := 0
		fails := map[string]int{}
		for i := 0; i < reps; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			f, err := New(ctx, "/dev/ttyACM0", opts)
			if err == nil {
				f.Close()
				ok++
			} else {
				fails[err.Error()]++
			}
			cancel()
			time.Sleep(100 * time.Millisecond)
		}
		t.Logf("[%s] success=%d/%d  fails=%v", name, ok, reps, fails)
	}
}
