//go:build js
// +build js

package api

import (
	"encoding/json"
	"syscall/js"
)

// GetCameras retrieves the list of cameras
func GetCameras(callback func([]Camera, error)) {
	if DemoModeEnabled() {
		callback(mockCameras(), nil)
		return
	}

	DefaultAsyncClient.Get("/cameras", func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		camerasArray := result.Get("cameras")
		if camerasArray.IsUndefined() || camerasArray.IsNull() {
			callback([]Camera{}, nil)
			return
		}

		cameras := parseCamerasArray(camerasArray)
		callback(cameras, nil)
	})
}

// parseCamerasArray parses a js.Value array into Camera slice
func parseCamerasArray(arr js.Value) []Camera {
	length := arr.Get("length").Int()
	cameras := make([]Camera, length)

	for i := 0; i < length; i++ {
		cameras[i] = parseCamera(arr.Index(i))
	}

	return cameras
}

// parseCamera parses a js.Value into Camera struct
func parseCamera(v js.Value) Camera {
	return Camera{
		ID:      ValueToString(v.Get("id")),
		Name:    ValueToString(v.Get("name")),
		Path:    ValueToString(v.Get("path")),
		Status:  ValueToString(v.Get("status")),
		Backend: ValueToString(v.Get("backend")),
		NodeID:  ValueToString(v.Get("node_id")),
	}
}

// CaptureImage requests a new image capture
func CaptureImage(req CaptureRequest, callback func(*CaptureResponse, error)) {
	if DemoModeEnabled() {
		callback(mockCaptureResponse(), nil)
		return
	}

	DefaultAsyncClient.Post("/cameras/capture", req, func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		response := &CaptureResponse{}
		if err := ParseJSONValue(result, response); err != nil {
			callback(nil, err)
			return
		}

		callback(response, nil)
	})
}

// CapturePreview captures a preview image and returns the blob URL
func CapturePreview(req CaptureRequest, callback func(string, error)) {
	if DemoModeEnabled() {
		// In demo mode, return a mock preview image
		previewURL := mockImageURL("/camera-preview/" + req.CameraID)
		callback(previewURL, nil)
		return
	}

	url := DefaultAsyncClient.baseURL + "/cameras/capture"

	// Create request body
	bodyData := map[string]interface{}{
		"camera_id": req.CameraID,
		"width":     req.Width,
		"height":    req.Height,
		"quality":   req.Quality,
		"format":    req.Format,
		"preview":   true,
	}

	jsonData, err := json.Marshal(bodyData)
	if err != nil {
		callback("", err)
		return
	}

	// Create fetch options
	opts := js.Global().Get("Object").New()
	opts.Set("method", "POST")
	headers := js.Global().Get("Object").New()
	headers.Set("Content-Type", "application/json")
	opts.Set("headers", headers)
	opts.Set("body", string(jsonData))

	// Use fetch directly to get blob
	fetch := js.Global().Get("fetch")
	promise := fetch.Invoke(url, opts)

	thenFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) == 0 {
			callback("", &HTTPError{Status: 0, Message: "No response"})
			return nil
		}

		response := args[0]
		status := response.Get("status").Int()

		if status >= 400 {
			callback("", &HTTPError{Status: status, Message: "Capture failed"})
			return nil
		}

		// Get blob from response
		blobPromise := response.Call("blob")
		blobPromise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
				callback("", &HTTPError{Status: 0, Message: "No blob data"})
				return nil
			}

			blob := args[0]
			// Create object URL from blob
			objectURL := js.Global().Get("URL").Call("createObjectURL", blob)
			callback(objectURL.String(), nil)
			return nil
		}))

		return nil
	})

	catchFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		callback("", &HTTPError{Status: 0, Message: "Network error"})
		return nil
	})

	promise.Call("then", thenFunc).Call("catch", catchFunc)
}
