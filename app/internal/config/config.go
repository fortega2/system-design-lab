// Package config loads the application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

const (
	defaultPort            = 8080
	defaultShutdownTimeout = 10 * time.Second
)

// Config holds every knob the lab app exposes. Each experiment changes
// behavior by setting environment variables, never by changing code.
type Config struct {
	Port         int
	InstanceName string
	LogLevel     slog.Level

	// ShutdownDelay is how long the app keeps serving (with /readyz failing)
	// after receiving SIGTERM, so a load balancer can stop routing to it.
	ShutdownDelay time.Duration
	// ShutdownTimeout bounds how long in-flight requests get to finish.
	ShutdownTimeout time.Duration
}

// Load reads the configuration from the environment, applying defaults.
func Load() (Config, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	var errs []error
	cfg := Config{
		Port:            intEnv("PORT", defaultPort, &errs),
		InstanceName:    stringEnv("INSTANCE_NAME", hostname),
		LogLevel:        levelEnv("LOG_LEVEL", slog.LevelInfo, &errs),
		ShutdownDelay:   durationEnv("SHUTDOWN_DELAY", 0, &errs),
		ShutdownTimeout: durationEnv("SHUTDOWN_TIMEOUT", defaultShutdownTimeout, &errs),
	}

	if cfg.Port <= 0 || cfg.Port > 65535 {
		errs = append(errs, fmt.Errorf("PORT out of range: %d", cfg.Port))
	}

	return cfg, errors.Join(errs...)
}

func stringEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func intEnv(key string, def int, errs *[]error) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: invalid integer %q", key, v))
		return def
	}
	return n
}

func durationEnv(key string, def time.Duration, errs *[]error) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: invalid duration %q", key, v))
		return def
	}
	return d
}

func levelEnv(key string, def slog.Level, errs *[]error) slog.Level {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(v)); err != nil {
		*errs = append(*errs, fmt.Errorf("%s: invalid level %q", key, v))
		return def
	}
	return lvl
}
