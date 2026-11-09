package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbe(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "healthy", status: http.StatusOK},
		{name: "unhealthy", status: http.StatusServiceUnavailable, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(srv.Close)

			err := probe(t.Context(), srv.URL+"/healthz")
			if (err != nil) != tt.wantErr {
				t.Errorf("probe() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProbe_Unreachable(t *testing.T) {
	if err := probe(t.Context(), "http://127.0.0.1:1/healthz"); err == nil {
		t.Error("probe() on a closed port: want error, got nil")
	}
}
