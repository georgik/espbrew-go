// Package powercycler provides a devicesleep.Controller implementation that
// drives USB hub port power over HTTP, so the espbrew leader can power-cycle
// boards without running the privileged sysfs write itself.
//
// Why this exists: flipping a USB hub port's power is the ONE operation in the
// whole system that the kernel refuses to an unprivileged container. Writing the
// sysfs per-port "disable" file (internal/powercontrol) requires a privileged
// pod — a device-cgroup allow rule (like the one the serial Device Plugin
// grants for /dev/ttyACM*) cannot help, because sysfs is not a device node and
// the rejection is a capability/mount check, not an open() check. See
// wiki/non-privileged-power-control-feasibility-report.md.
//
// So the actual port-power switch runs in a tiny, dedicated, privileged
// "hub-power" service (cmd/espbrew/hubpower.go, deployed as a privileged pod).
// This package is the CLIENT side: it speaks a small HTTP API to that service
// and otherwise behaves exactly like the local controller to the device-sleep
// subsystem, so the leader code is unchanged. A leader configured with
// ESPBREW_POWER_MODE=remote simply talks to the pod; a leader left on the
// default "direct" mode keeps using the in-process controller.
package powercycler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/georgik/espbrew-go/internal/powercontrol"
)

// defaultBaseURL is the in-cluster DNS of the hub-power Service. It is used
// when neither the config field nor the ESPBREW_POWER_REMOTE_URL env var is
// set, so a bare "remote" mode still points at the right place.
const defaultBaseURL = "http://espbrew-hub-power:8080"

// RemoteController is a devicesleep.Controller that switches hub port power by
// calling a remote (privileged) power-cycler service over HTTP. It implements
// the same two methods the device-sleep subsystem needs, so it can be dropped
// in wherever powercontrol.NewController() is used today.
type RemoteController struct {
	baseURL    string
	httpClient *http.Client
}

// NewRemote returns a RemoteController that calls the power-cycler service at
// baseURL. Constructing it performs NO network I/O, so it is safe to build up
// front and let individual calls fail later: the leader treats a power-cycle
// failure as non-fatal and falls back to the DTR/RTS reset (see
// wakeDeviceForFlash).
func NewRemote(baseURL string) *RemoteController {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &RemoteController{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// hubInfo is the JSON shape returned by GET /v1/hub/{location}.
type hubInfo struct {
	Location   string `json:"location"`
	Vendor     string `json:"vendor"`
	Product    string `json:"product"`
	NumPorts   int    `json:"numPorts"`
	SuperSpeed bool   `json:"superSpeed"`
}

// powerReq is the JSON body of a power request.
type powerReq struct {
	On bool `json:"on"`
}

// BaseURL returns the remote power-cycler endpoint this controller targets.
func (r *RemoteController) BaseURL() string { return r.baseURL }

// FindHubByLocation resolves the hub at loc by asking the remote service. The
// returned Hub carries the real NumPorts so the caller's port bounds check in
// SetPortPowerDual passes. A missing hub maps to powercontrol.ErrHubNotFound.
func (r *RemoteController) FindHubByLocation(loc string) (*powercontrol.Hub, error) {
	resp, err := r.httpClient.Get(r.baseURL + "/v1/hub/" + url.PathEscape(loc))
	if err != nil {
		return nil, fmt.Errorf("query hub %q: %w", loc, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, powercontrol.ErrHubNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query hub %q: unexpected status %d", loc, resp.StatusCode)
	}

	var info hubInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode hub %q: %w", loc, err)
	}
	return &powercontrol.Hub{
		Location:   info.Location,
		Vendor:     info.Vendor,
		Product:    info.Product,
		NumPorts:   info.NumPorts,
		SuperSpeed: info.SuperSpeed,
	}, nil
}

// SetPortPowerDual switches power for a port by asking the remote service. It
// forwards location, port and requested state; the remote pod performs the
// real (privileged) sysfs switch, including dual-interface hub linking.
func (r *RemoteController) SetPortPowerDual(hub *powercontrol.Hub, port int, on bool) error {
	if hub == nil {
		return powercontrol.ErrHubNotFound
	}
	if port < 1 || port > hub.NumPorts {
		return powercontrol.ErrPortNotFound
	}

	body, err := json.Marshal(powerReq{On: on})
	if err != nil {
		return err
	}

	reqURL := fmt.Sprintf("%s/v1/hub/%s/port/%d/power",
		r.baseURL, url.PathEscape(hub.Location), port)
	resp, err := r.httpClient.Post(reqURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("set port power %s/%d: %w", hub.Location, port, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("set port power %s/%d: status %d: %s",
			hub.Location, port, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
