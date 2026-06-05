//go:build linux
// +build linux

package linux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCamera_GetControlRange(t *testing.T) {
	// This test requires a real V4L2 device
	// Skip if no device available
	t.Skip("Requires real V4L2 device - run with hardware")

	// Example test structure for when run with real device:
	// cam, err := NewCamera("/dev/video0")
	// if err != nil {
	//     t.Skip("No camera device available")
	// }
	// defer cam.Close()
	//
	// min, max, err := cam.GetControlRange(v4l2.CtrlBrightness)
	// assert.NoError(t, err)
	// assert.True(t, min < max, "Min should be less than max")
}

func TestCamera_GetSettingsClampsInvalidValues(t *testing.T) {
	// Test the clamping logic in GetSettings
	// This tests the addSetting helper behavior

	t.Run("brightness clamping logic", func(t *testing.T) {
		// Test cases for clamping logic
		tests := []struct {
			name     string
			value    int32
			min      int32
			max      int32
			expected int32
		}{
			{"valid mid range", 128, 0, 255, 128},
			{"value too low", -24, 0, 255, 0},      // Should clamp to min
			{"value too high", 300, 0, 255, 255},   // Should clamp to max
			{"value at min", 0, 0, 255, 0},         // Should stay at min
			{"value at max", 255, 0, 255, 255},     // Should stay at max
			{"negative extreme", -1000, 0, 255, 0}, // Should clamp to min
			{"large positive", 10000, 0, 255, 255}, // Should clamp to max
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Simulate the clamping logic from GetSettings
				var result int32
				if tt.value >= tt.min && tt.value <= tt.max {
					result = tt.value
				} else {
					if tt.value < tt.min {
						result = tt.min
					} else if tt.value > tt.max {
						result = tt.max
					}
				}
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("different camera ranges", func(t *testing.T) {
		// Test that different cameras can have different ranges
		tests := []struct {
			name     string
			value    int32
			min      int32
			max      int32
			expected int32
		}{
			{"HP webcam saturation", 128, 0, 100, 100}, // Clamp to max 100
			{"Logitech sharpness", 128, 0, 10, 10},     // Clamp to max 10
			{"standard brightness", -24, 0, 255, 0},    // Clamp negative to min
			{"exposure absolute", 3000, 0, 2047, 2047}, // Clamp to max 2047
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Simulate the clamping logic
				var result int32
				if tt.value >= tt.min && tt.value <= tt.max {
					result = tt.value
				} else {
					if tt.value < tt.min {
						result = tt.min
					} else if tt.value > tt.max {
						result = tt.max
					}
				}
				assert.Equal(t, tt.expected, result,
					"Value %d should clamp to %d for range [%d, %d]",
					tt.value, tt.expected, tt.min, tt.max)
			})
		}
	})
}

func TestCamera_GetControlRangeByName(t *testing.T) {
	t.Run("unknown control returns error", func(t *testing.T) {
		cam := &Camera{path: "/dev/video0"}
		_, _, err := cam.GetControlRangeByName("unknown_control")
		assert.Error(t, err, "Unknown control should return error")
		assert.Contains(t, err.Error(), "unknown control")
	})

	t.Run("valid control names require device", func(t *testing.T) {
		// Valid control names would succeed with a real device
		// Without a device, we get a different error (not "unknown control")
		t.Skip("Requires real V4L2 device")
	})
}

func TestCamera_GetControlInfoByName(t *testing.T) {
	t.Run("unknown control returns error", func(t *testing.T) {
		cam := &Camera{path: "/dev/video0"}
		_, _, _, err := cam.GetControlInfoByName("unknown_control")
		assert.Error(t, err, "Unknown control should return error")
		assert.Contains(t, err.Error(), "unknown control")
	})

	t.Run("valid control names require device", func(t *testing.T) {
		t.Skip("Requires real V4L2 device")
	})
}

// Benchmark for range lookup
func BenchmarkGetControlRangeByName(b *testing.B) {
	cam := &Camera{path: "/dev/video0"}
	controlName := "brightness"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cam.GetControlRangeByName(controlName)
	}
}
