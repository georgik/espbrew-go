package http

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

// handleMetrics serves Prometheus metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	log.Debug().Str("remote_addr", r.RemoteAddr).Msg("Metrics requested")
	promhttp.Handler().ServeHTTP(w, r)
}
