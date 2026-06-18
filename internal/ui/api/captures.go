//go:build js
// +build js

package api

import (
	"syscall/js"
)

// GetCaptures retrieves the list of captures
func GetCaptures(callback func([]Capture, error)) {
	GetCapturesPaginated(1, 40, callback)
}

// GetCapturesPaginated retrieves captures with pagination
func GetCapturesPaginated(page, limit int, callback func([]Capture, error)) {
	if DemoModeEnabled() {
		captures, _, _ := mockCapturesPaginated(page, limit)
		callback(captures, nil)
		return
	}

	url := "/captures?page=" + js.Global().Get("encodeURIComponent").Invoke(js.ValueOf(page)).String()
	url += "&limit=" + js.Global().Get("encodeURIComponent").Invoke(js.ValueOf(limit)).String()

	DefaultAsyncClient.Get(url, func(result js.Value, err error) {
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

// GetCapturesMeta retrieves captures with pagination metadata
func GetCapturesMeta(page, limit int, callback func([]Capture, int, int, error)) {
	if DemoModeEnabled() {
		captures, total, totalPages := mockCapturesPaginated(page, limit)
		callback(captures, total, totalPages, nil)
		return
	}

	url := "/captures?page=" + js.Global().Get("encodeURIComponent").Invoke(js.ValueOf(page)).String()
	url += "&limit=" + js.Global().Get("encodeURIComponent").Invoke(js.ValueOf(limit)).String()

	DefaultAsyncClient.Get(url, func(result js.Value, err error) {
		if err != nil {
			callback(nil, 0, 0, err)
			return
		}

		capturesArray := result.Get("captures")
		if capturesArray.IsUndefined() || capturesArray.IsNull() {
			callback([]Capture{}, 0, 0, nil)
			return
		}

		captures := parseCapturesArray(capturesArray)
		total := int(result.Get("total").Int())
		totalPages := int(result.Get("total_pages").Int())

		callback(captures, total, totalPages, nil)
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
// The endpoint is DELETE /captures/{path} where path is the relative path within captures directory
// Full paths like "/captures/2026-06-15/file.jpg" should have "/captures/" stripped first
func DeleteCapture(path string, callback func(error)) {
	if DemoModeEnabled() {
		callback(nil)
		return
	}

	// Strip /captures/ prefix if present
	strippedPath := path
	if len(strippedPath) > 10 && strippedPath[:10] == "/captures/" {
		strippedPath = strippedPath[10:]
	}
	// Strip leading slash if present (now relative path)
	if len(strippedPath) > 0 && strippedPath[0] == '/' {
		strippedPath = strippedPath[1:]
	}
	// URL encode the path for the route parameter
	encodedPath := js.Global().Get("encodeURIComponent").Invoke(strippedPath).String()
	DefaultAsyncClient.Delete("/captures/"+encodedPath, func(result js.Value, err error) {
		callback(err)
	})
}

// GetDeviceCaptures retrieves captures for a specific device
func GetDeviceCaptures(deviceID string, callback func([]Capture, error)) {
	if DemoModeEnabled() {
		callback(mockDeviceCaptures(deviceID), nil)
		return
	}

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
	if DemoModeEnabled() {
		callback(mockCaptureDeviceCaptures(capturePath), nil)
		return
	}

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
