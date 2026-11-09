package server

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// maxSlowMS caps the total delay of /slow so a typo cannot hang a request forever.
const maxSlowMS = 60_000

// slowParams controls the behavior of /slow.
type slowParams struct {
	base     time.Duration // fixed delay
	jitter   time.Duration // extra random delay in [0, jitter]
	failRate float64       // probability in [0, 1] of answering 500
}

// handleSlow simulates a dependency with configurable latency and error rate:
//
//	GET /slow?ms=200&jitter=50&fail=0.05
//
// The wait honors the request context: if the client gives up (timeout,
// closed connection), the handler returns immediately instead of holding
// the goroutine until the delay ends.
func (s *Server) handleSlow(w http.ResponseWriter, r *http.Request) {
	p, err := parseSlowParams(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	delay := p.base
	if p.jitter > 0 {
		delay += time.Duration(rand.Int64N(int64(p.jitter) + 1))
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-r.Context().Done():
		s.logger.DebugContext(r.Context(), "slow request abandoned by client", "delay", delay)
		return
	case <-timer.C:
	}

	if rand.Float64() < p.failRate {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "injected failure"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"slept_ms": delay.Milliseconds(),
		"instance": s.instance,
	})
}

func parseSlowParams(q url.Values) (slowParams, error) {
	ms, err := intParam(q, "ms", 0, maxSlowMS)
	if err != nil {
		return slowParams{}, err
	}
	jitter, err := intParam(q, "jitter", 0, maxSlowMS)
	if err != nil {
		return slowParams{}, err
	}
	if ms+jitter > maxSlowMS {
		return slowParams{}, fmt.Errorf("ms + jitter must be at most %d", maxSlowMS)
	}
	fail, err := floatParam(q, "fail", 0, 1)
	if err != nil {
		return slowParams{}, err
	}

	return slowParams{
		base:     time.Duration(ms) * time.Millisecond,
		jitter:   time.Duration(jitter) * time.Millisecond,
		failRate: fail,
	}, nil
}

// intParam reads an optional integer query parameter within [lo, hi]; missing means 0.
func intParam(q url.Values, name string, lo, hi int) (int, error) {
	raw := q.Get(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%s must be an integer between %d and %d, got %q", name, lo, hi, raw)
	}
	return n, nil
}

// floatParam reads an optional float query parameter within [lo, hi]; missing means 0.
func floatParam(q url.Values, name string, lo, hi float64) (float64, error) {
	raw := q.Get(name)
	if raw == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < lo || f > hi {
		return 0, fmt.Errorf("%s must be a number between %g and %g, got %q", name, lo, hi, raw)
	}
	return f, nil
}
