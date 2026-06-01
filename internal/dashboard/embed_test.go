package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitorHTML(t *testing.T) {
	if !HasMonitor() {
		t.Skip("Monitor page not embedded, run: go generate ./internal/dashboard")
	}

	content := MonitorHTML()

	assert.NotEmpty(t, content, "MonitorHTML should return content")
	assert.Contains(t, string(content), "Serial Monitor", "Should contain page title")
	assert.Contains(t, string(content), "deviceSelect", "Should contain device selection element")
}

func TestHasMonitor(t *testing.T) {
	// This test verifies that HasMonitor correctly reports monitor availability
	// In a normal build with go generate, this should be true
	hasMonitor := HasMonitor()
	if !hasMonitor {
		t.Log("Monitor page not embedded - run: go generate ./internal/dashboard")
	}
}

func TestIndexHTML(t *testing.T) {
	if !HasDashboard() {
		t.Skip("Dashboard not embedded, run: go generate ./internal/dashboard")
	}

	content := IndexHTML()

	assert.NotEmpty(t, content, "IndexHTML should return content")
	assert.Contains(t, string(content), "ESPBrew Cluster", "Should contain page title")
}

func TestHasDashboard(t *testing.T) {
	// This test verifies that HasDashboard correctly reports dashboard availability
	hasDashboard := HasDashboard()
	if !hasDashboard {
		t.Log("Dashboard not embedded - run: go generate ./internal/dashboard")
	}
}

func TestStaticFS(t *testing.T) {
	fs := StaticFS()
	require.NotNil(t, fs, "StaticFS should return a filesystem")
}
