//go:build js
// +build js

package api

import (
	"syscall/js"
)

// GetCaptures retrieves the list of captures
func GetCaptures(callback func([]Capture, error)) {
	DefaultAsyncClient.Get("/captures", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		capturesArray := result.Get("captures")
		if capturesArray.IsUndefined() || capturesArray.IsNull() {
			callback([]Capture{}, nil)
			return
		}

		captures := parseCapturesArray(capturesArray)
		callback(captures, nil)
	})
}

// parseCapturesArray parses a js.Value array into Capture slice
func parseCapturesArray(arr js.Value) []Capture {
	length := arr.Get("length").Int()
	captures := make([]Capture, length)

	for i := 0; i < length; i++ {
		captures[i] = parseCapture(arr.Index(i))
	}

	return captures
}

// parseCapture parses a js.Value into Capture struct
func parseCapture(v js.Value) Capture {
	return Capture{
		Path:       ValueToString(v.Get("path")),
		Filename:   ValueToString(v.Get("filename")),
		CameraID:   ValueToString(v.Get("camera_id")),
		CameraName: ValueToString(v.Get("camera_name")),
		Timestamp:  int64(v.Get("timestamp").Int()),
		Size:       int64(v.Get("size").Int()),
	}
}

// DeleteCapture deletes a capture
// Note: The endpoint uses query parameter for path
func DeleteCapture(path string, callback func(error)) {
	// URL encode the path
	DefaultAsyncClient.Delete("/captures?path="+path, func(result js.Value, err error) {
		callback(err)
	})
}

// GetDeviceCaptures retrieves captures for a specific device
func GetDeviceCaptures(deviceID string, callback func([]Capture, error)) {
	DefaultAsyncClient.Get("/devices/"+deviceID+"/captures", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		capturesArray := result.Get("captures")
		if capturesArray.IsUndefined() || capturesArray.IsNull() {
			callback([]Capture{}, nil)
			return
		}

		captures := parseCapturesArray(capturesArray)
		callback(captures, nil)
	})
}

// GetCaptureDeviceCaptures retrieves device-specific subimages for a full capture
func GetCaptureDeviceCaptures(capturePath string, callback func([]DeviceCaptureInfo, error)) {
	DefaultAsyncClient.Get("/captures/"+capturePath+"/devices", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		deviceCapturesArray := result.Get("device_captures")
		if deviceCapturesArray.IsUndefined() || deviceCapturesArray.IsNull() {
			callback([]DeviceCaptureInfo{}, nil)
			return
		}

		deviceCaptures := parseDeviceCaptureInfoArray(deviceCapturesArray)
		callback(deviceCaptures, nil)
	})
}

// parseDeviceCaptureInfoArray parses a js.Value array into DeviceCaptureInfo slice
func parseDeviceCaptureInfoArray(arr js.Value) []DeviceCaptureInfo {
	length := arr.Get("length").Int()
	captures := make([]DeviceCaptureInfo, length)

	for i := 0; i < length; i++ {
		captures[i] = parseDeviceCaptureInfo(arr.Index(i))
	}

	return captures
}

// parseDeviceCaptureInfo parses a js.Value into DeviceCaptureInfo struct
func parseDeviceCaptureInfo(v js.Value) DeviceCaptureInfo {
	return DeviceCaptureInfo{
		DeviceID:    ValueToString(v.Get("device_id")),
		Bounds:      parseBoundingBox(v.Get("bounds")),
		Subimage:    ValueToString(v.Get("subimage")),
		Adjustment:  parseImageAdjustment(v.Get("adjustment")),
		GeneratedAt: ValueToString(v.Get("generated_at")),
	}
}

// parseBoundingBox parses a js.Value into BoundingBox struct
func parseBoundingBox(v js.Value) BoundingBox {
	return BoundingBox{
		X:      v.Get("x").Float(),
		Y:      v.Get("y").Float(),
		Width:  v.Get("width").Float(),
		Height: v.Get("height").Float(),
	}
}

// parseImageAdjustment parses a js.Value into ImageAdjustment struct
func parseImageAdjustment(v js.Value) ImageAdjustment {
	return ImageAdjustment{
		Brightness: v.Get("brightness").Int(),
		Contrast:   v.Get("contrast").Int(),
		Saturation: v.Get("saturation").Int(),
	}
}
