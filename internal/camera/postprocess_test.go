package camera

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdjustmentParams_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		params   AdjustmentParams
		expected bool
	}{
		{"All zero", AdjustmentParams{}, true},
		{"Only brightness", AdjustmentParams{Brightness: 10}, false},
		{"Only contrast", AdjustmentParams{Contrast: 5}, false},
		{"Only saturation", AdjustmentParams{Saturation: -5}, false},
		{"All set", AdjustmentParams{Brightness: 10, Contrast: 5, Saturation: -5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.params.IsZero())
		})
	}
}

func TestAdjustmentParams_Validate(t *testing.T) {
	tests := []struct {
		name        string
		params      AdjustmentParams
		expectError bool
	}{
		{"Valid all zero", AdjustmentParams{}, false},
		{"Valid positive", AdjustmentParams{Brightness: 100, Contrast: 100, Saturation: 100}, false},
		{"Valid negative", AdjustmentParams{Brightness: -100, Contrast: -100, Saturation: -100}, false},
		{"Valid mixed", AdjustmentParams{Brightness: 50, Contrast: -30, Saturation: 20}, false},
		{"Invalid brightness", AdjustmentParams{Brightness: 101}, true},
		{"Invalid contrast", AdjustmentParams{Contrast: -101}, true},
		{"Invalid saturation", AdjustmentParams{Saturation: 150}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestApplyAdjustments_Brightness(t *testing.T) {
	// Create test image (gray)
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, gray)
		}
	}

	tests := []struct {
		name         string
		brightness   int
		expectDarker bool
		expectLight  bool
	}{
		{"Negative brightness", -50, true, false},
		{"Positive brightness", 50, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adj := &AdjustmentParams{Brightness: tt.brightness}
			result, err := ApplyAdjustments(img, img.Bounds(), adj)
			require.NoError(t, err)

			r, g, b, _ := result.At(5, 5).RGBA()
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)

			if tt.expectDarker {
				assert.Less(t, r8, uint8(128))
				assert.Less(t, g8, uint8(128))
				assert.Less(t, b8, uint8(128))
			}
			if tt.expectLight {
				assert.Greater(t, r8, uint8(128))
				assert.Greater(t, g8, uint8(128))
				assert.Greater(t, b8, uint8(128))
			}
		})
	}
}

func TestApplyAdjustments_NoOp(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	pixel := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	img.Set(5, 5, pixel)

	// Nil adjustment
	result, err := ApplyAdjustments(img, img.Bounds(), nil)
	assert.NoError(t, err)
	assert.Equal(t, img, result)

	// Zero adjustment
	adj := &AdjustmentParams{}
	result, err = ApplyAdjustments(img, img.Bounds(), adj)
	assert.NoError(t, err)

	r, g, b, _ := result.At(5, 5).RGBA()
	assert.Equal(t, uint8(100), uint8(r>>8))
	assert.Equal(t, uint8(150), uint8(g>>8))
	assert.Equal(t, uint8(200), uint8(b>>8))
}

func TestExtractAndAdjust(t *testing.T) {
	// Create 100x100 image
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Extract and adjust center 10x10 region
	adj := &AdjustmentParams{Brightness: 50}
	result, err := ExtractAndAdjust(img, 45, 45, 10, 10, adj)
	require.NoError(t, err)

	bounds := result.Bounds()
	assert.Equal(t, 10, bounds.Dx())
	assert.Equal(t, 10, bounds.Dy())
}

func TestExtractAndAdjust_Invalid(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	tests := []struct {
		name        string
		x, y, w, h  int
		expectError bool
	}{
		{"Negative x", -1, 0, 10, 10, true},
		{"Negative y", 0, -1, 10, 10, true},
		{"Zero width", 0, 0, 0, 10, true},
		{"Zero height", 0, 0, 10, 0, true},
		{"Exceeds bounds", 95, 95, 10, 10, true},
		{"Valid region", 0, 0, 50, 50, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adj := &AdjustmentParams{}
			_, err := ExtractAndAdjust(img, tt.x, tt.y, tt.w, tt.h, adj)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExtractAndAdjust_NonOriginBounds(t *testing.T) {
	// Create 100x100 image with distinct pattern
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Set distinct colors at known positions
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	green := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	blue := color.RGBA{R: 0, G: 0, B: 255, A: 255}

	// Pattern: red at (10,10), green at (15,10), blue at (10,15)
	img.Set(10, 10, red)
	img.Set(15, 10, green)
	img.Set(10, 15, blue)

	// Extract region from (10,10) to (20,20) - non-origin bounds
	result, err := ExtractAndAdjust(img, 10, 10, 10, 10, nil)
	require.NoError(t, err)

	// Verify result dimensions
	bounds := result.Bounds()
	assert.Equal(t, 0, bounds.Min.X, "Result should start at origin X")
	assert.Equal(t, 0, bounds.Min.Y, "Result should start at origin Y")
	assert.Equal(t, 10, bounds.Dx())
	assert.Equal(t, 10, bounds.Dy())

	// Verify pixels are correctly positioned (not black)
	// Source (10,10) should be at result (0,0)
	r, g, b, a := result.At(0, 0).RGBA()
	assert.Equal(t, uint8(255), uint8(r>>8), "Pixel at (0,0) should be red")
	assert.Equal(t, uint8(0), uint8(g>>8), "Pixel at (0,0) should be red")
	assert.Equal(t, uint8(0), uint8(b>>8), "Pixel at (0,0) should be red")
	assert.Equal(t, uint8(255), uint8(a>>8), "Pixel at (0,0) should be opaque")

	// Source (15,10) should be at result (5,0)
	r, g, b, a = result.At(5, 0).RGBA()
	assert.Equal(t, uint8(0), uint8(r>>8), "Pixel at (5,0) should be green")
	assert.Equal(t, uint8(255), uint8(g>>8), "Pixel at (5,0) should be green")
	assert.Equal(t, uint8(0), uint8(b>>8), "Pixel at (5,0) should be green")

	// Source (10,15) should be at result (0,5)
	r, g, b, a = result.At(0, 5).RGBA()
	assert.Equal(t, uint8(0), uint8(r>>8), "Pixel at (0,5) should be blue")
	assert.Equal(t, uint8(0), uint8(g>>8), "Pixel at (0,5) should be blue")
	assert.Equal(t, uint8(255), uint8(b>>8), "Pixel at (0,5) should be blue")
}

func TestExtractAndAdjust_CornerExtraction(t *testing.T) {
	// Test extraction from each corner to ensure offset handling works
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Mark each corner
	corners := []struct {
		x, y int
		c    color.RGBA
	}{
		{0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255}},     // Top-left red
		{90, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255}},    // Top-right green
		{0, 90, color.RGBA{R: 0, G: 0, B: 255, A: 255}},    // Bottom-left blue
		{90, 90, color.RGBA{R: 255, G: 255, B: 0, A: 255}}, // Bottom-right yellow
	}

	for _, corner := range corners {
		img.Set(corner.x, corner.y, corner.c)
	}

	var result image.Image
	var err error
	var r, g, b uint32

	// Extract top-left 10x10 (origin extraction)
	result, err = ExtractAndAdjust(img, 0, 0, 10, 10, nil)
	require.NoError(t, err)
	r, _, _, _ = result.At(0, 0).RGBA()
	assert.Equal(t, uint8(255), uint8(r>>8), "Top-left corner should be red")

	// Extract top-right 10x10 (non-origin X)
	result, err = ExtractAndAdjust(img, 90, 0, 10, 10, nil)
	require.NoError(t, err)
	_, g, _, _ = result.At(0, 0).RGBA()
	assert.Equal(t, uint8(255), uint8(g>>8), "Top-right corner should be green")

	// Extract bottom-left 10x10 (non-origin Y)
	result, err = ExtractAndAdjust(img, 0, 90, 10, 10, nil)
	require.NoError(t, err)
	_, _, b, _ = result.At(0, 0).RGBA()
	assert.Equal(t, uint8(255), uint8(b>>8), "Bottom-left corner should be blue")

	// Extract bottom-right 10x10 (non-origin X and Y)
	result, err = ExtractAndAdjust(img, 90, 90, 10, 10, nil)
	require.NoError(t, err)
	r, g, _, _ = result.At(0, 0).RGBA()
	assert.Equal(t, uint8(255), uint8(r>>8), "Bottom-right corner should have red")
	assert.Equal(t, uint8(255), uint8(g>>8), "Bottom-right corner should have green")
}
