package espflash

import (
	"context"
	"testing"
	"time"
)

// TestZzPostPowerCycle connects after a real hub power-cycle to confirm the
// board is fully responsive on a cold boot (the "does it talk to us" check).
func TestZzPostPowerCycle(t *testing.T) {
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 3
	base.BaudRate = 115200
	ok, total := 0, 5
	for i := 0; i < total; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		f, err := New(ctx, "/dev/ttyACM0", base)
		cancel()
		if err == nil {
			f.Close()
			ok++
		} else {
			t.Logf("  conn %d failed: %v", i, err)
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Logf("post-power-cycle connect success=%d/%d", ok, total)
}
