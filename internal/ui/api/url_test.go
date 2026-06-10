package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAsyncClientURLConstruction tests that URL construction doesn't duplicate path segments
// This test runs on host system to validate URL construction logic
func TestAsyncClientURLConstruction(t *testing.T) {
	client := NewAsyncClient("/api/v1")

	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "captures endpoint",
			endpoint: "/captures",
			expected: "/api/v1/captures",
		},
		{
			name:     "captures with path",
			endpoint: "/captures/2026-06-08/test.jpg/devices",
			expected: "/api/v1/captures/2026-06-08/test.jpg/devices",
		},
		{
			name:     "devices endpoint",
			endpoint: "/devices",
			expected: "/api/v1/devices",
		},
		{
			name:     "cameras endpoint",
			endpoint: "/cameras",
			expected: "/api/v1/cameras",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify that baseURL + endpoint doesn't create duplicate /api/v1
			url := client.baseURL + tt.endpoint
			assert.NotContains(t, url, "/api/v1/api/v1", "URL should not contain duplicate /api/v1")
			assert.Equal(t, tt.expected, url)
		})
	}
}

// TestImageAdjustmentIsZero tests the IsZero method
func TestImageAdjustmentIsZero(t *testing.T) {
	adj := ImageAdjustment{}
	assert.True(t, adj.IsZero(), "Empty adjustment should be zero")

	adj.Brightness = 10
	assert.False(t, adj.IsZero(), "Adjustment with values should not be zero")

	adj2 := ImageAdjustment{Brightness: 0, Contrast: 0, Saturation: 0}
	assert.True(t, adj2.IsZero(), "All zeros should be zero")
}

// TestHTTPError tests HTTP error
func TestHTTPError(t *testing.T) {
	err := &HTTPError{Status: 404, Message: "Not Found"}
	assert.Equal(t, "HTTP error: Not Found", err.Error())

	errNoMsg := &HTTPError{Status: 500}
	assert.Contains(t, errNoMsg.Error(), "HTTP error")
}

// TestNetworkError tests network error
func TestNetworkError(t *testing.T) {
	err := &NetworkError{Message: "Connection failed"}
	assert.Equal(t, "Network error: Connection failed", err.Error())

	errNoMsg := &NetworkError{}
	assert.Equal(t, "Network error", errNoMsg.Error())
}

// TestJSONError tests JSON error
func TestJSONError(t *testing.T) {
	err := &JSONError{}
	assert.Equal(t, "Failed to parse JSON response", err.Error())
}
