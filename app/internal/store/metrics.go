package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

var _ prometheus.Collector = (*PoolCollector)(nil)

// poolMetric describes one metric read from pgxpool.Stat on every scrape.
type poolMetric struct {
	desc      *prometheus.Desc
	valueType prometheus.ValueType
	value     func(*pgxpool.Stat) float64
}

// PoolCollector exports the connection pool state. It holds no state of its
// own: each scrape reads a fresh pool.Stat().
type PoolCollector struct {
	pool    *pgxpool.Pool
	metrics []poolMetric
}

// Collector returns a Prometheus collector for the store's connection pool.
//
// Pool saturation (Phase 2): db_pool_acquired_conns == db_pool_max_conns and
//
//	rate(db_pool_empty_acquires_total[1m]) > 0
//
// means requests are waiting for a free connection; the time they wait is
//
//	rate(db_pool_empty_acquire_wait_seconds_total[1m])
func (s *Store) Collector() *PoolCollector {
	gauge := func(name, help string, v func(*pgxpool.Stat) float64) poolMetric {
		return poolMetric{prometheus.NewDesc(name, help, nil, nil), prometheus.GaugeValue, v}
	}
	counter := func(name, help string, v func(*pgxpool.Stat) float64) poolMetric {
		return poolMetric{prometheus.NewDesc(name, help, nil, nil), prometheus.CounterValue, v}
	}

	return &PoolCollector{
		pool: s.pool,
		metrics: []poolMetric{
			gauge("db_pool_max_conns", "Maximum size of the connection pool.",
				func(st *pgxpool.Stat) float64 { return float64(st.MaxConns()) }),
			gauge("db_pool_total_conns", "Connections currently open (idle + acquired + constructing).",
				func(st *pgxpool.Stat) float64 { return float64(st.TotalConns()) }),
			gauge("db_pool_acquired_conns", "Connections currently in use by a query.",
				func(st *pgxpool.Stat) float64 { return float64(st.AcquiredConns()) }),
			gauge("db_pool_idle_conns", "Connections currently idle in the pool.",
				func(st *pgxpool.Stat) float64 { return float64(st.IdleConns()) }),
			counter("db_pool_acquires_total", "Successful connection acquisitions.",
				func(st *pgxpool.Stat) float64 { return float64(st.AcquireCount()) }),
			counter("db_pool_empty_acquires_total", "Acquisitions that had to wait because no connection was free.",
				func(st *pgxpool.Stat) float64 { return float64(st.EmptyAcquireCount()) }),
			counter("db_pool_empty_acquire_wait_seconds_total", "Total time spent waiting for a free connection.",
				func(st *pgxpool.Stat) float64 { return st.EmptyAcquireWaitTime().Seconds() }),
			counter("db_pool_canceled_acquires_total", "Acquisitions canceled by the caller while waiting.",
				func(st *pgxpool.Stat) float64 { return float64(st.CanceledAcquireCount()) }),
			counter("db_pool_new_conns_total", "Connections opened against Postgres.",
				func(st *pgxpool.Stat) float64 { return float64(st.NewConnsCount()) }),
		},
	}
}

// Describe implements prometheus.Collector.
func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m.desc
	}
}

// Collect implements prometheus.Collector.
func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	st := c.pool.Stat()
	for _, m := range c.metrics {
		ch <- prometheus.MustNewConstMetric(m.desc, m.valueType, m.value(st))
	}
}
