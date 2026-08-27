// Package config loads and validates the platform-probe's runtime configuration
// from environment variables. All values are optional and fall back to sensible
// defaults, so the service starts with no configuration in a standard in-cluster
// deployment.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds validated runtime configuration.
type Config struct {
	Port             string
	ProbeInterval    time.Duration
	ProbeTimeout     time.Duration
	ChecksConfigFile string
	LogLevel         string
}

// Load reads configuration from the environment and returns a validated Config.
//
// Optional variables and their defaults: PORT=8080, PROBE_INTERVAL=15s,
// PROBE_TIMEOUT=5s, CHECKS_CONFIG_FILE=/etc/platform-probe/checks.json,
// LOG_LEVEL=info. PROBE_INTERVAL and PROBE_TIMEOUT must parse as time.Duration
// values, and the timeout must be shorter than the interval so a slow cycle
// cannot overrun the next one. The checks file itself is loaded (and reloaded)
// by the prober, not here, so a missing file does not prevent startup.
func Load() (*Config, error) {
	envOr := func(name, def string) string {
		if v := os.Getenv(name); v != "" {
			return v
		}
		return def
	}

	cfg := &Config{
		Port:             envOr("PORT", "8080"),
		ChecksConfigFile: envOr("CHECKS_CONFIG_FILE", "/etc/platform-probe/checks.json"),
		LogLevel:         envOr("LOG_LEVEL", "info"),
	}

	intervalStr := envOr("PROBE_INTERVAL", "15s")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("parse PROBE_INTERVAL %q: %w", intervalStr, err)
	}
	if interval <= 0 {
		return nil, fmt.Errorf("PROBE_INTERVAL must be positive, got %s", interval)
	}
	cfg.ProbeInterval = interval

	timeoutStr := envOr("PROBE_TIMEOUT", "5s")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("parse PROBE_TIMEOUT %q: %w", timeoutStr, err)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("PROBE_TIMEOUT must be positive, got %s", timeout)
	}
	if timeout >= interval {
		return nil, fmt.Errorf("PROBE_TIMEOUT (%s) must be shorter than PROBE_INTERVAL (%s)", timeout, interval)
	}
	cfg.ProbeTimeout = timeout

	return cfg, nil
}
