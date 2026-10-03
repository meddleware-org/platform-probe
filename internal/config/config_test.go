package config

import (
	"strings"
	"testing"
	"time"
)

// TestLoadDefaults verifies the documented defaults apply with an empty environment.
func TestLoadDefaults(t *testing.T) {
	for _, k := range []string{"PORT", "PROBE_INTERVAL", "PROBE_TIMEOUT", "CHECKS_CONFIG_FILE", "LOG_LEVEL"} {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" || cfg.ProbeInterval != 15*time.Second || cfg.ProbeTimeout != 5*time.Second ||
		cfg.ChecksConfigFile != "/etc/platform-probe/checks.json" || cfg.LogLevel != "info" {
		t.Errorf("defaults: got %+v", cfg)
	}
}

// TestLoadRejectsBadDurations verifies that unparsable, non-positive and overrunning
// durations are refused, so a probe cycle can never outlast its interval.
func TestLoadRejectsBadDurations(t *testing.T) {
	cases := []struct{ interval, timeout, want string }{
		{"soon", "1s", "PROBE_INTERVAL"},
		{"-1s", "1s", "PROBE_INTERVAL must be positive"},
		{"10s", "x", "PROBE_TIMEOUT"},
		{"10s", "-1s", "PROBE_TIMEOUT must be positive"},
		{"10s", "10s", "shorter than PROBE_INTERVAL"},
	}
	for _, c := range cases {
		t.Setenv("PROBE_INTERVAL", c.interval)
		t.Setenv("PROBE_TIMEOUT", c.timeout)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("interval=%s timeout=%s: got %v, want error containing %q", c.interval, c.timeout, err, c.want)
		}
	}
}
