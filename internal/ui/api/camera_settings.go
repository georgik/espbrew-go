//go:build js
// +build js

package api

import (
	"syscall/js"
)

// GetCameraControls retrieves available controls for a camera
func GetCameraControls(cameraID string, callback func(*CameraControlsResponse, error)) {
	DefaultAsyncClient.Get("/camera/"+cameraID+"/controls", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		response := &CameraControlsResponse{
			Current:       make(map[string]int32),
			DisplayPreset: make(map[string]int32),
			FocusPresets:  make(map[string]int32),
			Ranges:        make(map[string]ControlRange),
		}

		// Parse current values
		current := result.Get("current")
		if !current.IsUndefined() && !current.IsNull() {
			parseCurrentValues(current, response.Current)
		}

		// Parse availability
		response.Available = result.Get("available").Bool()
		response.Platform = ValueToString(result.Get("platform"))

		// Parse display preset
		displayPreset := result.Get("display_preset")
		if !displayPreset.IsUndefined() && !displayPreset.IsNull() {
			parseCurrentValues(displayPreset, response.DisplayPreset)
		}

		// Parse focus presets
		focusPresets := result.Get("focus_presets")
		if !focusPresets.IsUndefined() && !focusPresets.IsNull() {
			parseCurrentValues(focusPresets, response.FocusPresets)
		}

		// Parse ranges
		ranges := result.Get("ranges")
		if !ranges.IsUndefined() && !ranges.IsNull() {
			parseControlRanges(ranges, response.Ranges)
		}

		callback(response, nil)
	})
}

// parseCurrentValues parses current values from js.Value
func parseCurrentValues(v js.Value, target map[string]int32) {
	keys := js.Global().Get("Object").Call("keys", v)
	length := keys.Get("length").Int()

	for i := 0; i < length; i++ {
		key := ValueToString(keys.Index(i))
		value := int32(v.Get(key).Int())
		target[key] = value
	}
}

// parseControlRanges parses control ranges from js.Value
func parseControlRanges(v js.Value, target map[string]ControlRange) {
	keys := js.Global().Get("Object").Call("keys", v)
	length := keys.Get("length").Int()

	for i := 0; i < length; i++ {
		key := ValueToString(keys.Index(i))
		rangeValue := v.Get(key)

		target[key] = ControlRange{
			Min:     int32(rangeValue.Get("min").Int()),
			Max:     int32(rangeValue.Get("max").Int()),
			Current: int32(rangeValue.Get("current").Int()),
		}
	}
}

// ApplyCameraSettings applies settings to a camera
func ApplyCameraSettings(cameraID string, settings *CameraSettingsRequest, callback func(bool, error)) {
	DefaultAsyncClient.Post("/camera/settings/"+cameraID+"/apply", settings, func(result js.Value, err error) {
		if err != nil {
			callback(false, err)
			return
		}

		// Check if result is valid
		if result.IsUndefined() || result.IsNull() {
			callback(false, nil)
			return
		}

		// Check for status field (backend returns "applied" on success)
		statusField := result.Get("status")
		if !statusField.IsUndefined() && !statusField.IsNull() {
			status := statusField.String()
			success := (status == "applied" || status == "ok")
			callback(success, nil)
			return
		}

		// Fallback: check for success field
		successField := result.Get("success")
		if !successField.IsUndefined() && !successField.IsNull() {
			callback(successField.Bool(), nil)
			return
		}

		callback(false, nil)
	})
}

// GetCameraSettings retrieves saved settings for a camera
func GetCameraSettings(cameraID string, callback func(*CameraSettings, error)) {
	DefaultAsyncClient.Get("/camera/settings/"+cameraID, func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		settings := &CameraSettings{}
		if err := ParseJSONValue(result, settings); err != nil {
			callback(nil, err)
			return
		}

		callback(settings, nil)
	})
}

// SaveCameraSettings saves settings for a camera
func SaveCameraSettings(cameraID string, settings *CameraSettingsRequest, callback func(error)) {
	DefaultAsyncClient.Post("/camera/settings/"+cameraID, settings, func(result js.Value, err error) {
		callback(err)
	})
}
