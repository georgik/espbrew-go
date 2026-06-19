//go:build js
// +build js

package api

import (
	"syscall/js"
)

// FlashUploadRequest represents a firmware upload request
type FlashUploadRequest struct {
	FileID string
	File   js.Value // File object from JavaScript
}

// FlashUploadResponse is the response from firmware upload
type FlashUploadResponse struct {
	FileID string `json:"file_id"`
	Size   int64  `json:"size"`
}

// FlashJobRequest represents a flash job submission
type FlashJobRequest struct {
	DevicePath string                 `json:"device_path"`
	FileID     string                 `json:"file_id"`
	Offset     int                    `json:"offset,omitempty"`
	Options    map[string]interface{} `json:"options,omitempty"`
}

// FlashJobResponse is the response from flash job submission
type FlashJobResponse struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	DevicePath string `json:"device_path"`
}

// FlashProgress represents flash progress update
type FlashProgress struct {
	JobID    string `json:"job_id"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

// FlashProgressCallback is called when progress updates
type FlashProgressCallback func(progress *FlashProgress)

// UploadFirmware uploads a firmware file
func UploadFirmware(file js.Value, callback func(response *FlashUploadResponse, err error)) {
	if DemoModeEnabled() {
		callback(mockFlashUploadResponse(), nil)
		return
	}

	if file.IsUndefined() || file.IsNull() {
		callback(nil, &NetworkError{Message: "No file provided"})
		return
	}

	// Create FormData
	formData := js.Global().Get("FormData").New()
	formData.Call("append", "firmware", file)

	// Create fetch options with body
	opts := js.Global().Get("Object").New()
	opts.Set("method", "POST")
	opts.Set("body", formData)

	// Make fetch call
	url := DefaultAsyncClient.baseURL + "/flash/upload"
	promise := js.Global().Call("fetch", url, opts)

	// Handle response
	thenFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			result := args[0]
			status := result.Get("status").Int()

			if status >= 400 {
				callback(nil, &HTTPError{Status: status})
				return nil
			}

			// Get JSON response
			jsonPromise := result.Call("json")
			jsonPromise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				if len(args) > 0 && !args[0].IsUndefined() {
					var resp FlashUploadResponse
					if err := ParseJSONValue(args[0], &resp); err != nil {
						callback(nil, err)
					} else {
						callback(&resp, nil)
					}
				}
				return nil
			}))
		}
		return nil
	})

	catchFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		callback(nil, &NetworkError{})
		return nil
	})

	promise.Call("then", thenFunc).Call("catch", catchFunc)
}

// SubmitFlashJob submits a flash job
func SubmitFlashJob(req *FlashJobRequest, callback func(response *FlashJobResponse, err error)) {
	if DemoModeEnabled() {
		callback(mockFlashJobResponse(), nil)
		return
	}

	DefaultAsyncClient.Post("/flash", req, func(result js.Value, err error) {
		if err != nil {
			callback(nil, err)
			return
		}

		var resp FlashJobResponse
		if err := ParseJSONValue(result, &resp); err != nil {
			callback(nil, err)
		} else {
			callback(&resp, nil)
		}
	})
}

// WatchFlashProgress watches flash progress via polling (WebSocket alternative for WASM)
func WatchFlashProgress(jobID string, callback FlashProgressCallback) {
	if DemoModeEnabled() {
		// In demo mode, immediately return completed progress
		callback(mockFlashProgress(jobID))
		return
	}

	pollProgress(jobID, 0, callback)
}

func pollProgress(jobID string, attempt int, callback FlashProgressCallback) {
	// Poll every 500ms
	js.Global().Get("setTimeout").Invoke(js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fetchProgress(jobID, callback, func(done bool) {
			if !done {
				pollProgress(jobID, attempt+1, callback)
			}
		})
		return nil
	}), 500)
}

func fetchProgress(jobID string, callback FlashProgressCallback, doneCallback func(bool)) {
	// Use jobs endpoint with query parameter
	url := DefaultAsyncClient.baseURL + "/jobs?id=" + jobID
	opts := js.Global().Get("Object").New()
	opts.Set("method", "GET")

	promise := js.Global().Call("fetch", url, opts)

	thenFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			result := args[0]
			status := result.Get("status").Int()

			if status >= 400 {
				// Error or not found
				callback(&FlashProgress{
					JobID:  jobID,
					Status: "error",
					Error:  "Failed to fetch progress",
				})
				doneCallback(true)
				return nil
			}

			// Get JSON response - returns array of jobs
			jsonPromise := result.Call("json")
			jsonPromise.Call("then", js.FuncOf(func(this js.Value, args2 []js.Value) interface{} {
				if len(args2) > 0 && !args2[0].IsUndefined() {
					arrVal := args2[0]
					// Check if it's an array
					if arrVal.Get("length").Int() > 0 {
						firstJob := arrVal.Index(0)
						// Map job status to FlashProgress
						status := firstJob.Get("status").String()
						progress := firstJob.Get("progress").Int()

						progressObj := &FlashProgress{
							JobID:    jobID,
							Status:   status,
							Progress: progress,
							Message:  "", // Could be added from job data if available
						}

						callback(progressObj)

						// Stop polling if complete or error
						if status == "completed" || status == "succeeded" || status == "error" || status == "failed" {
							doneCallback(true)
						} else {
							doneCallback(false)
						}
					} else {
						// Empty array - job not found
						callback(&FlashProgress{
							JobID:  jobID,
							Status: "error",
							Error:  "Job not found",
						})
						doneCallback(true)
					}
				}
				return nil
			}))
		}
		return nil
	})

	catchFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		callback(&FlashProgress{
			JobID:  jobID,
			Status: "error",
			Error:  "Network error",
		})
		doneCallback(true)
		return nil
	})

	promise.Call("then", thenFunc).Call("catch", catchFunc)
}
