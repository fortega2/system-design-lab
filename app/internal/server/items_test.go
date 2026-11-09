package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/fortega2/system-design-lab/app/internal/store"
)

// fakeStore returns the configured item/error and records the last call.
type fakeStore struct {
	item store.Item
	err  error

	gotID    int64
	gotName  string
	gotValue string
}

func (f *fakeStore) Get(_ context.Context, id int64) (store.Item, error) {
	f.gotID = id
	return f.item, f.err
}

func (f *fakeStore) Create(_ context.Context, name, value string) (store.Item, error) {
	f.gotName, f.gotValue = name, value
	return f.item, f.err
}

func newItemsServer(items ItemStore) http.Handler {
	return New(Deps{
		Instance: "test-1",
		Logger:   slog.New(slog.DiscardHandler),
		Registry: prometheus.NewRegistry(),
		Items:    items,
	}).Handler()
}

func doBody(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body)))
	return rec
}

func TestGetItem(t *testing.T) {
	item := store.Item{ID: 7, Name: "widget", Value: "blue", CreatedAt: time.Unix(0, 0).UTC()}

	tests := []struct {
		name string
		path string
		err  error
		want int
	}{
		{name: "found", path: "/items/7", want: http.StatusOK},
		{name: "not found", path: "/items/7", err: store.ErrNotFound, want: http.StatusNotFound},
		{name: "database timeout", path: "/items/7", err: context.DeadlineExceeded, want: http.StatusGatewayTimeout},
		{name: "database error", path: "/items/7", err: errors.New("boom"), want: http.StatusInternalServerError},
		{name: "non numeric id", path: "/items/abc", want: http.StatusBadRequest},
		{name: "zero id", path: "/items/0", want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := &fakeStore{item: item, err: tt.err}
			rec := do(t, newItemsServer(fs), http.MethodGet, tt.path)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want != http.StatusOK {
				return
			}
			var got store.Item
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			if got != item || fs.gotID != 7 {
				t.Errorf("got item %+v (store asked for id %d), want %+v", got, fs.gotID, item)
			}
		})
	}
}

func TestCreateItem(t *testing.T) {
	tests := []struct {
		name string
		body string
		err  error
		want int
	}{
		{name: "created", body: `{"name":"widget","value":"blue"}`, want: http.StatusCreated},
		{name: "missing name", body: `{"value":"blue"}`, want: http.StatusBadRequest},
		{name: "blank name", body: `{"name":"   "}`, want: http.StatusBadRequest},
		{name: "invalid json", body: `{"name":`, want: http.StatusBadRequest},
		{name: "unknown field", body: `{"name":"w","color":"red"}`, want: http.StatusBadRequest},
		{name: "database error", body: `{"name":"widget"}`, err: errors.New("boom"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fs := &fakeStore{item: store.Item{ID: 1, Name: "widget", Value: "blue"}, err: tt.err}
			rec := doBody(t, newItemsServer(fs), http.MethodPost, "/items", tt.body)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want == http.StatusCreated && (fs.gotName != "widget" || fs.gotValue != "blue") {
				t.Errorf("store got (%q, %q), want (widget, blue)", fs.gotName, fs.gotValue)
			}
		})
	}
}

func TestItems_NotRegisteredWithoutStore(t *testing.T) {
	rec := do(t, newItemsServer(nil), http.MethodGet, "/items/1")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no store is configured", rec.Code)
	}
}
