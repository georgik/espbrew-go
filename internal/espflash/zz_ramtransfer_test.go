package espflash

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// TestZzFlashPathReliability connects (current P4 default) then runs the full
// flashBegin/flashData/flashEnd command->data->verify SLIP path, the exact path
// that surfaces in the logs as "attach SPI flash timeout" when the chip drops.
func TestZzFlashPathReliability(t *testing.T) {
	requireHardware(t)
	const reps = 6
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 2
	base.BaudRate = 115200

	dist := map[string]int{}
	var last string

	for i := 0; i < reps; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		f, err := New(ctx, hardwarePort(), base)
		if err != nil {
			dist["connect-fail"]++
			cancel()
			time.Sleep(120 * time.Millisecond)
			continue
		}

		var phase string
		berr := f.conn.flashBegin(64, 0x10000, false, false)
		switch {
		case berr != nil:
			phase = "flashBegin"
		default:
			block := bytes.Repeat([]byte{0xAB, 0xCD, 0x11, 0x22}, 32)
			derr := f.conn.flashData(block, 0)
			if derr != nil {
				phase = "flashData"
			} else {
				phase = "flashEnd"
				_ = f.conn.flashEnd(false)
			}
		}
		f.Close()
		cancel()
		dist[phase]++
		last = phase
		time.Sleep(120 * time.Millisecond)
	}
	t.Logf("flash-path phase distribution: %v", dist)
	t.Logf("last phase: %s", last)
}
