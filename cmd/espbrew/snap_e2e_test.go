//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	e2eProjectDir = "/home/georgik/projects/esp32-conways-game-of-life-rs/esp32-s3-box-3"
	e2eServerPort = 18080 // Use non-standard port to avoid conflicts
)

var (
	e2eServerAddr  = fmt.Sprintf("localhost:%d", e2eServerPort)
	e2eServerURL   = fmt.Sprintf("http://%s", e2eServerAddr)
	e2eSnapTimeout = 60 * time.Second
)

// TestE2E_SnapWithCluster tests the full snap workflow against a real cluster server.
// Prerequisites:
// - ESP32-S3-Box-3 device connected via USB
// - Project at e2eProjectDir must be built (cargo build --release)
// - Device must be available at /dev/ttyACM0 or /dev/ttyUSB0
//
// Run with: go test -tags=e2e -v ./cmd/espbrew -run TestE2E_SnapWithCluster
func TestE2E_SnapWithCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if project directory exists
	if _, err := os.Stat(e2eProjectDir); os.IsNotExist(err) {
		t.Skipf("Project directory not found: %s", e2eProjectDir)
	}

	// Find firmware binary
	firmware, err := findE2EFirmware()
	if err != nil {
		t.Skipf("Cannot find firmware binary: %v", err)
	}
	t.Logf("Using firmware: %s", firmware)

	// Start cluster server in background
	serverCtx, serverCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer serverCancel()

	serverCmd, err := startE2EServer(serverCtx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		shutdownE2EServer(t)
		serverCmd.Wait()
	}()

	// Wait for server to be ready
	if !waitForE2EServer(t, 30*time.Second) {
		t.Fatal("Server did not become ready in time")
	}

	// Find available device
	deviceID := findE2EDevice(t)
	if deviceID == "" {
		t.Skip("No available ESP32 device found")
	}
	t.Logf("Using device: %s", deviceID)

	// Run snap test
	t.Run("snap_with_skip_flash", func(t *testing.T) {
		testE2ESnapSkipFlash(t, deviceID)
	})

	t.Run("snap_with_flash", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping flash test in short mode")
		}
		testE2ESnapWithFlash(t, deviceID, firmware)
	})
}

// findE2EFirmware locates the firmware binary in the project directory.
func findE2EFirmware() (string, error) {
	// Check common build directories
	buildDirs := []string{
		filepath.Join(e2eProjectDir, "target", "xtensa-esp32s3-none-elf", "release"),
		filepath.Join(e2eProjectDir, "target", "release"),
	}

	for _, buildDir := range buildDirs {
		entries, err := os.ReadDir(buildDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			// Look for ELF binaries (no extension)
			if !entry.IsDir() && !strings.Contains(entry.Name(), ".") {
				// Check if it's executable or large enough
				info, _ := entry.Info()
				if info.Mode().Perm()&0111 != 0 || info.Size() > 100000 {
					return filepath.Join(buildDir, entry.Name()), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no firmware binary found")
}

// startE2EServer starts the cluster server in background with dev mode.
func startE2EServer(ctx context.Context) (*exec.Cmd, error) {
	// Build the server first
	buildCmd := exec.Command("go", "build", "-o", "/tmp/espbrew-e2e", "./cmd/espbrew")
	if _, err := buildCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build server: %w", err)
	}

	// Start server with dev mode
	cmd := exec.CommandContext(ctx, "/tmp/espbrew-e2e", "cluster",
		"--dev-mode",
		"--port", fmt.Sprint(e2eServerPort),
		"--role", "standalone",
		"--log-level", "info")

	// Capture server output for debugging
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}

	return cmd, nil
}

// waitForE2EServer waits for the server to be ready.
func waitForE2EServer(t *testing.T, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(e2eServerURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return false
}

// shutdownE2EServer gracefully shuts down the E2E server.
func shutdownE2EServer(t *testing.T) {
	t.Helper()

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("POST", e2eServerURL+"/api/v1/dev/shutdown", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Shutdown request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Logf("Shutdown returned status: %d", resp.StatusCode)
	}
}

// findE2EDevice finds an available ESP32 device.
func findE2EDevice(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/tmp/espbrew-e2e", "--cluster", e2eServerURL, "device", "list")
	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to list devices: %v", err)
		return ""
	}

	// Parse output for available device
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "available") || strings.Contains(line, "/dev/tty") {
			// Extract device path
			fields := strings.Fields(line)
			for _, field := range fields {
				if strings.HasPrefix(field, "/dev/tty") {
					return field
				}
			}
		}
	}

	return ""
}

// testE2ESnapSkipFlash tests snap operation without flashing.
func testE2ESnapSkipFlash(t *testing.T, deviceID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), e2eSnapTimeout)
	defer cancel()

	// Build snap command
	args := []string{
		"--cluster", e2eServerURL,
		"--device", deviceID,
		"--skip-flash",
		"--duration", "5", // 5 seconds for faster testing
		"--output", "json",
		snapCmd.Name(),
	}

	cmd := exec.CommandContext(ctx, "/tmp/espbrew-e2e", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("Snap output: %s", string(output))
		t.Fatalf("Snap command failed: %v", err)
	}

	// Parse JSON output
	var result struct {
		SnapID string `json:"snap_id"`
		Status string `json:"status"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		t.Logf("Snap output: %s", string(output))
		t.Fatalf("Failed to parse snap output: %v", err)
	}

	// Validate results
	if result.SnapID == "" {
		t.Error("Snap ID should not be empty")
	}

	if result.Status != "success" && result.Status != "partial" {
		t.Errorf("Expected success/partial status, got: %s", result.Status)
	}

	t.Logf("Snap completed successfully: ID=%s Status=%s", result.SnapID, result.Status)
}

// testE2ESnapWithFlash tests snap operation with flashing.
func testE2ESnapWithFlash(t *testing.T, deviceID, firmware string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Build snap command with flash
	args := []string{
		"--cluster", e2eServerURL,
		"--device", deviceID,
		"--firmware", firmware,
		"--force-flash", // Force flash to ensure it runs
		"--duration", "5",
		"--output", "json",
		snapCmd.Name(),
	}

	cmd := exec.CommandContext(ctx, "/tmp/espbrew-e2e", args...)

	// Use pipe for output to monitor progress
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start snap command: %v", err)
	}

	// Monitor output
	var allOutput strings.Builder
	go io.Copy(&allOutput, stdout)
	go io.Copy(&allOutput, stderr)

	// Wait for completion
	err := cmd.Wait()

	output := allOutput.String()
	t.Logf("Snap with flash output: %s", output)

	if err != nil {
		t.Fatalf("Snap with flash command failed: %v", err)
	}

	// Verify snap completed
	if !strings.Contains(output, "Snap ID:") {
		t.Error("Expected snap ID in output")
	}

	t.Log("Snap with flash completed successfully")
}
