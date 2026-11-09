//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestStore connects to DATABASE_URL and makes sure the schema exists.
func newTestStore(t *testing.T, timeout time.Duration) *Store {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}

	applySchema(t, url)

	s, err := New(t.Context(), url, 4, timeout)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func applySchema(t *testing.T, url string) {
	t.Helper()

	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer pool.Close()

	var exists bool
	if err := pool.QueryRow(t.Context(), `SELECT to_regclass('items') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("checking schema: %v", err)
	}
	if exists {
		return
	}

	schema, err := os.ReadFile("../../schema/001_items.sql")
	if err != nil {
		t.Fatalf("reading schema: %v", err)
	}
	if _, err := pool.Exec(t.Context(), string(schema)); err != nil {
		t.Fatalf("applying schema: %v", err)
	}
}

func TestStore_CreateAndGet(t *testing.T) {
	s := newTestStore(t, 0)

	created, err := s.Create(t.Context(), "widget", "blue")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID <= 0 || created.CreatedAt.IsZero() {
		t.Fatalf("Create() = %+v, want generated id and created_at", created)
	}

	got, err := s.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != "widget" || got.Value != "blue" {
		t.Errorf("Get() = %+v, want name=widget value=blue", got)
	}
}

func TestStore_GetNotFound(t *testing.T) {
	s := newTestStore(t, 0)

	if _, err := s.Get(t.Context(), 1<<62); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestStore_TimeoutIsDeadlineExceeded(t *testing.T) {
	s := newTestStore(t, time.Nanosecond)

	if _, err := s.Get(t.Context(), 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Get() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestStore_Ping(t *testing.T) {
	s := newTestStore(t, 0)

	if err := s.Ping(t.Context()); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}
