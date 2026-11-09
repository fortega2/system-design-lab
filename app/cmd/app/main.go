// Command app is the reusable service used across every phase of the lab.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/fortega2/system-design-lab/app/internal/config"
	"github.com/fortega2/system-design-lab/app/internal/server"
	"github.com/fortega2/system-design-lab/app/internal/store"
)

const (
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("app stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})).
		With("instance", cfg.InstanceName)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	deps := server.Deps{Instance: cfg.InstanceName, Logger: logger, Registry: reg}
	if cfg.DatabaseURL != "" {
		st, err := store.New(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBTimeout)
		if err != nil {
			return fmt.Errorf("creating store: %w", err)
		}
		defer st.Close() // runs after the HTTP server has shut down
		reg.MustRegister(st.Collector())
		deps.Items = st
		deps.Checks = map[string]server.ReadinessCheck{"postgres": st.Ping}
		logger.Info("postgres store enabled", "max_conns", cfg.DBMaxConns, "timeout", cfg.DBTimeout)
	}

	srv := server.New(deps)
	httpSrv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", httpSrv.Addr)
		serveErr <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serving: %w", err)
	case <-ctx.Done():
	}

	// Fail readiness first and keep serving for a while, so a load balancer
	// notices and stops routing here before connections are closed.
	logger.Info("shutdown started", "delay", cfg.ShutdownDelay)
	srv.StartDraining()
	time.Sleep(cfg.ShutdownDelay)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving: %w", err)
	}

	logger.Info("shutdown complete")
	return nil
}
