//go:build js
// +build js

package api

import (
	"encoding/json"
	"fmt"
	"syscall/js"
)

// AsyncCallback is called when async operation completes
type AsyncCallback func(result js.Value, err error)

// AsyncClient handles async API communication
type AsyncClient struct {
	baseURL string
}

// NewAsyncClient creates a new async API client
func NewAsyncClient(baseURL string) *AsyncClient {
	return &AsyncClient{
		baseURL: baseURL,
	}
}

// DefaultAsyncClient for the application
var DefaultAsyncClient = NewAsyncClient("/api/v1")

// Get makes an async GET request
func (c *AsyncClient) Get(endpoint string, callback AsyncCallback) {
	c.Request("GET", endpoint, nil, callback)
}

// Post makes an async POST request
func (c *AsyncClient) Post(endpoint string, body interface{}, callback AsyncCallback) {
	c.Request("POST", endpoint, body, callback)
}

// Put makes an async PUT request
func (c *AsyncClient) Put(endpoint string, body interface{}, callback AsyncCallback) {
	c.Request("PUT", endpoint, body, callback)
}

// Delete makes an async DELETE request
func (c *AsyncClient) Delete(endpoint string, callback AsyncCallback) {
	c.Request("DELETE", endpoint, nil, callback)
}

// Patch makes an async PATCH request
func (c *AsyncClient) Patch(endpoint string, body interface{}, callback AsyncCallback) {
	c.Request("PATCH", endpoint, body, callback)
}

// Request makes an async HTTP request
func (c *AsyncClient) Request(method, endpoint string, body interface{}, callback AsyncCallback) {
	url := c.baseURL + endpoint

	// Create fetch options
	opts := js.Global().Get("Object").New()
	opts.Set("method", method)

	// Set headers
	headers := js.Global().Get("Object").New()
	headers.Set("Content-Type", "application/json")
	opts.Set("headers", headers)

	// Add body for POST/PUT/PATCH
	if body != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		jsonData, err := json.Marshal(body)
		if err != nil {
			callback(js.Value{}, err)
			return
		}
		opts.Set("body", string(jsonData))
	}

	// Create promise handlers
	thenFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			result := args[0]
			status := result.Get("status").Int()

			if status >= 400 {
				// Try to get error message from body
				jsonPromise := result.Call("json")
				jsonPromise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
					if len(args) > 0 && !args[0].IsUndefined() {
						// Check for error field in response
						if errField := args[0].Get("error"); !errField.IsUndefined() {
							callback(js.Value{}, &HTTPError{
								Status:  status,
								Message: errField.String(),
							})
							return nil
						}
					}
					callback(js.Value{}, &HTTPError{Status: status})
					return nil
				}))
				return nil
			}

			// Get JSON response
			jsonPromise := result.Call("json")
			jsonPromise.Call("then", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				if len(args) > 0 {
					callback(args[0], nil)
				} else {
					callback(js.Value{}, &JSONError{})
				}
				return nil
			}))
		}
		return nil
	})

	catchFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			callback(js.Value{}, &NetworkError{Message: args[0].String()})
		} else {
			callback(js.Value{}, &NetworkError{})
		}
		return nil
	})

	// Make fetch call
	js.Global().Call("fetch", url, opts).Call("then", thenFunc).Call("catch", catchFunc)
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("HTTP error %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("HTTP error %d", e.Status)
}

// NetworkError represents a network error
type NetworkError struct {
	Message string
}

func (e *NetworkError) Error() string {
	if e.Message != "" {
		return "Network error: " + e.Message
	}
	return "Network error"
}

// JSONError represents a JSON parsing error
type JSONError struct{}

func (e *JSONError) Error() string {
	return "Failed to parse JSON response"
}

// ValueToInt converts a js.Value to int
func ValueToInt(v js.Value) int {
	if v.IsUndefined() || v.IsNull() {
		return 0
	}
	return v.Int()
}

// ValueToString converts a js.Value to string
func ValueToString(v js.Value) string {
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	return v.String()
}

// ValueToBool converts a js.Value to bool
func ValueToBool(v js.Value) bool {
	if v.IsUndefined() || v.IsNull() {
		return false
	}
	return v.Bool()
}

// ValueToArray converts a js.Value to array of js.Value
func ValueToArray(v js.Value) []js.Value {
	if v.IsUndefined() || v.IsNull() {
		return []js.Value{}
	}

	length := v.Get("length").Int()
	result := make([]js.Value, length)

	for i := 0; i < length; i++ {
		result[i] = v.Index(i)
	}

	return result
}

// ParseJSONValue parses a js.Value into a Go struct
func ParseJSONValue(v js.Value, target interface{}) error {
	jsonStr := js.Global().Get("JSON").Call("stringify", v).String()
	return json.Unmarshal([]byte(jsonStr), target)
}
