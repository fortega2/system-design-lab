package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// statusClientClosed mirrors nginx's 499: the client went away before a
// response was written (e.g. it timed out waiting on /slow).
const statusClientClosed = 499

// httpMetrics are the RED metrics (rate, errors, duration) of every route.
type httpMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

func newHTTPMetrics(reg prometheus.Registerer) *httpMetrics {
	f := promauto.With(reg)
	labels := []string{"method", "route", "code"}
	return &httpMetrics{
		// rate:   sum by (route) (rate(http_requests_total[1m]))
		// errors: sum by (route) (rate(http_requests_total{code=~"5.."}[1m]))
		requests: f.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests handled, by method, route and status code.",
		}, labels),
		// p99: histogram_quantile(0.99, sum by (le, route) (rate(http_request_duration_seconds_bucket[1m])))
		duration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency, by method, route and status code.",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, labels),
		// saturation: http_requests_in_flight
		inFlight: f.NewGauge(prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "HTTP requests currently being served.",
		}),
	}
}

// middleware must wrap the ServeMux (not sit inside it) so r.Pattern is
// already set by the mux when the request is recorded.
func (m *httpMetrics) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		method, route := routeLabels(r.Pattern)
		labels := []string{method, route, strconv.Itoa(rec.code(r))}
		m.requests.WithLabelValues(labels...).Inc()
		m.duration.WithLabelValues(labels...).Observe(time.Since(start).Seconds())
	})
}

// routeLabels derives bounded label values from the matched pattern
// ("GET /items/{id}"), never from the raw path, to keep cardinality low.
func routeLabels(pattern string) (method, route string) {
	method, route, found := strings.Cut(pattern, " ")
	if !found || route == "" {
		return "other", "unmatched"
	}
	return method, route
}

// statusRecorder captures the status code written by the handler.
type statusRecorder struct {
	http.ResponseWriter

	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *statusRecorder) code(req *http.Request) int {
	switch {
	case r.status != 0:
		return r.status
	case req.Context().Err() != nil:
		return statusClientClosed
	default:
		return http.StatusOK
	}
}
