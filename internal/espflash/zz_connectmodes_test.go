package espflash

import (
	"context"
	"testing"
	"time"
)

// TestZzConnectModes measures full connect() success per ResetMode.
func TestZzConnectModes(t *testing.T) {
	requireHardware(t)
	const reps = 4
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 2
	base.BaudRate = 115200

	configs := map[string]*FlasherOptions{
		"ResetDefault":           func() *FlasherOptions { o := *base; o.ResetMode = ResetDefault; o.ResetBridge = false; return &o }(),
		"ResetUSBJTAG":           func() *FlasherOptions { o := *base; o.ResetMode = ResetUSBJTAG; o.ResetBridge = false; return &o }(),
		"ResetAuto+Bridge=false": func() *FlasherOptions { o := *base; o.ResetMode = ResetAuto; o.ResetBridge = false; return &o }(),
		"ResetAuto+Bridge=true":  func() *FlasherOptions { o := *base; o.ResetMode = ResetAuto; o.ResetBridge = true; return &o }(),
	}
	for name, opts := range configs {
		ok := 0
		fails := map[string]int{}
		for i := 0; i < reps; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			f, err := New(ctx, hardwarePort(), opts)
			if err == nil {
				f.Close()
				ok++
			} else {
				fails[err.Error()]++
			}
			cancel()
			time.Sleep(120 * time.Millisecond)
		}
		t.Logf("[%s] success=%d/%d fails=%v", name, ok, reps, fails)
	}
}
