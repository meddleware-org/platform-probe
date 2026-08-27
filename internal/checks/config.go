package checks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// fileConfig is the on-disk (mounted) check configuration. It is loaded at
// runtime — never compiled in — so the published image and repository contain no
// real probe targets. The structure preserves group and component order.
type fileConfig struct {
	Groups []fileGroup `json:"groups"`
}

type fileGroup struct {
	Name       string          `json:"name"`
	Components []fileComponent `json:"components"`
}

type fileComponent struct {
	Name               string `json:"name"`
	URL                string `json:"url"`
	ExpectStatus       int    `json:"expect_status"`
	ExpectBodyContains string `json:"expect_body_contains"`
}

// LoadFile reads and validates the checks configuration at path and flattens it
// into an ordered []Check (group order, then component order within each group).
//
// It rejects an empty or malformed file and any component missing a name or URL,
// so a bad edit surfaces as an error the caller can log and ignore (keeping the
// last-good config) rather than silently probing nothing.
func LoadFile(path string) ([]Check, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checks config %q: %w", path, err)
	}

	var cfg fileConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse checks config %q: %w", path, err)
	}

	if len(cfg.Groups) == 0 {
		return nil, fmt.Errorf("checks config %q defines no groups", path)
	}

	var checks []Check
	for gi, g := range cfg.Groups {
		if g.Name == "" {
			return nil, fmt.Errorf("checks config: group %d has no name", gi)
		}
		if len(g.Components) == 0 {
			return nil, fmt.Errorf("checks config: group %q has no components", g.Name)
		}
		for _, c := range g.Components {
			if c.Name == "" {
				return nil, fmt.Errorf("checks config: a component in group %q has no name", g.Name)
			}
			if c.URL == "" {
				return nil, fmt.Errorf("checks config: component %q in group %q has no url", c.Name, g.Name)
			}
			checks = append(checks, Check{
				Group:              g.Name,
				Name:               c.Name,
				URL:                c.URL,
				ExpectStatus:       c.ExpectStatus,
				ExpectBodyContains: c.ExpectBodyContains,
			})
		}
	}
	return checks, nil
}
