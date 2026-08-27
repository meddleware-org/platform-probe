// Package checks defines the service health checks, runs them server-side on a
// timer, and caches an aggregated, topology-free snapshot for the status page to
// read. Clients only ever read the cached snapshot; they never trigger a probe.
//
// The internal probe URLs live here in code and are never included in the
// snapshot, so the public API discloses only capability-level health, not the
// cluster's internal endpoints or tech stack.
package checks

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Status is a component/group/overall health state.
type Status string

const (
	// StatusOperational means every underlying check passed.
	StatusOperational Status = "operational"
	// StatusDegraded means some, but not all, underlying checks failed.
	StatusDegraded Status = "degraded"
	// StatusDown means every underlying check failed.
	StatusDown Status = "down"
	// StatusUnknown is the seed state before the first probe cycle completes.
	StatusUnknown Status = "unknown"
)

// Check is a single server-side health probe. It is internal configuration and
// is never serialised — in particular URL must not reach clients.
type Check struct {
	Group              string // capability group the component belongs to
	Name               string // component name shown on the status page
	URL                string // internal in-cluster URL to probe
	ExpectStatus       int    // required HTTP status code (e.g. 200)
	ExpectBodyContains string // optional substring the response body must contain
}

// Component is the public health of a single component (no URL, no error text).
type Component struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
}

// Group is a capability grouping of components with an aggregated status.
type Group struct {
	Name       string      `json:"name"`
	Status     Status      `json:"status"`
	Components []Component `json:"components"`
}

// Snapshot is the cached, topology-free view returned by GET /api/status.
type Snapshot struct {
	GeneratedAt time.Time `json:"generated_at"`
	Overall     Status    `json:"overall"`
	Groups      []Group   `json:"groups"`
}

// Prober loads its checks from configPath, re-reading the file on every cycle so
// edits to the mounted config take effect without a restart. It aggregates each
// cycle's results into a Snapshot and caches the latest for concurrent readers.
//
// No probe targets are compiled in: the check definitions live only in the
// runtime config file, so the published binary/image discloses nothing about
// what is probed.
type Prober struct {
	configPath string
	timeout    time.Duration
	client     *http.Client
	cache      atomic.Pointer[Snapshot]

	mu     sync.Mutex // guards checks (swapped on reload)
	checks []Check
}

// NewProber returns a Prober that reads its checks from configPath. It attempts
// an initial load so Snapshot has real component names immediately; if the file
// is missing or invalid it logs and seeds an empty "unknown" snapshot, then keeps
// retrying each cycle (a mis-mounted config must not crash the service). Snapshot
// never returns nil.
func NewProber(configPath string, timeout time.Duration) *Prober {
	p := &Prober{
		configPath: configPath,
		timeout:    timeout,
		// No global client timeout; each probe uses a per-request context deadline.
		client: &http.Client{},
	}
	if loaded, err := LoadFile(configPath); err != nil {
		slog.Warn("initial checks config load failed; serving empty status until it is valid", "err", err)
		p.cache.Store(&Snapshot{GeneratedAt: time.Now().UTC(), Overall: StatusUnknown})
	} else {
		p.checks = loaded
		p.cache.Store(seedSnapshot(loaded))
	}
	return p
}

// reload re-reads the config file and swaps in the new checks. On error it keeps
// the last-good set so a bad edit never blanks the page.
func (p *Prober) reload() {
	loaded, err := LoadFile(p.configPath)
	if err != nil {
		slog.Warn("checks config reload failed; keeping last-good config", "err", err)
		return
	}
	p.mu.Lock()
	p.checks = loaded
	p.mu.Unlock()
}

// currentChecks returns a snapshot of the active check set for this cycle.
func (p *Prober) currentChecks() []Check {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.checks
}

// Snapshot returns the most recently cached snapshot. It is safe for concurrent
// use and never returns nil.
func (p *Prober) Snapshot() *Snapshot {
	return p.cache.Load()
}

// Start runs an initial cycle immediately, then re-runs every interval until ctx
// is cancelled. It blocks, so callers typically run it in a goroutine.
func (p *Prober) Start(ctx context.Context, interval time.Duration) {
	p.RunOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.RunOnce(ctx)
		}
	}
}

// RunOnce reloads the config, probes every check concurrently, builds a fresh
// Snapshot, and stores it in the cache. Probe failures are recorded as "down" —
// a cycle never fails. If the config currently has no checks, it stores an empty
// "unknown" snapshot.
func (p *Prober) RunOnce(ctx context.Context) {
	p.reload()
	checks := p.currentChecks()
	if len(checks) == 0 {
		p.cache.Store(&Snapshot{GeneratedAt: time.Now().UTC(), Overall: StatusUnknown})
		return
	}

	statuses := make([]Status, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		go func(i int, c Check) {
			defer wg.Done()
			statuses[i] = p.probe(ctx, c)
		}(i, c)
	}
	wg.Wait()

	snap := aggregate(checks, statuses)
	p.cache.Store(snap)
	slog.Debug("probe cycle complete", "overall", snap.Overall)
}

// probe performs one HTTP GET and returns StatusOperational only if the response
// matches the check's expected status code (and optional body substring). Any
// error, timeout, or mismatch yields StatusDown. No error detail is retained.
func (p *Prober) probe(ctx context.Context, c Check) Status {
	rctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(rctx, http.MethodGet, c.URL, nil)
	if err != nil {
		slog.Debug("probe request build failed", "component", c.Name)
		return StatusDown
	}

	resp, err := p.client.Do(req)
	if err != nil {
		slog.Debug("probe failed", "component", c.Name)
		return StatusDown
	}
	defer func() { _ = resp.Body.Close() }()

	if c.ExpectStatus != 0 && resp.StatusCode != c.ExpectStatus {
		slog.Debug("probe status mismatch", "component", c.Name, "got", resp.StatusCode, "want", c.ExpectStatus)
		return StatusDown
	}

	if c.ExpectBodyContains != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if err != nil || !strings.Contains(string(body), c.ExpectBodyContains) {
			slog.Debug("probe body mismatch", "component", c.Name)
			return StatusDown
		}
	}

	return StatusOperational
}

// aggregate turns per-check statuses (aligned with checks) into a Snapshot,
// preserving the group and component order of the check list.
func aggregate(checks []Check, statuses []Status) *Snapshot {
	var groups []Group
	index := map[string]int{} // group name -> position in groups

	for i, c := range checks {
		gi, ok := index[c.Group]
		if !ok {
			gi = len(groups)
			index[c.Group] = gi
			groups = append(groups, Group{Name: c.Group})
		}
		groups[gi].Components = append(groups[gi].Components, Component{
			Name:   c.Name,
			Status: statuses[i],
		})
	}

	groupStatuses := make([]Status, len(groups))
	for i := range groups {
		componentStatuses := make([]Status, len(groups[i].Components))
		for j, comp := range groups[i].Components {
			componentStatuses[j] = comp.Status
		}
		groups[i].Status = rollup(componentStatuses)
		groupStatuses[i] = groups[i].Status
	}

	return &Snapshot{
		GeneratedAt: time.Now().UTC(),
		Overall:     rollup(groupStatuses),
		Groups:      groups,
	}
}

// rollup reduces child statuses to a parent status: all operational →
// operational, all down → down, anything mixed → degraded. An empty set (which
// should not occur in practice) is unknown.
func rollup(children []Status) Status {
	if len(children) == 0 {
		return StatusUnknown
	}
	allOperational, allDown := true, true
	for _, s := range children {
		if s != StatusOperational {
			allOperational = false
		}
		if s != StatusDown {
			allDown = false
		}
	}
	switch {
	case allOperational:
		return StatusOperational
	case allDown:
		return StatusDown
	default:
		return StatusDegraded
	}
}

// seedSnapshot builds an initial all-"unknown" snapshot so the API has something
// coherent to serve before the first probe cycle finishes.
func seedSnapshot(checks []Check) *Snapshot {
	unknown := make([]Status, len(checks))
	for i := range unknown {
		unknown[i] = StatusUnknown
	}
	snap := aggregate(checks, unknown)
	snap.Overall = StatusUnknown
	for i := range snap.Groups {
		snap.Groups[i].Status = StatusUnknown
	}
	return snap
}
