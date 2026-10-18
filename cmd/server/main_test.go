package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/meddleware-org/platform-probe/internal/checks"
)

func TestHandlerSecurityHeaders(t *testing.T) {
	h := newHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})
	want := map[string]string{
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "cross-origin",
	}
	cases := []struct {
		method, path string
		code         int
	}{
		{http.MethodGet, "/api/status", http.StatusOK},
		{http.MethodOptions, "/api/status", http.StatusNoContent},
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/nope", http.StatusNotFound},
		{http.MethodPost, "/api/status", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.code {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, rec.Code, c.code)
		}
		for k, v := range want {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("%s %s: %s = %q, want %q", c.method, c.path, k, got, v)
			}
		}
	}
}

func TestStatusKeepsCORS(t *testing.T) {
	h := newHandler(func(w http.ResponseWriter, _ *http.Request) {})
	for _, m := range []string{http.MethodGet, http.MethodOptions} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(m, "/api/status", nil))
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s /api/status: Access-Control-Allow-Origin = %q, want *", m, got)
		}
	}
}

// TestHandleStatusServesTheCachedSnapshot drives the real prober against a local
// target and reads the snapshot through the full handler: body shape, content
// type, no-store caching, the security headers, and that nothing about the probe
// target reaches the response.
func TestHandleStatusServesTheCachedSnapshot(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	cfgPath := filepath.Join(t.TempDir(), "checks.json")
	cfg := `{"groups":[{"name":"Group A","components":[` +
		`{"name":"Good","url":"` + target.URL + `/good","expect_status":200},` +
		`{"name":"Omitted","url":"` + target.URL + `/bad"}]}]}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	prober := checks.NewProber(cfgPath, 2*time.Second)
	h := newHandler(handleStatus(prober))

	// Before the first cycle the seeded snapshot is served (all unknown).
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var seed checks.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &seed); err != nil {
		t.Fatalf("seed body is not JSON: %v: %s", err, rec.Body)
	}
	if seed.Overall != checks.StatusUnknown {
		t.Errorf("seed overall = %q, want unknown", seed.Overall)
	}

	prober.RunOnce(context.Background())
	hitsAfterCycle := hits.Load()

	// Reading the status must never probe: serve it several times and compare.
	for range 5 {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	}
	if n := hits.Load(); n != hitsAfterCycle {
		t.Errorf("target hit %d times after the cycle, want %d: a request must not trigger a probe", n, hitsAfterCycle)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}

	var snap checks.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("body is not JSON: %v: %s", err, rec.Body)
	}
	if snap.GeneratedAt.IsZero() {
		t.Error("generated_at is empty")
	}
	if snap.Overall != checks.StatusDegraded {
		t.Errorf("overall = %q, want degraded (one good, one 503)", snap.Overall)
	}
	if len(snap.Groups) != 1 || snap.Groups[0].Name != "Group A" || len(snap.Groups[0].Components) != 2 {
		t.Fatalf("groups = %+v", snap.Groups)
	}
	got := snap.Groups[0].Components
	if got[0].Name != "Good" || got[0].Status != checks.StatusOperational ||
		got[1].Name != "Omitted" || got[1].Status != checks.StatusDown {
		t.Errorf("components = %+v, want Good operational and Omitted down (a 503 without expect_status)", got)
	}

	// Topology-free: the body names only groups, components and statuses.
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &generic); err != nil {
		t.Fatal(err)
	}
	for k := range generic {
		if k != "generated_at" && k != "overall" && k != "groups" {
			t.Errorf("unexpected top-level field %q", k)
		}
	}
	host := strings.TrimPrefix(target.URL, "http://")
	for _, leak := range []string{host, "127.0.0.1", "/good", "/bad", "http://"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("response leaked %q: %s", leak, rec.Body)
		}
	}
}
