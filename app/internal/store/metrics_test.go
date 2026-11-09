package store

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCollector_ExportsPoolMetrics(t *testing.T) {
	// The pool is lazy: no Postgres is needed to read its stats.
	s, err := New(t.Context(), "postgres://lab:lab@127.0.0.1:1/lab", 7, 0)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(s.Close)

	c := s.Collector()

	if got := testutil.CollectAndCount(c); got != 9 {
		t.Errorf("collected %d metrics, want 9", got)
	}
	if err := testutil.CollectAndCompare(c, strings.NewReader(`
# HELP db_pool_max_conns Maximum size of the connection pool.
# TYPE db_pool_max_conns gauge
db_pool_max_conns 7
`), "db_pool_max_conns"); err != nil {
		t.Errorf("db_pool_max_conns: %v", err)
	}
}
