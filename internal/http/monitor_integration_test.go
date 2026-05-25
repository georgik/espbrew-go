//go:build integration
// +build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"codeberg.org/georgik/projects/espbrew-go/internal/cluster"
	"codeberg.org/georgik/projects/espbrew-go/internal/device"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMonitorWebSocket_DataFlow tests the complete data flow:
// serial device → monitor server → WebSocket → client
func TestMonitorWebSocket_DataFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Find an available ESP device
	scanner := device.NewScanner()
	espPorts, err := scanner.ScanESP()
	if err != nil || len(espPorts) == 0 {
		t.Skip("No ESP device available for integration test")
	}

	port := espPorts[0]
	log.Info().Str("port", port.Path).Msg("Using device for integration test")

	// Create a minimal leader node
	leader := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:          8081,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})
	require.NoError(t, leader.Start(context.Background()))
	defer leader.Stop()

	// Register the device
	leader.RegisterDevice(&cluster.DeviceInfo{
		Path:   port.Path,
		VID:    port.VID,
		PID:    port.PID,
		Status: "available",
	})

	// Start HTTP server
	server := NewServer("127.0.0.1:18081", leader)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create monitor client
	deviceName := cluster.DeviceName(port.Path)
	cfg := cluster.MonitorConfig{
		Baud:     115200,
		Reset:    true, // Reset to get boot messages
		Duration: 5 * time.Second,
	}

	client := cluster.NewMonitorClient("http://127.0.0.1:18081", port.Path, cfg)

	// Connect and receive data
	dataCh := make(chan []byte, 100)
	errorCh := make(chan error, 1)

	streamCtx, streamCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer streamCancel()

	err = client.Stream(streamCtx, dataCh, errorCh)
	if err != nil {
		t.Fatalf("Failed to start monitor stream: %v", err)
	}
	defer client.Close()

	// Collect received data
	var receivedData [][]byte
	doneCh := make(chan struct{})

	go func() {
		for {
			select {
			case data, ok := <-dataCh:
				if !ok {
					close(doneCh)
					return
				}
				if len(data) > 0 {
					receivedData = append(receivedData, data)
					t.Logf("Received %d bytes: %q", len(data), string(data))
				}
			case err := <-errorCh:
				t.Logf("Stream error: %v", err)
				close(doneCh)
				return
			case <-streamCtx.Done():
				close(doneCh)
				return
			}
		}
	}()

	// Wait for some data or timeout
	select {
	case <-doneCh:
	case <-time.After(8 * time.Second):
		t.Log("Test timeout - collecting received data")
	}

	// Verify we received some data
	if len(receivedData) == 0 {
		t.Error("No data received from device")
	} else {
		t.Logf("Successfully received %d data chunks from device", len(receivedData))

		// Check for expected boot strings
		fullData := string(concatBytes(receivedData))
		t.Logf("Total data length: %d bytes", len(fullData))

		// Look for common ESP boot strings
		expectedStrings := []string{"ESP", "ROM"}
		foundAny := false
		for _, expected := range expectedStrings {
			if contains(fullData, expected) {
				t.Logf("Found expected string: %q", expected)
				foundAny = true
			}
		}

		if !foundAny {
			t.Errorf("Data does not contain expected boot strings (ESP, ROM). Got: %q", fullData)
		}
	}

	// Clean shutdown
	streamCancel()
	client.Close()
}

func TestMonitorWebSocket_ResetWithDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	scanner := device.NewScanner()
	espPorts, err := scanner.ScanESP()
	if err != nil || len(espPorts) == 0 {
		t.Skip("No ESP device available for integration test")
	}

	port := espPorts[0]
	log.Info().Str("port", port.Path).Msg("Testing reset functionality")

	leader := cluster.NewLeaderNode("test", &cluster.LeaderConfig{
		HTTPPort:          8082,
		DisablemDNS:       true,
		HeartbeatInterval: time.Second,
		NodeTimeout:       5 * time.Second,
	})
	require.NoError(t, leader.Start(context.Background()))
	defer leader.Stop()

	leader.RegisterDevice(&cluster.DeviceInfo{
		Path:   port.Path,
		VID:    port.VID,
		PID:    port.PID,
		Status: "available",
	})

	server := NewServer("127.0.0.1:18082", leader)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, server.Start(ctx))
	time.Sleep(100 * time.Millisecond)

	// Test reset via WebSocket
	deviceName := cluster.DeviceName(port.Path)
	wsURL := fmt.Sprintf("ws://127.0.0.1:18082/api/v1/monitor/%s?reset=1", deviceName)

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Receive messages
	resetComplete := false
	dataReceived := false

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	for i := 0; i < 50; i++ {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			break
		}

		msgType, _ := msg["type"].(string)
		t.Logf("Received message type: %s", msgType)

		switch msgType {
		case "monitor_start":
			// Expected
		case "reset_complete":
			resetComplete = true
			t.Log("Reset completed successfully")
		case "data":
			dataReceived = true
			t.Log("Data received after reset")
		case "error":
			t.Fatalf("Received error: %v", msg["error"])
		}
	}

	// Verify reset was triggered
	if !resetComplete {
		t.Error("Reset complete message not received")
	}

	// We expect some data after reset (boot messages)
	if !dataReceived {
		t.Log("Warning: No data received after reset (device may be slow)")
	}
}

func concatBytes(slices [][]byte) []byte {
	var total int
	for _, s := range slices {
		total += len(s)
	}
	result := make([]byte, total)
	var i int
	for _, s := range slices {
		i += copy(result[i:], s)
	}
	return result
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestMain allows running integration tests directly
func TestMain(m *testing.M) {
	// Set log level for tests
	os.Setenv("ESPBREW_LOG_LEVEL", "debug")

	os.Exit(m.Run())
}
