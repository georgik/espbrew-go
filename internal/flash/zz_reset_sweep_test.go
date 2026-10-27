package flash

import (
	"context"
	"testing"
	"time"

	"github.com/georgik/espbrew-go/internal/chips"
	"github.com/georgik/espbrew-go/internal/espflash"
)

// TestResetReliabilitySweep is the reset-reliability experiment. It drives the
// REAL Flash() path with the actual P4 ELF. Each successful Flash() leaves the
// chip booting the freshly-written app; the NEXT Flash()'s connect() must
// therefore re-enter download mode from the "app running" state.
//
// espflash.LastConnectAttempts records, per successful connect(), the 0-based
// attempt index that finally won. For a single Flash() call the FIRST recorded
// index is the reconnect-from-app attempt we care about:
//
//	0  -> the FIRST reset strategy caught the download window (no retry)
//	>0 -> the first reset missed and a later attempt fell through (retry)
//	-1 -> the reset cascade exhausted every attempt (hard failure)
//
// We sweep the three ResetMode strategies the production path can select and
// report the first-attempt success rate for each. A low first-attempt rate is
// exactly what surfaces in the logs as
// "timeout waiting for response to SLIP read" followed by a retry.
func TestResetReliabilitySweep(t *testing.T) {
	// Hardware sweep test: gated behind EB_HARDWARE_TEST so it never blocks on
	// a stale/absent board node, and skips when the fixture ELF is not built.
	if !hardwareTestEnabled() || !hasHardwarePort("/dev/ttyACM0") {
		t.Skip("hardware test disabled (set EB_HARDWARE_TEST=1) or no /dev/ttyACM0 attached")
	}
	data := readFixtureOrSkip(t, investigationFixture)
	t.Logf("elf size: %d bytes", len(data))

	strategies := []struct {
		name string
		mode espflash.ResetMode
	}{
		{"ResetAuto(currentP4Default)", espflash.ResetAuto},
		{"ResetUSBJTAG(usb-first)", espflash.ResetUSBJTAG},
		{"ResetDefault(classic-first)", espflash.ResetDefault},
	}

	const reps = 8
	for _, s := range strategies {
		f := NewFlasher(&FlasherOptions{
			BaudRate:      115200,
			FlashBaudRate: 460800,
			Compress:      true,
			Erase:         false,
			FastMode:      false, // keep retries room (espflash uses 9 attempts)
			ResetMode:     s.mode,
		})

		first0, retry, fail := 0, 0, 0
		seenAll := [][]int{}
		for i := 0; i < reps; i++ {
			espflash.LastConnectAttempts = nil
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

			res := f.Flash(ctx, &FlashRequest{
				Port:     "/dev/ttyACM0",
				Firmware: data,
				Chip:     chips.ChipESP32P4,
			})

			seen := append([]int(nil), espflash.LastConnectAttempts...)
			cancel()

			if !res.Success {
				fail++
				t.Logf("  %s iter %d: FLASH FAILED (%v)", s.name, i, res.Error)
				time.Sleep(300 * time.Millisecond)
				continue
			}
			first := -1
			if len(seen) > 0 {
				first = seen[0]
			}
			if first == 0 {
				first0++
			} else {
				retry++
			}
			seenAll = append(seenAll, seen)
			t.Logf("  %s iter %d: first-attempt=%d (full connect attempts=%v)", s.name, i, first, seen)
			time.Sleep(300 * time.Millisecond)
		}
		t.Logf("=== %s : first-attempt-ok=%d/%d  retry-needed=%d  hard-fail=%d",
			s.name, first0, reps, retry, fail)
		t.Logf("    per-iter full connect-attempt vectors: %v", seenAll)
	}
}
