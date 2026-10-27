package espflash

import (
	"context"
	"testing"
	"time"
)

// TestZzRealFlowVars tests FastMode and re-open reuse — variables present in the
// real flow but not in the plain connect sweep.
func TestZzRealFlowVars(t *testing.T) {
	requireHardware(t)
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 2
	base.BaudRate = 115200

	// FastMode
	fm := *base
	fm.FastMode = true
	fm.ConnectAttempts = 2
	fastOk, fastTotal := 0, 6
	for i := 0; i < fastTotal; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		f, err := New(ctx, hardwarePort(), &fm)
		cancel()
		if err == nil {
			f.Close()
			fastOk++
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("FastMode connect success=%d/%d", fastOk, fastTotal)

	// Reuse one flasher: connect, attach, connect again on same port (no reopen)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	f, err := New(ctx, hardwarePort(), base)
	cancel()
	if err != nil {
		t.Logf("reuse: first connect failed: %v", err)
	} else {
		a1 := f.attachFlash()
		a2 := f.attachFlash()
		t.Logf("reuse: attach#1=%v attach#2=%v", a1 == nil, a2 == nil)
		f.Close()
	}
}
