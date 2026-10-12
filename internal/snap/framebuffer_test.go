package snap

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"
)

// buildFrameLog builds serial log lines the same way the embedded-screenshot
// Rust crate emits them: header, base64 payload chunks, footer.
func buildFrameLog(width, height int, pixels []byte, chunkSize int) []string {
	lines := []string{strings.Join([]string{
		"FRAMEBUFFER_BEGIN",
		strings.TrimSpace(itoa(width)),
		strings.TrimSpace(itoa(height)),
		"RGBL",
	}, " ")}
	for len(pixels) > 0 {
		chunk := chunkSize
		if chunk > len(pixels) {
			chunk = len(pixels)
		}
		lines = append(lines, "FB_BASE64 "+base64.StdEncoding.EncodeToString(pixels[:chunk]))
		pixels = pixels[chunk:]
	}
	lines = append(lines, "FRAMEBUFFER_END")
	return lines
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestFramebufferCollectorExtractsPNG(t *testing.T) {
	// Two pixels: red 0xF800 (LE: 0x00, 0xF8) and blue 0x001F (LE: 0x1F, 0x00).
	pixels := []byte{0x00, 0xF8, 0x1F, 0x00}
	lines := buildFrameLog(2, 1, pixels, 3) // tiny chunks force multiple FB_BASE64 lines
	lines = append([]string{"I (100) boot noise", "other log line"}, lines...)
	lines = append(lines, "I (9000) trailing noise")

	collector := NewFramebufferCollector()
	var stored []string
	for _, line := range lines {
		if consumed, _ := collector.Feed(line); !consumed {
			stored = append(stored, line)
		}
	}

	if !collector.Complete() {
		t.Fatalf("collector did not complete, err: %v", collector.Err())
	}
	fb := collector.Image()
	if fb == nil {
		t.Fatal("no image produced")
	}
	if fb.Width != 2 || fb.Height != 1 || fb.Fourcc != "RGBL" {
		t.Fatalf("unexpected metadata: %dx%d %s", fb.Width, fb.Height, fb.Fourcc)
	}

	// Protocol lines must never reach the stored log.
	joined := strings.Join(stored, "\n")
	if strings.Contains(joined, "FRAMEBUFFER") || strings.Contains(joined, "FB_BASE64") {
		t.Fatalf("protocol lines leaked into log: %q", joined)
	}
	want := []string{"I (100) boot noise", "other log line", "I (9000) trailing noise"}
	if len(stored) != len(want) {
		t.Fatalf("stored log = %q, want %q", stored, want)
	}

	// The PNG must decode to the expected RGB888 pixels.
	img, err := png.Decode(bytes.NewReader(fb.PNGData))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 2 || bounds.Dy() != 1 {
		t.Fatalf("PNG size %dx%d, want 2x1", bounds.Dx(), bounds.Dy())
	}
	// RGB565 0xF800 -> R=248 (0xF8<<3), G=0, B=0; 0x001F -> B=248.
	r0, g0, b0, _ := img.At(0, 0).RGBA()
	r1, g1, b1, _ := img.At(1, 0).RGBA()
	if r0>>8 != 248 || g0>>8 != 0 || b0>>8 != 0 {
		t.Fatalf("pixel 0 = (%d,%d,%d), want (248,0,0)", r0>>8, g0>>8, b0>>8)
	}
	if r1>>8 != 0 || g1>>8 != 0 || b1>>8 != 248 {
		t.Fatalf("pixel 1 = (%d,%d,%d), want (0,0,248)", r1>>8, g1>>8, b1>>8)
	}
}

func TestFramebufferCollectorPassthroughWithoutFrame(t *testing.T) {
	collector := NewFramebufferCollector()
	for _, line := range []string{"hello", "FRAMEBUFFER_UNKNOWN", "world"} {
		if consumed, complete := collector.Feed(line); consumed || complete {
			t.Fatalf("line %q unexpectedly consumed", line)
		}
	}
	if collector.Complete() || collector.Err() != nil {
		t.Fatal("collector changed state on unrelated lines")
	}
}

func TestFramebufferCollectorInterruptedPayload(t *testing.T) {
	collector := NewFramebufferCollector()
	collector.Feed("FRAMEBUFFER_BEGIN 2 1 RGBL")
	collector.Feed("FB_BASE64 AAAA")
	// A non-protocol line while collecting means the dump was interrupted.
	consumed, _ := collector.Feed("I (123) unrelated line")
	if consumed {
		t.Fatal("interrupting line should not be consumed")
	}
	if collector.Err() == nil {
		t.Fatal("expected error after interrupted payload")
	}
	// After the error the collector must be reusable for a later frame.
	collector.Feed("FRAMEBUFFER_BEGIN 1 1 RGBL")
	if !collector.Active() {
		t.Fatal("collector did not return to collecting state after error")
	}
}

func TestFramebufferCollectorSizeMismatch(t *testing.T) {
	collector := NewFramebufferCollector()
	collector.Feed("FRAMEBUFFER_BEGIN 4 4 RGBL")
	// Only 2 bytes of payload for a 4x4 framebuffer.
	collector.Feed("FB_BASE64 " + base64.StdEncoding.EncodeToString([]byte{0, 0}))
	collector.Feed("FRAMEBUFFER_END")
	if collector.Complete() {
		t.Fatal("size mismatch must not produce an image")
	}
	if collector.Err() == nil {
		t.Fatal("expected size mismatch error")
	}
}
