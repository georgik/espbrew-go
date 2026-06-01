package persistence

import (
	"fmt"
	"math"
)

// BoundingBox represents normalized coordinates (0.0-1.0) relative to image dimensions
type BoundingBox struct {
	X      float64 `json:"x"`      // Top-left X (0.0-1.0)
	Y      float64 `json:"y"`      // Top-left Y (0.0-1.0)
	Width  float64 `json:"width"`  // Width (0.0-1.0)
	Height float64 `json:"height"` // Height (0.0-1.0)
}

// ToPixels converts normalized coordinates to pixel coordinates
func (b *BoundingBox) ToPixels(imageWidth, imageHeight int) (x, y, width, height int) {
	x = int(math.Round(b.X * float64(imageWidth)))
	y = int(math.Round(b.Y * float64(imageHeight)))
	width = int(math.Round(b.Width * float64(imageWidth)))
	height = int(math.Round(b.Height * float64(imageHeight)))
	return
}

// FromPixels creates a BoundingBox from pixel coordinates
func (b *BoundingBox) FromPixels(x, y, width, height, imageWidth, imageHeight int) error {
	if imageWidth <= 0 || imageHeight <= 0 {
		return fmt.Errorf("image dimensions must be positive")
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("box dimensions must be positive")
	}

	b.X = float64(x) / float64(imageWidth)
	b.Y = float64(y) / float64(imageHeight)
	b.Width = float64(width) / float64(imageWidth)
	b.Height = float64(height) / float64(imageHeight)

	return b.Validate()
}

// Validate checks that bounding box coordinates are valid normalized values
func (b *BoundingBox) Validate() error {
	// Check x and y are in [0, 1]
	if b.X < 0 || b.X > 1 {
		return fmt.Errorf("x must be in [0, 1], got %f", b.X)
	}
	if b.Y < 0 || b.Y > 1 {
		return fmt.Errorf("y must be in [0, 1], got %f", b.Y)
	}

	// Check width and height are in (0, 1]
	if b.Width <= 0 || b.Width > 1 {
		return fmt.Errorf("width must be in (0, 1], got %f", b.Width)
	}
	if b.Height <= 0 || b.Height > 1 {
		return fmt.Errorf("height must be in (0, 1], got %f", b.Height)
	}

	// Check that box stays within image bounds
	if b.X+b.Width > 1 {
		return fmt.Errorf("box extends beyond image width: x+width = %f > 1", b.X+b.Width)
	}
	if b.Y+b.Height > 1 {
		return fmt.Errorf("box extends beyond image height: y+height = %f > 1", b.Y+b.Height)
	}

	return nil
}
