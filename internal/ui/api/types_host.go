//go:build !js
// +build !js

package api

// ImageAdjustment represents image adjustments for a region
type ImageAdjustment struct {
	Brightness int `json:"brightness,omitempty"`
	Contrast   int `json:"contrast,omitempty"`
	Saturation int `json:"saturation,omitempty"`
}

// IsZero returns true if all adjustments are zero
func (a *ImageAdjustment) IsZero() bool {
	return a.Brightness == 0 && a.Contrast == 0 && a.Saturation == 0
}

// HTTPError represents an HTTP error response
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return "HTTP error: " + e.Message
	}
	return "HTTP error"
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

// AsyncClient handles async API communication (host version for testing)
type AsyncClient struct {
	baseURL string
}

// NewAsyncClient creates a new async API client
func NewAsyncClient(baseURL string) *AsyncClient {
	return &AsyncClient{
		baseURL: baseURL,
	}
}
