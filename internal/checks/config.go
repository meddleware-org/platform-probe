// config.go loads the mounted checks file: the one place probe targets come from, so the image and repository hold no real endpoints.

package checks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
)

// fileConfig is the on-disk (mounted) check configuration. It is loaded at
// runtime — never compiled in — so the published image and repository contain no
// real probe targets. The structure preserves group and component order.
type fileConfig struct {
	Groups []fileGroup `json:"groups"`
}

// fileGroup is one capability group in the checks file: a name and its components, in display order.
type fileGroup struct {
	Name       string          `json:"name"`
	Components []fileComponent `json:"components"`
}

// fileComponent is one probe target in the checks file; the field tags are the file's JSON keys.
type fileComponent struct {
	Name               string `json:"name"`
	URL                string `json:"url"`
	ExpectStatus       int    `json:"expect_status"`
	ExpectBodyContains string `json:"expect_body_contains"`
}

// LoadFile reads and validates the checks configuration at path and flattens it
// into an ordered []Check (group order, then component order within each group).
//
// It rejects an empty or malformed file, any component missing a name or URL (or whose URL is not an http or https
// address with a host), and
// an expect_status outside 100..599 (0 or omitted means any 2xx response), so a
// bad edit surfaces as an error the caller can log and ignore (keeping the
// last-good config) rather than silently probing nothing.
func LoadFile(path string) ([]Check, error) {
	// The path (not a secret) is quoted in errors so an operator sees which mounted file is wrong.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read checks config %q: %w", path, err)
	}

	var cfg fileConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	// A misspelt key (expect_staus) is an error, not a silently ignored check setting.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse checks config %q: %w", path, err)
	}

	if len(cfg.Groups) == 0 {
		return nil, fmt.Errorf("checks config %q defines no groups", path)
	}

	// Flatten groups and components into one ordered list, validating each as it is read.
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
			// A URL that cannot be probed is refused here, where the operator is editing, instead of reading "down" forever. The
			// message names the component, never the URL: a configured URL can carry a user name, password or key (CONF lens).
			if u, err := url.Parse(c.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return nil, fmt.Errorf("checks config: component %q in group %q has a url that is not a valid http or https address", c.Name, g.Name)
			}
			if c.ExpectStatus != 0 && (c.ExpectStatus < 100 || c.ExpectStatus > 599) {
				return nil, fmt.Errorf("checks config: component %q in group %q has expect_status %d outside 100..599", c.Name, g.Name, c.ExpectStatus)
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
