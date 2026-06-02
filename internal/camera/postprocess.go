package camera

import (
	"image"
	"image/color"
	"math"

	"github.com/rs/zerolog/log"
)

// AdjustmentParams holds image enhancement values
type AdjustmentParams struct {
	Brightness int // -100 to 100
	Contrast   int // -100 to 100
	Saturation int // -100 to 100
}

// IsZero returns true if all values are zero
func (a *AdjustmentParams) IsZero() bool {
	return a.Brightness == 0 && a.Contrast == 0 && a.Saturation == 0
}

// Validate checks values are in range
func (a *AdjustmentParams) Validate() error {
	if a.Brightness < -100 || a.Brightness > 100 {
		return &ValidationError{Field: "brightness", Value: a.Brightness, Min: -100, Max: 100}
	}
	if a.Contrast < -100 || a.Contrast > 100 {
		return &ValidationError{Field: "contrast", Value: a.Contrast, Min: -100, Max: 100}
	}
	if a.Saturation < -100 || a.Saturation > 100 {
		return &ValidationError{Field: "saturation", Value: a.Saturation, Min: -100, Max: 100}
	}
	return nil
}

// ValidationError represents an out-of-range value
type ValidationError struct {
	Field string
	Value int
	Min   int
	Max   int
}

func (e *ValidationError) Error() string {
	return e.Field + " must be in [" + string(rune(e.Min)) + ", " + string(rune(e.Max)) + "], got " + string(rune(e.Value))
}

// ApplyAdjustments applies brightness/contrast/saturation to image region
func ApplyAdjustments(src image.Image, bounds image.Rectangle, adj *AdjustmentParams) (image.Image, error) {
	if adj != nil {
		if err := adj.Validate(); err != nil {
			return nil, err
		}

		log.Debug().
			Int("brightness", adj.Brightness).
			Int("contrast", adj.Contrast).
			Int("saturation", adj.Saturation).
			Msg("Applying image adjustments")
	}

	// Extract the region first
	result := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			result.Set(x-bounds.Min.X, y-bounds.Min.Y, color.RGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			})
		}
	}

	// If no adjustments, return extracted region as-is
	if adj == nil || adj.IsZero() {
		return result, nil
	}

	// Calculate factors
	brightnessFactor := float64(adj.Brightness) / 100.0 * 128.0 // -128 to +128
	contrastFactor := 1.0 + float64(adj.Contrast)/100.0         // 0.0 to 2.0
	saturationFactor := 1.0 + float64(adj.Saturation)/100.0     // 0.0 to 2.0

	// Apply adjustments to the extracted region
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			r, g, b, a := result.At(x, y).RGBA()

			// Convert to 0-255 range
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)

			// Apply brightness
			if adj.Brightness != 0 {
				r8 = clamp(int(r8) + int(brightnessFactor))
				g8 = clamp(int(g8) + int(brightnessFactor))
				b8 = clamp(int(b8) + int(brightnessFactor))
			}

			// Apply contrast
			if adj.Contrast != 0 {
				r8 = clamp(int(contrastFactor*(float64(r8)-128) + 128))
				g8 = clamp(int(contrastFactor*(float64(g8)-128) + 128))
				b8 = clamp(int(contrastFactor*(float64(b8)-128) + 128))
			}

			// Apply saturation
			if adj.Saturation != 0 {
				// Convert to HSL-like, adjust saturation, convert back
				gray := 0.299*float64(r8) + 0.587*float64(g8) + 0.114*float64(b8)
				r8 = clamp(int(gray + saturationFactor*(float64(r8)-gray)))
				g8 = clamp(int(gray + saturationFactor*(float64(g8)-gray)))
				b8 = clamp(int(gray + saturationFactor*(float64(b8)-gray)))
			}

			result.Set(x, y, color.RGBA{R: r8, G: g8, B: b8, A: uint8(a >> 8)})
		}
	}

	return result, nil
}

// ExtractAndAdjust extracts a region and applies adjustments
func ExtractAndAdjust(src image.Image, x, y, width, height int, adj *AdjustmentParams) (image.Image, error) {
	if x < 0 || y < 0 || width <= 0 || height <= 0 {
		return nil, &ExtractError{x, y, width, height}
	}

	bounds := src.Bounds()
	if x+width > bounds.Dx() || y+height > bounds.Dy() {
		return nil, &ExtractError{x, y, width, height}
	}

	regionRect := image.Rect(x, y, x+width, y+height)
	return ApplyAdjustments(src, regionRect, adj)
}

// ExtractError represents invalid extract parameters
type ExtractError struct {
	X, Y, Width, Height int
}

func (e *ExtractError) Error() string {
	return "invalid extract region"
}

// clamp value to 0-255 range
func clamp(v int) uint8 {
	return uint8(math.Min(255, math.Max(0, float64(v))))
}
