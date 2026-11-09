package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_CountsByRouteAndCode(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		labels []string // method, route, code
	}{
		{name: "matched route ok", path: "/healthz", labels: []string{"GET", "/healthz", "200"}},
		{name: "matched route error", path: "/slow?fail=1", labels: []string{"GET", "/slow", "500"}},
		{name: "unknown route", path: "/nope", labels: []string{"other", "unmatched", "404"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newTestServer(nil)
			do(t, s.Handler(), http.MethodGet, tt.path)

			if got := testutil.ToFloat64(s.metrics.requests.WithLabelValues(tt.labels...)); got != 1 {
				t.Errorf("http_requests_total%v = %v, want 1", tt.labels, got)
			}
			if got := testutil.CollectAndCount(s.metrics.duration); got != 1 {
				t.Errorf("http_request_duration_seconds series = %d, want 1", got)
			}
		})
	}
}

func TestMetrics_ClientClosedIs499(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()

		s := newTestServer(nil)
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/slow?ms=60000", nil)
		s.Handler().ServeHTTP(httptest.NewRecorder(), req)

		if got := testutil.ToFloat64(s.metrics.requests.WithLabelValues("GET", "/slow", "499")); got != 1 {
			t.Errorf("http_requests_total{GET,/slow,499} = %v, want 1", got)
		}
	})
}

func TestMetrics_Endpoint(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	h := New(Deps{Instance: "test-1", Logger: newTestServer(nil).logger, Registry: reg}).Handler()

	do(t, h, http.MethodGet, "/healthz")
	rec := do(t, h, http.MethodGet, "/metrics")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"http_requests_total", "http_request_duration_seconds", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics output missing %q", want)
		}
	}
}
