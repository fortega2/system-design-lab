package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

func TestSlow_InvalidParams(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "negative ms", query: "ms=-1"},
		{name: "non numeric ms", query: "ms=abc"},
		{name: "ms above cap", query: "ms=60001"},
		{name: "negative jitter", query: "jitter=-5"},
		{name: "ms plus jitter above cap", query: "ms=40000&jitter=30000"},
		{name: "fail above one", query: "fail=1.5"},
		{name: "non numeric fail", query: "fail=x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, newTestServer(nil).Handler(), http.MethodGet, "/slow?"+tt.query)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
			}
		})
	}
}

func TestSlow_FailRate(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "never fails", query: "fail=0", want: http.StatusOK},
		{name: "always fails", query: "fail=1", want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := do(t, newTestServer(nil).Handler(), http.MethodGet, "/slow?"+tt.query)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestSlow_WaitsRequestedDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		rec := do(t, newTestServer(nil).Handler(), http.MethodGet, "/slow?ms=200")

		if elapsed := time.Since(start); elapsed != 200*time.Millisecond {
			t.Errorf("elapsed = %v, want 200ms", elapsed)
		}
		var body struct {
			SleptMS int64 `json:"slept_ms"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		if body.SleptMS != 200 {
			t.Errorf("slept_ms = %d, want 200", body.SleptMS)
		}
	})
}

func TestSlow_JitterStaysInRange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTestServer(nil).Handler()
		for range 50 {
			start := time.Now()
			do(t, h, http.MethodGet, "/slow?ms=100&jitter=50")

			if elapsed := time.Since(start); elapsed < 100*time.Millisecond || elapsed > 150*time.Millisecond {
				t.Fatalf("elapsed = %v, want within [100ms, 150ms]", elapsed)
			}
		}
	})
}

func TestSlow_ReturnsWhenClientGivesUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/slow?ms=60000", nil)
		rec := httptest.NewRecorder()

		start := time.Now()
		newTestServer(nil).Handler().ServeHTTP(rec, req)

		if elapsed := time.Since(start); elapsed != 50*time.Millisecond {
			t.Errorf("elapsed = %v, want 50ms (handler must stop when the client gives up)", elapsed)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want empty for an abandoned request", rec.Body)
		}
	})
}
