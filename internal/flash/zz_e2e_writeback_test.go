package flash

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/georgik/espbrew-go/internal/chips"
)

// TestE2E_RealFlashWriteback flashes the ACTUAL P4 ELF through the real
// Flash() path, then reads back the factory app offset 0x10000 and checks the
// image magic byte (0xE9). The boot log shows "invalid magic byte at 0x10000"
// when this write does not persist, so this is the end-to-end reproduction.
func TestE2E_RealFlashWriteback(t *testing.T) {
	// Hardware e2e test: skip in CI (no board attached) and when the fixture
	// ELF is not built, so it never blocks on a timeout or fails the suite.
	if !hasHardwarePort("/dev/ttyACM0") {
		t.Skip("no /dev/ttyACM0 attached, skipping hardware e2e test")
	}
	data := readFixtureOrSkip(t, investigationFixture)
	t.Logf("elf size: %d bytes", len(data))

	f := NewFlasher(&FlasherOptions{
		BaudRate:      115200,
		FlashBaudRate: 460800,
		Compress:      true,
		Erase:         false,
		FastMode:      true,
	})

	magicOK := 0
	dist := map[string]int{}
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)

		res := f.Flash(ctx, &FlashRequest{
			Port:     "/dev/ttyACM0",
			Firmware: data,
			Chip:     chips.ChipESP32P4,
		})

		// Read back the first 4 bytes at 0x10000.
		rbRes := f.ReadFlash(ctx, &ReadFlashRequest{Port: "/dev/ttyACM0", Address: 0x10000, Size: 4})
		cancel()

		if !res.Success {
			dist["flash-fail:"+res.Error.Error()]++
			t.Logf("  iter %d: flash FAILED: %v", i, res.Error)
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if !rbRes.Success {
			dist["readback-fail:"+rbRes.Error.Error()]++
			t.Logf("  iter %d: readback FAILED: %v", i, rbRes.Error)
			time.Sleep(300 * time.Millisecond)
			continue
		}
		magic := "none"
		if len(rbRes.Data) > 0 {
			magic = hex.EncodeToString(rbRes.Data[:min(4, len(rbRes.Data))])
		}
		if len(rbRes.Data) > 0 && rbRes.Data[0] == 0xE9 {
			magicOK++
			dist["magic-OK"]++
		} else {
			dist["magic-BAD("+magic+")"]++
		}
		t.Logf("  iter %d: flash OK, 0x10000=%s", i, magic)
		time.Sleep(300 * time.Millisecond)
	}
	t.Logf("e2e distribution: %v", dist)
	t.Logf("magic-OK: %d/5", magicOK)
}
