package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("INSTANCE_NAME", "test-1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.InstanceName != "test-1" {
		t.Errorf("InstanceName = %q, want %q", cfg.InstanceName, "test-1")
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want INFO", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 10s", cfg.ShutdownTimeout)
	}
	if cfg.DatabaseURL != "" || cfg.DBMaxConns != 10 || cfg.DBTimeout != 0 {
		t.Errorf("db config = (%q, %d, %v), want (\"\", 10, 0)", cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("PORT", "9000")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("SHUTDOWN_DELAY", "5s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("Port = %d, want 9000", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want DEBUG", cfg.LogLevel)
	}
	if cfg.ShutdownDelay != 5*time.Second {
		t.Errorf("ShutdownDelay = %v, want 5s", cfg.ShutdownDelay)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{name: "non numeric port", key: "PORT", value: "abc"},
		{name: "port out of range", key: "PORT", value: "70000"},
		{name: "bad duration", key: "SHUTDOWN_DELAY", value: "5 seconds"},
		{name: "bad level", key: "LOG_LEVEL", value: "loud"},
		{name: "zero db conns", key: "DB_MAX_CONNS", value: "0"},
		{name: "too many db conns", key: "DB_MAX_CONNS", value: "5000"},
		{name: "bad db timeout", key: "DB_TIMEOUT", value: "fast"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			if _, err := Load(); err == nil {
				t.Errorf("Load() with %s=%q: want error, got nil", tt.key, tt.value)
			}
		})
	}
}
