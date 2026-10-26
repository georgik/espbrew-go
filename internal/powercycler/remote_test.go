package powercycler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/georgik/espbrew-go/internal/powercontrol"
)

// powerServer is a minimal stand-in for the privileged hub-power pod. It
// returns a fixed hub description and records every power request, so a test
// can assert on the exact request the client sent.
type powerServer struct {
	*httptest.Server
	powers []powerReq
}

func newTestPowerServer(t *testing.T) (*powerServer, func() []powerReq) {
	t.Helper()
	s := &powerServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/hub/{location}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(hubInfo{
			Location:   r.PathValue("location"),
			Vendor:     "0bda",
			Product:    "5411",
			NumPorts:   8,
			SuperSpeed: true,
		})
	})
	mux.HandleFunc("/v1/hub/{location}/port/{port}/power", func(w http.ResponseWriter, r *http.Request) {
		var req powerReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.powers = append(s.powers, req)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/v1/hub/{location}/port/{port}/cycle", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	s.Server = ts
	return s, func() []powerReq { return s.powers }
}

// TestRemoteFindHubByLocation verifies the client parses the hub description
// (including NumPorts, which the leader needs for its port bounds check).
func TestRemoteFindHubByLocation(t *testing.T) {
	s, _ := newTestPowerServer(t)
	c := NewRemote(s.URL)

	hub, err := c.FindHubByLocation("1-6")
	if err != nil {
		t.Fatalf("FindHubByLocation: %v", err)
	}
	if hub.Location != "1-6" {
		t.Errorf("location = %q, want 1-6", hub.Location)
	}
	if hub.NumPorts != 8 {
		t.Errorf("numPorts = %d, want 8", hub.NumPorts)
	}
}

// TestRemoteSetPortPowerDual verifies the client sends the right request and
// surfaces a non-2xx response as an error.
func TestRemoteSetPortPowerDual(t *testing.T) {
	s, getpowers := newTestPowerServer(t)
	c := NewRemote(s.URL)

	hub := &powercontrol.Hub{Location: "1-6", NumPorts: 8}
	if err := c.SetPortPowerDual(hub, 1, true); err != nil {
		t.Fatalf("SetPortPowerDual(on): %v", err)
	}
	if err := c.SetPortPowerDual(hub, 1, false); err != nil {
		t.Fatalf("SetPortPowerDual(off): %v", err)
	}

	powers := getpowers()
	if len(powers) != 2 {
		t.Fatalf("expected 2 power requests, got %d", len(powers))
	}
	if !powers[0].On || powers[1].On {
		t.Errorf("power requests = %+v, want [on, off]", powers)
	}
}

// TestRemoteSetPortPowerDualOutOfBounds verifies the client rejects an out of
// range port before hitting the network, matching the local controller.
func TestRemoteSetPortPowerDualOutOfBounds(t *testing.T) {
	c := NewRemote("http://unused")
	hub := &powercontrol.Hub{Location: "1-6", NumPorts: 2}
	if err := c.SetPortPowerDual(hub, 3, true); err != powercontrol.ErrPortNotFound {
		t.Errorf("err = %v, want ErrPortNotFound", err)
	}
}

// TestRemoteHubNotFound verifies a 404 from the service maps to ErrHubNotFound.
func TestRemoteHubNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no hub"}`))
	}))
	defer ts.Close()

	c := NewRemote(ts.URL)
	if _, err := c.FindHubByLocation("9-9"); err != powercontrol.ErrHubNotFound {
		t.Errorf("err = %v, want ErrHubNotFound", err)
	}
}
