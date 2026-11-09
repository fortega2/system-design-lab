package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func newTestServer(checks map[string]ReadinessCheck) *Server {
	return New("test-1", slog.New(slog.DiscardHandler), checks, prometheus.NewRegistry())
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := do(t, newTestServer(nil).Handler(), http.MethodGet, "/healthz")

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("X-Instance"); got != "test-1" {
		t.Errorf("X-Instance = %q, want %q", got, "test-1")
	}
}

func TestReadyz(t *testing.T) {
	ok := func(context.Context) error { return nil }
	fail := func(context.Context) error { return errors.New("down") }

	tests := []struct {
		name     string
		checks   map[string]ReadinessCheck
		draining bool
		want     int
	}{
		{name: "no checks", want: http.StatusOK},
		{name: "all checks pass", checks: map[string]ReadinessCheck{"db": ok}, want: http.StatusOK},
		{name: "a check fails", checks: map[string]ReadinessCheck{"db": ok, "cache": fail}, want: http.StatusServiceUnavailable},
		{name: "draining", draining: true, want: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(tt.checks)
			if tt.draining {
				s.StartDraining()
			}

			rec := do(t, s.Handler(), http.MethodGet, "/readyz")
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}
