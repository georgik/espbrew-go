package espflash

import (
	"context"
	"testing"
	"time"
)

// TestZzAttachReliability connects (current P4 default ResetAuto) and then issues
// the SPI attach command, the exact operation that times out in the logs.
func TestZzAttachReliability(t *testing.T) {
	requireHardware(t)
	const reps = 6
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 2
	base.BaudRate = 115200

	ok := 0
	attachFails := map[string]int{}
	for i := 0; i < reps; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		f, err := New(ctx, hardwarePort(), base)
		if err != nil {
			attachFails["connect:"+err.Error()]++
			cancel()
			time.Sleep(120 * time.Millisecond)
			continue
		}
		aerr := f.attachFlash()
		f.Close()
		cancel()
		if aerr == nil {
			ok++
		} else {
			attachFails["attach:"+aerr.Error()]++
		}
		time.Sleep(120 * time.Millisecond)
	}
	t.Logf("attach success=%d/%d  fails=%v", ok, reps, attachFails)
}
