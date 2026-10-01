package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
