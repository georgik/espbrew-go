package espflash

import (
	"context"
	"testing"
	"time"
)

// TestZzRepeatedAttach connects ONCE and issues attachFlash repeatedly, like a
// multi-image flash that re-attaches per part, to see whether attach degrades.
func TestZzRepeatedAttach(t *testing.T) {
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 2
	base.BaudRate = 115200

	for trial := 0; trial < 3; trial++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		f, err := New(ctx, "/dev/ttyACM0", base)
		cancel()
		if err != nil {
			t.Logf("trial %d: connect failed: %v", trial, err)
			continue
		}
		res := []bool{}
		for i := 0; i < 4; i++ {
			res = append(res, f.attachFlash() == nil)
		}
		t.Logf("trial %d: attach results across 4 calls = %v", trial, res)
		f.Close()
		time.Sleep(150 * time.Millisecond)
	}
}
