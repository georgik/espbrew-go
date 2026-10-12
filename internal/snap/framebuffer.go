package snap

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"
)

// FramebufferImage holds a framebuffer extracted from the serial log in the
// esp_lcd_screenshot text protocol:
//
//	FRAMEBUFFER_BEGIN <width> <height> <fourcc>
//	FB_BASE64 <base64 payload chunk>
//	...
//	FRAMEBUFFER_END
//
// The firmware emits the payload as plain base64 lines (RGB565 little-endian
// for fourcc RGBL), so it can be reassembled without storing the bulky log
// lines themselves. PNG encoding makes the image directly viewable and keeps
// the snap package small compared to the raw base64 text.
type FramebufferImage struct {
	Width   int    `json:"width"`  // Framebuffer width in pixels
	Height  int    `json:"height"` // Framebuffer height in pixels
	Fourcc  string `json:"fourcc"` // Pixel format, e.g. RGBL (RGB565 LE)
	PNGData []byte `json:"-"`      // Encoded PNG image (not JSON serialized)
	PNGSize int    `json:"png_size"`

	// Base64 of the PNG for cluster transport, populated by RecomputeBase64.
	Base64 string `json:"framebuffer_base64,omitempty"`
}

// RecomputeBase64 refreshes the transport base64 from the PNG bytes.
func (f *FramebufferImage) RecomputeBase64() {
	if len(f.PNGData) > 0 {
		f.Base64 = base64.StdEncoding.EncodeToString(f.PNGData)
	} else {
		f.Base64 = ""
	}
}

// ToMap converts the framebuffer to a map for JSON serialization.
func (f *FramebufferImage) ToMap() map[string]interface{} {
	m := map[string]interface{}{
		"width":    f.Width,
		"height":   f.Height,
		"fourcc":   f.Fourcc,
		"png_size": f.PNGSize,
	}
	if f.Base64 != "" {
		m["framebuffer_base64"] = f.Base64
	}
	return m
}

// FramebufferCollector is a line-oriented state machine that extracts a
// framebuffer from a serial log stream. Lines that belong to the framebuffer
// protocol are consumed (they never reach the stored log); every other line
// passes through untouched.
type FramebufferCollector struct {
	state   int // 0 = idle, 1 = collecting payload, 2 = complete
	width   int
	height  int
	fourcc  string
	payload strings.Builder
	err     error
	image   *FramebufferImage
}

// NewFramebufferCollector creates a collector in the idle state.
func NewFramebufferCollector() *FramebufferCollector {
	return &FramebufferCollector{}
}

// Active reports whether the collector is currently inside a
// FRAMEBUFFER_BEGIN ... FRAMEBUFFER_END block.
func (c *FramebufferCollector) Active() bool {
	return c.state == 1
}

// Complete reports whether a full framebuffer has been extracted.
func (c *FramebufferCollector) Complete() bool {
	return c.state == 2
}

// Err returns the error that aborted a collection, if any. After an error the
// collector returns to the idle state so later frames can still be captured.
func (c *FramebufferCollector) Err() error {
	return c.err
}

// Image returns the extracted framebuffer, or nil if none was collected yet.
func (c *FramebufferCollector) Image() *FramebufferImage {
	return c.image
}

// Feed processes one serial log line. It reports whether the line belonged to
// the framebuffer protocol (consumed = true, the caller must not store it in
// the log) and whether this line completed the collection.
func (c *FramebufferCollector) Feed(line string) (consumed, complete bool) {
	consumed, complete, _ = c.feed(line)
	return consumed, complete
}

func (c *FramebufferCollector) feed(line string) (consumed, complete bool, err error) {
	switch c.state {
	case 0:
		if rest, ok := strings.CutPrefix(line, "FRAMEBUFFER_BEGIN "); ok {
			parts := strings.Fields(rest)
			if len(parts) != 3 {
				c.abort(fmt.Errorf("invalid FRAMEBUFFER_BEGIN line: %q", line))
				return true, false, c.err
			}
			width, errW := strconv.Atoi(parts[0])
			height, errH := strconv.Atoi(parts[1])
			if errW != nil || errH != nil || width <= 0 || height <= 0 {
				c.abort(fmt.Errorf("invalid framebuffer dimensions: %q", line))
				return true, false, c.err
			}
			c.width = width
			c.height = height
			c.fourcc = parts[2]
			c.payload.Reset()
			c.state = 1
			return true, false, nil
		}
		return false, false, nil

	case 1:
		if rest, ok := strings.CutPrefix(line, "FB_BASE64 "); ok {
			c.payload.WriteString(strings.TrimRight(rest, " \t"))
			return true, false, nil
		}
		if line == "FRAMEBUFFER_END" {
			return true, c.finish(), nil
		}
		// Any other line inside the block terminates the frame: either the
		// dump was interrupted or the payload would be corrupt anyway.
		c.abort(fmt.Errorf("line %q interrupted framebuffer payload", line))
		return false, false, c.err

	default:
		return false, false, nil
	}
}

// finish decodes the accumulated payload and builds the PNG image.
func (c *FramebufferCollector) finish() bool {
	raw, err := base64.StdEncoding.DecodeString(c.payload.String())
	if err != nil {
		c.abort(fmt.Errorf("decode framebuffer base64: %w", err))
		return false
	}

	pngData, err := encodeFramebufferPNG(c.fourcc, c.width, c.height, raw)
	if err != nil {
		c.abort(err)
		return false
	}

	c.image = &FramebufferImage{
		Width:   c.width,
		Height:  c.height,
		Fourcc:  c.fourcc,
		PNGData: pngData,
		PNGSize: len(pngData),
	}
	c.state = 2
	return true
}

// abort records the error and returns to idle so a later frame can be caught.
func (c *FramebufferCollector) abort(err error) {
	c.err = err
	c.state = 0
}

// encodeFramebufferPNG converts raw framebuffer bytes (RGB565 little-endian
// for fourcc RGBL) to RGB888 and encodes a PNG of width × height. Only RGBL
// is supported — the format produced by the embedded-screenshot Rust crate.
func encodeFramebufferPNG(fourcc string, width, height int, raw []byte) ([]byte, error) {
	if fourcc != "RGBL" {
		return nil, fmt.Errorf("unsupported framebuffer fourcc: %s", fourcc)
	}

	expected := width * height * 2
	if len(raw) != expected {
		return nil, fmt.Errorf("RGBL payload size %d does not match %dx%d framebuffer (%d expected)",
			len(raw), width, height, expected)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i, px := range raw {
		// RGB565 little-endian: two bytes per pixel, expand to RGB888 with
		// the same shift-and-scale as the esp_lcd_screenshot decoder.
		if i%2 == 0 {
			continue
		}
		value := uint16(raw[i-1]) | uint16(px)<<8
		r := uint8((value>>11)&0x1F) << 3
		g := uint8((value>>5)&0x3F) << 2
		b := uint8(value&0x1F) << 3
		offset := (i - 1) / 2 * 4
		img.Pix[offset] = r
		img.Pix[offset+1] = g
		img.Pix[offset+2] = b
		img.Pix[offset+3] = 0xFF
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode framebuffer PNG: %w", err)
	}
	return buf.Bytes(), nil
}
