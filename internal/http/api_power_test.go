package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/georgik/espbrew-go/internal/cluster"
	"github.com/georgik/espbrew-go/internal/config"
	"github.com/georgik/espbrew-go/internal/persistence"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

// newPowerTestLeader builds a leader with the given power-managed board
// configured. With DisableWatcher the reconcile pass creates a record for an
// absent board and marks it sleeping, so a no-op power-off succeeds without
// touching a real hub (the sleep manager returns early for a sleeping board).
func newPowerTestLeader(t *testing.T, devices ...config.DeviceConfig) (*cluster.LeaderNode, *persistence.Store) {
	t.Helper()
	store, err := persistence.Open(persistence.DefaultConfig(t.TempDir() + "/power.db"))
	require.NoError(t, err)

	leader := cluster.NewLeaderNode("test-leader", &cluster.LeaderConfig{
		HeartbeatInterval:  10 * time.Second,
		NodeTimeout:        30 * time.Second,
		HTTPPort:           8080,
		DisablemDNS:        true,
		DisableWatcher:     true,
		DisableMaintenance: true,
		DisableVirtual:     true,
		Devices:            devices,
	}, store)
	ctx := context.Background()
	require.NoError(t, leader.Start(ctx))
	t.Cleanup(func() { leader.Stop() })
	t.Cleanup(func() { store.Close() })
	return leader, store
}

func TestAPI_PowerDeviceRoute(t *testing.T) {
	leader, store := newPowerTestLeader(t, config.DeviceConfig{
		Path:        "/dev/serial/by-id/usb-powerboard-if00",
		ID:          "esp-aa:bb:cc:dd:ee:ff-if00",
		ChipType:    "ESP32-C3",
		Alias:       "power-board",
		USBLocation: "1-2",
		USBPort:     2,
	})

	handler := NewAPIHandler(leader, store)
	router := mux.NewRouter()
	router.SkipClean(true)
	api := router.PathPrefix("/api/v1").Subrouter()
	api.SkipClean(true)
	api.HandleFunc("/devices/{name}/power", handler.handleDevicePower).Methods("POST")

	// Power off an already-sleeping board: the leader accepts it (no-op) and
	// reports the board as powered off / sleeping. This exercises the handler's
	// success path and response shape end-to-end.
	w := doReq(t, router, "POST", "/api/v1/devices/power-board/power", `{"state":"off"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	if resp["power"] != false {
		t.Errorf("expected power=false, got %v", resp["power"])
	}
	if resp["sleeping"] != true {
		t.Errorf("expected sleeping=true, got %v", resp["sleeping"])
	}

	// Unknown state -> 400.
	w = doReq(t, router, "POST", "/api/v1/devices/power-board/power", `{"state":"sideways"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown state, got %d", w.Code)
	}

	// Unknown device -> 404.
	w = doReq(t, router, "POST", "/api/v1/devices/nope/power", `{"state":"on"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown device, got %d", w.Code)
	}
}

func TestParsePowerState(t *testing.T) {
	cases := map[string][2]bool{
		"on":    {true, true},
		"ON":    {true, true},
		" up ":  {true, true},
		"true":  {true, true},
		"1":     {true, true},
		"off":   {false, true},
		"down":  {false, true},
		"false": {false, true},
		"0":     {false, true},
		"":      {false, false},
		"maybe": {false, false},
	}
	for in, want := range cases {
		gotOn, gotOK := parsePowerState(in)
		if gotOn != want[0] || gotOK != want[1] {
			t.Errorf("parsePowerState(%q) = (%v,%v), want (%v,%v)", in, gotOn, gotOK, want[0], want[1])
		}
	}
}
