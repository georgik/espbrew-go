package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoundingBoxValidate(t *testing.T) {
	tests := []struct {
		name        string
		box         BoundingBox
		wantErr     bool
		errContains string
	}{
		{
			name: "valid box",
			box: BoundingBox{
				X:      0.1,
				Y:      0.2,
				Width:  0.3,
				Height: 0.4,
			},
			wantErr: false,
		},
		{
			name: "full image box",
			box: BoundingBox{
				X:      0,
				Y:      0,
				Width:  1.0,
				Height: 1.0,
			},
			wantErr: false,
		},
		{
			name: "negative x",
			box: BoundingBox{
				X:      -0.1,
				Y:      0.2,
				Width:  0.3,
				Height: 0.4,
			},
			wantErr:     true,
			errContains: "x must be in [0, 1]",
		},
		{
			name: "negative y",
			box: BoundingBox{
				X:      0.1,
				Y:      -0.1,
				Width:  0.3,
				Height: 0.4,
			},
			wantErr:     true,
			errContains: "y must be in [0, 1]",
		},
		{
			name: "x greater than 1",
			box: BoundingBox{
				X:      1.1,
				Y:      0.2,
				Width:  0.3,
				Height: 0.4,
			},
			wantErr:     true,
			errContains: "x must be in [0, 1]",
		},
		{
			name: "y greater than 1",
			box: BoundingBox{
				X:      0.1,
				Y:      1.1,
				Width:  0.3,
				Height: 0.4,
			},
			wantErr:     true,
			errContains: "y must be in [0, 1]",
		},
		{
			name: "zero width",
			box: BoundingBox{
				X:      0.1,
				Y:      0.2,
				Width:  0,
				Height: 0.4,
			},
			wantErr:     true,
			errContains: "width must be in (0, 1]",
		},
		{
			name: "zero height",
			box: BoundingBox{
				X:      0.1,
				Y:      0.2,
				Width:  0.3,
				Height: 0,
			},
			wantErr:     true,
			errContains: "height must be in (0, 1]",
		},
		{
			name: "negative width",
			box: BoundingBox{
				X:      0.1,
				Y:      0.2,
				Width:  -0.1,
				Height: 0.4,
			},
			wantErr:     true,
			errContains: "width must be in (0, 1]",
		},
		{
			name: "box exceeds image width",
			box: BoundingBox{
				X:      0.8,
				Y:      0.2,
				Width:  0.3,
				Height: 0.4,
			},
			wantErr:     true,
			errContains: "extends beyond image width",
		},
		{
			name: "box exceeds image height",
			box: BoundingBox{
				X:      0.1,
				Y:      0.8,
				Width:  0.3,
				Height: 0.4,
			},
			wantErr:     true,
			errContains: "extends beyond image height",
		},
		{
			name: "box at edge",
			box: BoundingBox{
				X:      0.9,
				Y:      0.0,
				Width:  0.1,
				Height: 1.0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.box.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBoundingBoxToPixels(t *testing.T) {
	tests := []struct {
		name         string
		box          BoundingBox
		imageWidth   int
		imageHeight  int
		wantX, wantY int
		wantW, wantH int
	}{
		{
			name:        "full image",
			box:         BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
			imageWidth:  1920,
			imageHeight: 1080,
			wantX:       0,
			wantY:       0,
			wantW:       1920,
			wantH:       1080,
		},
		{
			name:        "half image",
			box:         BoundingBox{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5},
			imageWidth:  1000,
			imageHeight: 800,
			wantX:       250,
			wantY:       200,
			wantW:       500,
			wantH:       400,
		},
		{
			name:        "small box",
			box:         BoundingBox{X: 0.1, Y: 0.1, Width: 0.2, Height: 0.2},
			imageWidth:  640,
			imageHeight: 480,
			wantX:       64,
			wantY:       48,
			wantW:       128,
			wantH:       96,
		},
		{
			name:        "pixel precision",
			box:         BoundingBox{X: 0.5, Y: 0.5, Width: 0.5, Height: 0.5},
			imageWidth:  1001,
			imageHeight: 1001,
			wantX:       501,
			wantY:       501,
			wantW:       501,
			wantH:       501,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x, y, w, h := tt.box.ToPixels(tt.imageWidth, tt.imageHeight)
			assert.Equal(t, tt.wantX, x)
			assert.Equal(t, tt.wantY, y)
			assert.Equal(t, tt.wantW, w)
			assert.Equal(t, tt.wantH, h)
		})
	}
}

func TestBoundingBoxFromPixels(t *testing.T) {
	tests := []struct {
		name       string
		x, y, w, h int
		imgW, imgH int
		want       BoundingBox
	}{
		{
			name: "full image",
			x:    0, y: 0, w: 1920, h: 1080,
			imgW: 1920, imgH: 1080,
			want: BoundingBox{X: 0, Y: 0, Width: 1, Height: 1},
		},
		{
			name: "half image",
			x:    250, y: 200, w: 500, h: 400,
			imgW: 1000, imgH: 800,
			want: BoundingBox{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5},
		},
		{
			name: "small box",
			x:    10, y: 10, w: 32, h: 24,
			imgW: 320, imgH: 240,
			want: BoundingBox{X: 0.03125, Y: 0.041666666666666664, Width: 0.1, Height: 0.1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got BoundingBox
			err := got.FromPixels(tt.x, tt.y, tt.w, tt.h, tt.imgW, tt.imgH)
			require.NoError(t, err)
			assert.InDelta(t, tt.want.X, got.X, 0.0001)
			assert.InDelta(t, tt.want.Y, got.Y, 0.0001)
			assert.InDelta(t, tt.want.Width, got.Width, 0.0001)
			assert.InDelta(t, tt.want.Height, got.Height, 0.0001)
		})
	}
}

func TestBoundingBoxRoundTrip(t *testing.T) {
	imageSizes := []struct{ width, height int }{
		{640, 480},
		{1280, 720},
		{1920, 1080},
		{3840, 2160},
	}

	boxes := []BoundingBox{
		{X: 0, Y: 0, Width: 1, Height: 1},
		{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5},
		{X: 0.1, Y: 0.1, Width: 0.2, Height: 0.2},
		{X: 0.9, Y: 0.9, Width: 0.1, Height: 0.1},
	}

	for _, imgSize := range imageSizes {
		for _, box := range boxes {
			t.Run("", func(t *testing.T) {
				px, py, pw, ph := box.ToPixels(imgSize.width, imgSize.height)
				var reconstructed BoundingBox
				err := reconstructed.FromPixels(px, py, pw, ph, imgSize.width, imgSize.height)
				require.NoError(t, err)
				assert.InDelta(t, box.X, reconstructed.X, 0.0001)
				assert.InDelta(t, box.Y, reconstructed.Y, 0.0001)
				assert.InDelta(t, box.Width, reconstructed.Width, 0.0001)
				assert.InDelta(t, box.Height, reconstructed.Height, 0.0001)
			})
		}
	}
}
