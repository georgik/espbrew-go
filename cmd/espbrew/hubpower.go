//go:build linux
// +build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/georgik/espbrew-go/internal/powercontrol"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"syscall"
)

// hubPowerCmd is a tiny long-running service that exposes USB hub port power
// control over HTTP, so an unprivileged espbrew leader can power-cycle boards
// by talking to it instead of running the privileged sysfs write itself.
//
// It runs in a dedicated privileged pod (that pod mounts /sys read-write,
// which is the only thing the kernel allows to flip a hub port's power — see
// deploy/deployment.yaml and wiki/non-privileged-power-control-feasibility-report.md).
// This binary is otherwise just the normal espbrew, so the "one codebase,
// switch config" model holds: the same image runs both the leader and this
// service. The leader talks to it over the cluster network via the
// espbrew-hub-power Service.
var hubPowerCmd = &cobra.Command{
	Use:   "hub-power",
	Short: "Serve USB hub port power control over HTTP",
	Long: `Serve USB hub port power control over HTTP.

This is the minimal, single-purpose service that owns the ONE operation the
kernel will not allow the unprivileged espbrew leader to do itself: physically
toggling a USB hub port's power. It wraps the same powercontrol controller the
leader uses in-process, but exposes it as a small HTTP API so the leader can
stay non-privileged and simply call the pod.

Run it in a privileged pod that mounts /sys read-write.`,
}

var hubPowerServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the hub-power HTTP service and block until signalled",
	RunE:  runHubPowerServe,
}

var hubPowerOpts struct {
	addr string
	port int
}

func init() {
	rootCmd.AddCommand(hubPowerCmd)
	hubPowerCmd.AddCommand(hubPowerServeCmd)

	hubPowerServeCmd.Flags().StringVar(&hubPowerOpts.addr, "addr", "0.0.0.0", "Bind address")
	hubPowerServeCmd.Flags().IntVar(&hubPowerOpts.port, "port", 8080, "HTTP port")
}

// runHubPowerServe starts the HTTP server and blocks until SIGINT/SIGTERM.
func runHubPowerServe(cmd *cobra.Command, args []string) error {
	level := resolveLogLevel()
	zerolog.SetGlobalLevel(level)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	srv := &hubPowerServer{ctrl: powercontrol.NewController()}
	mux := http.NewServeMux()
	srv.mount(mux)

	addr := fmt.Sprintf("%s:%d", hubPowerOpts.addr, hubPowerOpts.port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
		// A power cycle is off + delay + on, so allow generous read time.
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info().Str("addr", addr).Msg("hub-power service listening")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("hub-power server stopped")
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info().Msg("Shutting down hub-power...")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	return nil
}

// hubPowerServer owns the single privileged operation and exposes it as HTTP.
type hubPowerServer struct {
	ctrl powercontrol.PowerController
}

// mount registers the hub-power API. The routes are intentionally tiny:
//
//	GET  /health
//	GET  /v1/hub/{location}            -> hub description (for discovery)
//	POST /v1/hub/{location}/port/{port}/power  {"on": bool}  -> set power
//	POST /v1/hub/{location}/port/{port}/cycle[?delay=..]   -> power cycle
func (h *hubPowerServer) mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/hub/{location}", h.handleHub)
	mux.HandleFunc("/v1/hub/{location}/port/{port}/power", h.handlePower)
	mux.HandleFunc("/v1/hub/{location}/port/{port}/cycle", h.handleCycle)
}

func (h *hubPowerServer) handleHub(w http.ResponseWriter, r *http.Request) {
	hub, err := h.ctrl.FindHubByLocation(r.PathValue("location"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, hubResp{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, hubResp{
		Location:   hub.Location,
		Vendor:     hub.Vendor,
		Product:    hub.Product,
		NumPorts:   hub.NumPorts,
		SuperSpeed: hub.SuperSpeed,
	})
}

func (h *hubPowerServer) handlePower(w http.ResponseWriter, r *http.Request) {
	var req powerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, hubResp{Error: err.Error()})
		return
	}

	port, err := strconv.Atoi(r.PathValue("port"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, hubResp{Error: "invalid port"})
		return
	}

	hub, err := h.ctrl.FindHubByLocation(r.PathValue("location"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, hubResp{Error: err.Error()})
		return
	}

	log.Info().Str("location", hub.Location).Int("port", port).Bool("on", req.On).
		Msg("hub-power: setting port power")
	if err := h.ctrl.SetPortPowerDual(hub, port, req.On); err != nil {
		writeJSON(w, http.StatusInternalServerError, hubResp{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, hubResp{Ok: true})
}

func (h *hubPowerServer) handleCycle(w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(r.PathValue("port"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, hubResp{Error: "invalid port"})
		return
	}

	hub, err := h.ctrl.FindHubByLocation(r.PathValue("location"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, hubResp{Error: err.Error()})
		return
	}

	// Optional delay between the off and on halves (e.g. "600ms").
	delay := 500 * time.Millisecond
	if d := r.URL.Query().Get("delay"); d != "" {
		if parsed, perr := time.ParseDuration(d); perr == nil {
			delay = parsed
		}
	}

	log.Info().Str("location", hub.Location).Int("port", port).
		Dur("delay", delay).Msg("hub-power: power cycling")
	if err := h.ctrl.PowerCycle(hub, port, delay); err != nil {
		writeJSON(w, http.StatusInternalServerError, hubResp{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, hubResp{Ok: true})
}

// hubResp is the JSON envelope for every hub-power response.
type hubResp struct {
	Ok         bool   `json:"ok,omitempty"`
	Location   string `json:"location,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	Product    string `json:"product,omitempty"`
	NumPorts   int    `json:"numPorts,omitempty"`
	SuperSpeed bool   `json:"superSpeed,omitempty"`
	Error      string `json:"error,omitempty"`
}

// powerReq is the JSON body of a power request.
type powerReq struct {
	On bool `json:"on"`
}

func writeJSON(w http.ResponseWriter, status int, v hubResp) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
