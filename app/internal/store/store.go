// Package store is the Postgres access layer of the lab app.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when the requested item does not exist.
var ErrNotFound = errors.New("item not found")

// Item is a row of the items table.
type Item struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

// Store runs queries against Postgres through a connection pool.
type Store struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// New creates the pool. It does not connect yet: connections are opened on
// first use, so the app starts even while Postgres is down (and /readyz
// reports it). timeout bounds every query; 0 disables it on purpose, to be
// able to observe what happens without timeouts.
func New(ctx context.Context, url string, maxConns int32, timeout time.Duration) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	cfg.MaxConns = maxConns

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}
	return &Store{pool: pool, timeout: timeout}, nil
}

// Get returns the item with the given id, or ErrNotFound.
func (s *Store) Get(ctx context.Context, id int64) (Item, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	var it Item
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, value, created_at FROM items WHERE id = $1`, id,
	).Scan(&it.ID, &it.Name, &it.Value, &it.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("getting item %d: %w", id, err)
	}
	return it, nil
}

// Create inserts a new item and returns it as stored.
func (s *Store) Create(ctx context.Context, name, value string) (Item, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	var it Item
	err := s.pool.QueryRow(ctx,
		`INSERT INTO items (name, value) VALUES ($1, $2) RETURNING id, name, value, created_at`, name, value,
	).Scan(&it.ID, &it.Name, &it.Value, &it.CreatedAt)
	if err != nil {
		return Item{}, fmt.Errorf("creating item: %w", err)
	}
	return it, nil
}

// Ping checks that Postgres answers. Used as the readiness check.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging postgres: %w", err)
	}
	return nil
}

// Close closes every connection of the pool.
func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, s.timeout)
}
