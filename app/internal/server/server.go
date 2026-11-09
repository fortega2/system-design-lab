// Package server wires the HTTP handlers of the lab app.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// readinessTimeout bounds how long /readyz waits on all dependency checks.
const readinessTimeout = 2 * time.Second

// ReadinessCheck reports whether a dependency is usable. A non-nil error
// makes /readyz fail, which tells a load balancer to stop sending traffic.
type ReadinessCheck func(ctx context.Context) error

// Deps are the dependencies of a Server.
type Deps struct {
	Instance string
	Logger   *slog.Logger
	// Registry receives the HTTP metrics and is exposed on /metrics.
	Registry *prometheus.Registry
	// Checks are evaluated by /readyz. Optional.
	Checks map[string]ReadinessCheck
	// Items backs the /items routes. Optional: nil leaves them unregistered.
	Items ItemStore
}

// Server holds the handlers and the state they share.
type Server struct {
	instance string
	logger   *slog.Logger
	checks   map[string]ReadinessCheck
	items    ItemStore
	registry *prometheus.Registry
	metrics  *httpMetrics
	draining atomic.Bool
}

// New builds a Server from its dependencies.
func New(d Deps) *Server {
	checks := d.Checks
	if checks == nil {
		checks = map[string]ReadinessCheck{}
	}
	return &Server{
		instance: d.Instance,
		logger:   d.Logger,
		checks:   checks,
		items:    d.Items,
		registry: d.Registry,
		metrics:  newHTTPMetrics(d.Registry),
	}
}

// Handler returns the root handler with all routes and middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /slow", s.handleSlow)
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{Registry: s.registry}))
	if s.items != nil {
		mux.HandleFunc("GET /items/{id}", s.handleGetItem)
		mux.HandleFunc("POST /items", s.handleCreateItem)
	}
	return s.withInstanceHeader(s.metrics.middleware(mux))
}

// StartDraining makes /readyz fail from now on while the server keeps
// answering every other request. Called when a shutdown begins.
func (s *Server) StartDraining() {
	s.draining.Store(true)
}

// handleHealthz is liveness: the process is up and able to answer.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz is readiness: the instance can do useful work right now.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "draining"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	// Error details stay in the logs: they reveal infrastructure (hosts,
	// users, database names) that anyone reaching /readyz would see.
	failed := map[string]string{}
	for name, check := range s.checks {
		if err := check(ctx); err != nil {
			s.logger.WarnContext(ctx, "readiness check failed", "check", name, "error", err)
			failed[name] = "unavailable"
		}
	}

	if len(failed) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not ready", "failed": failed})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// withInstanceHeader tags every response with the instance that served it,
// so it is visible which replica behind a load balancer answered.
func (s *Server) withInstanceHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Instance", s.instance)
		next.ServeHTTP(w, r)
	})
}
