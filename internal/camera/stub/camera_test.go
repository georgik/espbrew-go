// +build !linux

package stub

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStubCamera_GetControlRange(t *testing.T) {
	cam, err := NewCamera("/dev/video0")
	assert.NoError(t, err)

	t.Run("brightness standard range", func(t *testing.T) {
		min, max, err := cam.GetControlRange("brightness")
		assert.NoError(t, err)
		assert.Equal(t, int32(0), min)
		assert.Equal(t, int32(255), max)
	})

	t.Run("contrast standard range", func(t *testing.T) {
		min, max, err := cam.GetControlRange("contrast")
		assert.NoError(t, err)
		assert.Equal(t, int32(0), min)
		assert.Equal(t, int32(255), max)
	})

	t.Run("exposure absolute extended range", func(t *testing.T) {
		min, max, err := cam.GetControlRange("exposure_absolute")
		assert.NoError(t, err)
		assert.Equal(t, int32(0), min)
		assert.Equal(t, int32(2047), max)
	})

	t.Run("unknown control returns standard range", func(t *testing.T) {
		min, max, err := cam.GetControlRange("unknown_control")
		assert.NoError(t, err)
		assert.Equal(t, int32(0), min)
		assert.Equal(t, int32(255), max)
	})
}

func TestStubCamera_GetControlInfo(t *testing.T) {
	cam, err := NewCamera("/dev/video0")
	assert.NoError(t, err)

	t.Run("returns midpoint as current value", func(t *testing.T) {
		min, max, current, err := cam.GetControlInfo("brightness")
		assert.NoError(t, err)
		assert.Equal(t, int32(0), min)
		assert.Equal(t, int32(255), max)
		assert.Equal(t, int32(127), current) // (0 + 255) / 2
	})

	t.Run("exposure uses correct range", func(t *testing.T) {
		min, max, current, err := cam.GetControlInfo("exposure_absolute")
		assert.NoError(t, err)
		assert.Equal(t, int32(0), min)
		assert.Equal(t, int32(2047), max)
		assert.Equal(t, int32(1023), current) // (0 + 2047) / 2
	})

	t.Run("all standard controls have ranges", func(t *testing.T) {
		controls := []string{"brightness", "contrast", "saturation", "sharpness", "gain", "focus_absolute"}
		for _, control := range controls {
			min, max, current, err := cam.GetControlInfo(control)
			assert.NoError(t, err, "Control %s should return info", control)
			assert.Equal(t, int32(0), min, "Control %s min should be 0", control)
			assert.Equal(t, int32(255), max, "Control %s max should be 255", control)
			assert.Equal(t, int32(127), current, "Control %s current should be midpoint", control)
		}
	})
}

func TestStubCamera_GetSettings(t *testing.T) {
	cam, err := NewCamera("/dev/video0")
	assert.NoError(t, err)

	settings, err := cam.GetSettings()
	assert.NoError(t, err)
	assert.NotNil(t, settings)

	// Verify all expected keys exist
	expectedKeys := []string{"brightness", "contrast", "saturation", "sharpness"}
	for _, key := range expectedKeys {
		_, exists := settings[key]
		assert.True(t, exists, "Settings should contain %s", key)
	}

	// Verify default values
	assert.Equal(t, int32(128), settings["brightness"], "Default brightness should be 128")
	assert.Equal(t, int32(32), settings["contrast"], "Default contrast should be 32")
}

func TestStubCamera_OperationsAreNoOps(t *testing.T) {
	cam, err := NewCamera("/dev/video0")
	assert.NoError(t, err)
	defer cam.Close()

	t.Run("set operations return no error", func(t *testing.T) {
		assert.NoError(t, cam.SetBrightness(100))
		assert.NoError(t, cam.SetContrast(100))
		assert.NoError(t, cam.SetSharpness(100))
		assert.NoError(t, cam.SetSaturation(100))
		assert.NoError(t, cam.SetFocus(85))
		assert.NoError(t, cam.SetDisplayPreset())
	})

	t.Run("get operations return default values", func(t *testing.T) {
		brightness, err := cam.GetBrightness()
		assert.NoError(t, err)
		assert.Equal(t, int32(128), brightness)

		contrast, err := cam.GetContrast()
		assert.NoError(t, err)
		assert.Equal(t, int32(32), contrast)
	})

	t.Run("query controls returns error", func(t *testing.T) {
		_, err := cam.QueryControls()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not available")
	})
}
