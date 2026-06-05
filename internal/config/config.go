package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr            string
	UpstreamProxyBase     string
	IndexBase             string
	DefaultMinAge         time.Duration
	DBPath                string
	PollInterval          time.Duration
	BootstrapSince        string
	HTTPTimeout           time.Duration
	AllowRedirectsForInfo bool
	LogLevel              slog.Level
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		ListenAddr:        envString("LISTEN_ADDR", ":8080"),
		UpstreamProxyBase: envString("UPSTREAM_PROXY_BASE", "https://proxy.golang.org"),
		IndexBase:         envString("INDEX_BASE", "https://index.golang.org"),
		DefaultMinAge:     336 * time.Hour,
		DBPath:            envString("DB_PATH", "modguard.db"),
		PollInterval:      envDuration("POLL_INTERVAL", 60*time.Second),
		BootstrapSince:    os.Getenv("BOOTSTRAP_SINCE"),
		HTTPTimeout:       envDuration("HTTP_TIMEOUT", 15*time.Second),
	}

	if raw := os.Getenv("DEFAULT_MIN_AGE"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse DEFAULT_MIN_AGE: %w", err)
		}
		cfg.DefaultMinAge = parsed
	}
	if raw := os.Getenv("ALLOW_REDIRECTS_FOR_INFO"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse ALLOW_REDIRECTS_FOR_INFO: %w", err)
		}
		cfg.AllowRedirectsForInfo = parsed
	}
	if raw := os.Getenv("USE_CACHED_ONLY_UPSTREAM"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse USE_CACHED_ONLY_UPSTREAM: %w", err)
		}
		if parsed {
			cfg.UpstreamProxyBase = strings.TrimRight(cfg.UpstreamProxyBase, "/") + "/cached-only"
		}
	}
	level, err := parseLogLevel(envString("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}
	cfg.LogLevel = level
	return cfg, nil
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown LOG_LEVEL %q", raw)
	}
}
