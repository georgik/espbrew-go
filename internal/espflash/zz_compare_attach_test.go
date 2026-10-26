package espflash

import (
	"context"
	"testing"
	"time"
)

// TestZzCompareAttach does two things in one binary so results are comparable:
//
//	A) connect + single attachFlash   (the "standalone" measurement)
//	B) connect + 4x attachFlash        (the "multi-image" measurement)
func TestZzCompareAttach(t *testing.T) {
	base := DefaultOptions()
	base.Logger = &captureLogger{}
	base.ConnectAttempts = 2
	base.BaudRate = 115200

	// A) single attach
	aOk, aErr := 0, ""
	for i := 0; i < 6; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		f, err := New(ctx, "/dev/ttyACM0", base)
		if err != nil {
			cancel()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		ae := f.attachFlash()
		f.Close()
		cancel()
		if ae == nil {
			aOk++
		} else {
			aErr = ae.Error()
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("(A) single-attach success=%d/6  lastErr=%q", aOk, aErr)

	// B) four attaches on one connection
	bAll := []int{}
	for i := 0; i < 6; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		f, err := New(ctx, "/dev/ttyACM0", base)
		if err != nil {
			cancel()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		res := []string{}
		for j := 0; j < 4; j++ {
			if ae := f.attachFlash(); ae != nil {
				res = append(res, ae.Error())
			} else {
				res = append(res, "ok")
			}
		}
		bAll = append(bAll, countTrue(res))
		f.Close()
		cancel()
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("(B) 4-attach: ok-calls-per-trial (out of 4) = %v", bAll)
}

func countTrue(s []string) int {
	n := 0
	for _, x := range s {
		if x == "ok" {
			n++
		}
	}
	return n
}
