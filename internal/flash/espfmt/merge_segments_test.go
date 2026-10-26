package espfmt

import (
	"bytes"
	"testing"
)

// TestMergeSegmentsAppendsRealBytes is a regression test for a bug where the
// segment merger appended *zero padding* instead of the next segment's real
// bytes when joining adjacent/overlapping segments. On the ESP32-P4 that turned
// a valid RAM segment into zeros, so the 2nd stage bootloader jumped into
// zero-filled flash and the app crashed with "Illegal instruction (2)".
//
// The merger must always append the next segment's real bytes (zero-filling only
// genuine sub-4-byte gaps), never synthesize zeros where data belongs.
func TestMergeSegmentsAppendsRealBytes(t *testing.T) {
	b := NewImageBuilder(ChipESP32P4)

	// Two contiguous RAM segments: seg A ends exactly where seg B begins.
	a := Segment{Addr: 0x40380000, Data: []byte{0xde, 0xad, 0xbe, 0xef, 0x11, 0x22, 0x33, 0x44}}
	bseg := Segment{Addr: 0x40380008, Data: []byte{0xaa, 0xbb, 0xcc, 0xdd, 0x55, 0x66}}
	got := b.mergeSegments([]Segment{a, bseg})

	if len(got) != 1 {
		t.Fatalf("expected 1 merged segment, got %d: %+v", len(got), got)
	}
	want := append([]byte{}, a.Data...)
	want = append(want, bseg.Data...)
	if !bytes.Equal(got[0].Data, want) {
		t.Errorf("merged data = %x, want %x (real bytes must be preserved, not zeroed)",
			got[0].Data, want)
	}
	if anyZero(got[0].Data) {
		t.Errorf("merged data unexpectedly contains zeros: %x", got[0].Data)
	}
}

// TestMergeSegmentsSmallGapFillsOnlyTheGap verifies that a sub-4-byte gap is
// zero-filled while the following segment's real bytes are still appended.
func TestMergeSegmentsSmallGapFillsOnlyTheGap(t *testing.T) {
	b := NewImageBuilder(ChipESP32P4)
	// seg A ends at 0x16 (not 4-aligned); seg B starts 2 bytes later, within the
	// 4-byte merge tolerance.
	a := Segment{Addr: 0x10, Data: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}}
	bseg := Segment{Addr: 0x18, Data: []byte{0x0a, 0x0b, 0x0c, 0x0d}}
	got := b.mergeSegments([]Segment{a, bseg})

	if len(got) != 1 {
		t.Fatalf("expected 1 merged segment, got %d", len(got))
	}
	// layout: A(6 bytes) + 2 gap bytes + B(4 bytes) = 12 bytes
	want := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x00, 0x00, 0x0a, 0x0b, 0x0c, 0x0d}
	if !bytes.Equal(got[0].Data, want) {
		t.Errorf("merged data = %x, want %x", got[0].Data, want)
	}
}

// TestMergeSegmentsLargeGapKeepsSeparate verifies that a gap wider than the
// 4-byte tolerance leaves the segments as distinct merged entries.
func TestMergeSegmentsLargeGapKeepsSeparate(t *testing.T) {
	b := NewImageBuilder(ChipESP32P4)
	a := Segment{Addr: 0x10, Data: []byte{0x01, 0x02, 0x03, 0x04}}
	bseg := Segment{Addr: 0x20, Data: []byte{0x0a, 0x0b, 0x0c, 0x0d}}
	got := b.mergeSegments([]Segment{a, bseg})

	if len(got) != 2 {
		t.Fatalf("expected 2 separate segments for a wide gap, got %d", len(got))
	}
	if !bytes.Equal(got[0].Data, a.Data) || !bytes.Equal(got[1].Data, bseg.Data) {
		t.Errorf("segments not preserved: %+v", got)
	}
}

// TestBuildAppImagePreservesRamSegments is an end-to-end regression test: build
// an image whose RAM segments are contiguous and confirm the produced image
// carries the real RAM bytes (no zero-filled hole where RAM data should be).
func TestBuildAppImagePreservesRamSegments(t *testing.T) {
	b := NewImageBuilder(ChipESP32P4)

	// Two contiguous RAM segments (IRAM addresses, so they are classified as
	// RAM, not flash). AddSegment feeds b.segments, which BuildAppImage
	// processes; with no flash segments to gap-fill, the merged RAM segment is
	// written verbatim to the image.
	b.AddSegment(0x40380000, []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x81})
	b.AddSegment(0x40380008, []byte{0x90, 0xa0, 0xb0, 0xc0, 0xd0, 0xe0, 0xf0, 0x01})

	img, err := b.BuildAppImage()
	if err != nil {
		t.Fatalf("BuildAppImage: %v", err)
	}

	// The concatenated RAM bytes must appear in the built image. A zero-filled
	// merge would have dropped them entirely.
	segA := []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x81}
	segB := []byte{0x90, 0xa0, 0xb0, 0xc0, 0xd0, 0xe0, 0xf0, 0x01}
	var ramBytes []byte
	ramBytes = append(ramBytes, segA...)
	ramBytes = append(ramBytes, segB...)
	if !bytes.Contains(img, ramBytes) {
		t.Errorf("built image does not contain the contiguous RAM bytes; merge likely zeroed them\n  image=%x", img)
	}
}

// anyZero reports whether the slice contains any zero byte.
func anyZero(b []byte) bool {
	for _, x := range b {
		if x == 0 {
			return true
		}
	}
	return false
}
