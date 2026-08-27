package checks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRollup(t *testing.T) {
	tests := []struct {
		name     string
		children []Status
		want     Status
	}{
		{"all operational", []Status{StatusOperational, StatusOperational}, StatusOperational},
		{"all down", []Status{StatusDown, StatusDown}, StatusDown},
		{"mixed is degraded", []Status{StatusOperational, StatusDown}, StatusDegraded},
		{"degraded child bubbles to degraded", []Status{StatusOperational, StatusDegraded}, StatusDegraded},
		{"single operational", []Status{StatusOperational}, StatusOperational},
		{"empty is unknown", nil, StatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rollup(tt.children); got != tt.want {
				t.Errorf("rollup(%v) = %q, want %q", tt.children, got, tt.want)
			}
		})
	}
}

func TestAggregatePreservesOrderAndGroups(t *testing.T) {
	checks := []Check{
		{Group: "A", Name: "a1"},
		{Group: "B", Name: "b1"},
		{Group: "A", Name: "a2"},
	}
	// a1 down, b1 ok, a2 ok  → group A degraded, group B operational, overall degraded
	snap := aggregate(checks, []Status{StatusDown, StatusOperational, StatusOperational})

	if len(snap.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(snap.Groups))
	}
	if snap.Groups[0].Name != "A" || snap.Groups[1].Name != "B" {
		t.Fatalf("group order = %q,%q, want A,B", snap.Groups[0].Name, snap.Groups[1].Name)
	}
	if len(snap.Groups[0].Components) != 2 ||
		snap.Groups[0].Components[0].Name != "a1" || snap.Groups[0].Components[1].Name != "a2" {
		t.Errorf("component order within A wrong: %+v", snap.Groups[0].Components)
	}
	if snap.Groups[0].Status != StatusDegraded {
		t.Errorf("group A status = %q, want degraded", snap.Groups[0].Status)
	}
	if snap.Overall != StatusDegraded {
		t.Errorf("overall = %q, want degraded", snap.Overall)
	}
}

func TestSnapshotJSONHasNoURLs(t *testing.T) {
	checks := []Check{
		{Group: "Container Registry", Name: "Registry API", URL: "http://secret-internal:8181/v2/_catalog"},
	}
	b, err := json.Marshal(aggregate(checks, []Status{StatusOperational}))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"secret-internal", "8181", "_catalog"} {
		if contains(string(b), leak) {
			t.Errorf("snapshot JSON leaked internal detail %q: %s", leak, b)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestProbe(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"repositories":[]}`))
	}))
	defer okSrv.Close()
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer badSrv.Close()

	p := &Prober{timeout: 2 * time.Second, client: &http.Client{}}
	ctx := context.Background()

	if got := p.probe(ctx, Check{URL: okSrv.URL, ExpectStatus: 200}); got != StatusOperational {
		t.Errorf("200 vs expect 200: got %q, want operational", got)
	}
	if got := p.probe(ctx, Check{URL: badSrv.URL, ExpectStatus: 200}); got != StatusDown {
		t.Errorf("502 vs expect 200: got %q, want down", got)
	}
	if got := p.probe(ctx, Check{URL: okSrv.URL, ExpectStatus: 200, ExpectBodyContains: "repositories"}); got != StatusOperational {
		t.Errorf("body match: got %q, want operational", got)
	}
	if got := p.probe(ctx, Check{URL: okSrv.URL, ExpectStatus: 200, ExpectBodyContains: "nope"}); got != StatusDown {
		t.Errorf("body mismatch: got %q, want down", got)
	}
	if got := p.probe(ctx, Check{URL: "http://127.0.0.1:1/x", ExpectStatus: 200}); got != StatusDown {
		t.Errorf("unreachable: got %q, want down (not a panic)", got)
	}
}

// writeConfig writes a checks config with a single group "G" whose one component
// probes url, and returns the file path.
func writeConfig(t *testing.T, dir, url string) string {
	t.Helper()
	path := filepath.Join(dir, "checks.json")
	body := `{"groups":[{"name":"G","components":[{"name":"c","url":"` + url + `","expect_status":200}]}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewProberSeedsFromConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "http://127.0.0.1:1/x")
	p := NewProber(path, time.Second)
	snap := p.Snapshot()
	if snap.Overall != StatusUnknown {
		t.Fatalf("seed overall = %q, want unknown", snap.Overall)
	}
	if len(snap.Groups) != 1 || snap.Groups[0].Name != "G" {
		t.Fatalf("seed should reflect config groups, got %+v", snap.Groups)
	}
}

func TestNewProberMissingConfigStillStarts(t *testing.T) {
	p := NewProber(filepath.Join(t.TempDir(), "absent.json"), time.Second)
	if p.Snapshot() == nil {
		t.Fatal("Snapshot must never be nil, even with no config")
	}
	if p.Snapshot().Overall != StatusUnknown {
		t.Errorf("overall = %q, want unknown", p.Snapshot().Overall)
	}
	// A cycle with no checks must not panic and stays unknown.
	p.RunOnce(context.Background())
	if p.Snapshot().Overall != StatusUnknown {
		t.Errorf("after empty cycle overall = %q, want unknown", p.Snapshot().Overall)
	}
}

func TestRunOnceReloadsConfigDynamically(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	dir := t.TempDir()
	path := writeConfig(t, dir, up.URL) // component "c" is healthy
	p := NewProber(path, time.Second)

	p.RunOnce(context.Background())
	if got := p.Snapshot().Overall; got != StatusOperational {
		t.Fatalf("overall = %q, want operational", got)
	}

	// Rewrite the config so the same component now points at a dead address.
	// A later cycle must reflect the change with no restart.
	if err := os.WriteFile(path,
		[]byte(`{"groups":[{"name":"G","components":[{"name":"c","url":"http://127.0.0.1:1/x","expect_status":200}]}]}`),
		0o600); err != nil {
		t.Fatal(err)
	}
	p.RunOnce(context.Background())
	if got := p.Snapshot().Overall; got != StatusDown {
		t.Errorf("after reload overall = %q, want down", got)
	}
}

func TestReloadKeepsLastGoodOnBadEdit(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	dir := t.TempDir()
	path := writeConfig(t, dir, up.URL)
	p := NewProber(path, time.Second)
	p.RunOnce(context.Background())
	if got := p.Snapshot().Overall; got != StatusOperational {
		t.Fatalf("overall = %q, want operational", got)
	}

	// Corrupt the file: reload must keep the last-good checks, not blank out.
	if err := os.WriteFile(path, []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.RunOnce(context.Background())
	snap := p.Snapshot()
	if len(snap.Groups) != 1 || snap.Groups[0].Components[0].Name != "c" {
		t.Errorf("bad edit should keep last-good checks, got %+v", snap.Groups)
	}
	if snap.Overall != StatusOperational {
		t.Errorf("overall = %q, want operational (still probing last-good target)", snap.Overall)
	}
}
