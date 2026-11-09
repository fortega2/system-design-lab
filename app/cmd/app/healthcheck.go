package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// healthcheckTimeout bounds the whole probe, so a frozen app counts as unhealthy.
const healthcheckTimeout = 2 * time.Second

// runHealthcheck probes the local /healthz and returns the process exit code.
// The runtime image has no curl or wget, so the binary checks itself:
//
//	HEALTHCHECK CMD ["/app", "-healthcheck"]
func runHealthcheck(port int) int {
	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	if err := probe(ctx, "http://127.0.0.1:"+strconv.Itoa(port)+"/healthz"); err != nil {
		fmt.Println("unhealthy:", err)
		return 1
	}
	return 0
}

func probe(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("requesting %s: %w", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection closes cleanly

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
