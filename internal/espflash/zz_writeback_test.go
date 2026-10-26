package espflash

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"
	"time"
)

// TestZzWritebackFlips connects, writes 64 bytes to the factory app offset
// (0x10000) via the stub flash path, then reads them back and checks the first
// byte (the image magic 0xE9). This is the definitive "does the write persist"
// test that the boot log is failing.
func TestZzWritebackFlips(t *testing.T) {
	const reps = 6
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 2
	base.BaudRate = 115200

	magicOK := 0
	dist := map[string]int{}
	for i := 0; i < reps; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		f, err := New(ctx, "/dev/ttyACM0", base)
		if err != nil {
			dist["connect-fail"]++
			cancel()
			time.Sleep(150 * time.Millisecond)
			continue
		}
		// flashBegin(64B @ 0x10000, no erase) -> flashData -> flashEnd
		payload := append([]byte{0xE9, 0x00, 0x01, 0x02}, bytes.Repeat([]byte{0xAA}, 60)...)
		if err := f.conn.flashBegin(uint32(len(payload)), 0x10000, false, false); err != nil {
			dist["flashBegin:"+short(err)]++
			f.Close()
			cancel()
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if err := f.conn.flashData(payload, 0); err != nil {
			dist["flashData:"+short(err)]++
			f.Close()
			cancel()
			time.Sleep(150 * time.Millisecond)
			continue
		}
		_ = f.conn.flashEnd(false)

		// Read back the first 4 bytes at 0x10000.
		rb, err := f.ReadFlash(0x10000, 4)
		if err != nil {
			dist["readback:"+short(err)]++
			f.Close()
			cancel()
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if len(rb) > 0 && rb[0] == 0xE9 {
			magicOK++
			dist["magic-OK"]++
		} else {
			dist["magic-BAD("+hex.EncodeToString(rb)+")"]++
		}
		f.Close()
		cancel()
		time.Sleep(150 * time.Millisecond)
	}
	t.Logf("writeback distribution: %v", dist)
	t.Logf("magic-OK (0xE9 persisted): %d/%d", magicOK, reps)
}

func short(err error) string {
	s := err.Error()
	if len(s) > 30 {
		s = s[len(s)-30:]
	}
	return s
}
