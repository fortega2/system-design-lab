// Package server wires the HTTP handlers of the lab app.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// ReadinessCheck reports whether a dependency is usable. A non-nil error
// makes /readyz fail, which tells a load balancer to stop sending traffic.
type ReadinessCheck func(ctx context.Context) error

// Server holds the handlers and the state they share.
type Server struct {
	instance string
	logger   *slog.Logger
	checks   map[string]ReadinessCheck
	draining atomic.Bool
}

// New builds a Server. checks may be nil while the app has no dependencies.
func New(instance string, logger *slog.Logger, checks map[string]ReadinessCheck) *Server {
	if checks == nil {
		checks = map[string]ReadinessCheck{}
	}
	return &Server{
		instance: instance,
		logger:   logger,
		checks:   checks,
	}
}

// Handler returns the root handler with all routes and middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /slow", s.handleSlow)
	return s.withInstanceHeader(mux)
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

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	failed := map[string]string{}
	for name, check := range s.checks {
		if err := check(ctx); err != nil {
			failed[name] = err.Error()
		}
	}

	if len(failed) > 0 {
		s.logger.WarnContext(ctx, "readiness check failed", "failed", failed)
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
