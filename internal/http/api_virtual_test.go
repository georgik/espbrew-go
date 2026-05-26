package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codeberg.org/georgik/espbrew-go/internal/cluster"
)

func TestHandleDevicesIncludesVirtual(t *testing.T) {
	ctx := context.Background()

	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
		HTTPPort:          8080,
		DisablemDNS:       true,
		DisableWatcher:    true,
	})

	if err := leader.Start(ctx); err != nil {
		t.Fatalf("Failed to start leader: %v", err)
	}
	defer leader.Stop()

	// Wait for virtual device registration
	time.Sleep(200 * time.Millisecond)

	handler := NewAPIHandler(leader)

	req := httptest.NewRequest("GET", "/api/v1/devices", nil)
	w := httptest.NewRecorder()

	handler.handleDevices(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var devices []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&devices); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should include virtual devices
	virtualFound := map[string]bool{
		"wokwi-esp32s3": false,
		"wokwi-esp32":   false,
		"wokwi-esp32c3": false,
		"wokwi-esp32c6": false,
	}

	for _, dev := range devices {
		path, ok := dev["path"].(string)
		if !ok {
			continue
		}
		if _, exists := virtualFound[path]; exists {
			virtualFound[path] = true

			// Check virtual flag is set
			if dev["virtual"] != true {
				t.Errorf("Device %q missing virtual flag", path)
			}
		}
	}

	for path, found := range virtualFound {
		if !found {
			t.Errorf("Virtual device %q not found in response", path)
		}
	}

	t.Logf("Found %d devices including virtual", len(devices))
}
