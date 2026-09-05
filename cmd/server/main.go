// Command server runs the platform-probe: a small service that probes the
// platform's internal health endpoints server-side on a timer, caches an
// aggregated, capability-grouped snapshot, and serves it as JSON. Browsers never
// poll this service directly — they load the status-page SPA, which fetches the
// snapshot every 15 s. Probe load is decoupled from visitor count and the internal
// topology is never exposed.
//
// Endpoints:
//
//	GET /api/status  — the cached JSON snapshot (topology-free)
//	GET /healthz     — liveness (200; independent of probe results)
//
// The process also supports a "-healthcheck" argument used by the container
// HEALTHCHECK: it dials the local /healthz endpoint and exits 0 or 1.
//
// Environment variables:
//
//	PORT               — HTTP listen port (default: 8080)
//	PROBE_INTERVAL     — time between probe cycles (default: 15s)
//	PROBE_TIMEOUT      — per-probe timeout, must be < PROBE_INTERVAL (default: 5s)
//	CHECKS_CONFIG_FILE — path to the JSON checks file, reloaded each cycle
//	                     (default: /etc/platform-probe/checks.json)
//	LOG_LEVEL          — info | debug | warn | error (default: info)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meddleware-org/platform-probe/internal/checks"
	"github.com/meddleware-org/platform-probe/internal/config"
)

// version is the build version, overridden at link time via
// -ldflags "-X main.version=...". It defaults to "dev" for local builds.
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthCheck())
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	prober := checks.NewProber(cfg.ChecksConfigFile, cfg.ProbeTimeout)

	// Run the prober for the lifetime of the process. Cancelling proberCtx on
	// shutdown stops the loop.
	proberCtx, stopProber := context.WithCancel(context.Background())
	defer stopProber()
	go prober.Start(proberCtx, cfg.ProbeInterval)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /api/status", withCORS(handleStatus(prober)))
	mux.HandleFunc("OPTIONS /api/status", withCORS(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	srv := &http.Server{
		Addr:              net.JoinHostPort("", cfg.Port),
		Handler:           logRequest(mux),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", srv.Addr, "version", version,
			"probe_interval", cfg.ProbeInterval.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	stopProber()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
	slog.Info("shutdown complete")
}

// healthCheck dials the local /healthz endpoint. Used by the container HEALTHCHECK.
func healthCheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", port))
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// withCORS wraps a handler to allow cross-origin reads of the public /api/status
// endpoint. The endpoint is read-only and contains no sensitive data, so a
// wildcard origin is appropriate.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
		next(w, r)
	}
}

// handleHealth responds 200 to liveness/readiness probes. It reflects the
// process being up, not the health of the probed dependencies.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// handleStatus serves the cached snapshot as JSON. It never probes on the request
// path, so it is a cheap O(1) read regardless of visitor count.
func handleStatus(prober *checks.Prober) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(prober.Snapshot()); err != nil {
			slog.Error("encode snapshot", "err", err)
		}
	}
}

// responseWriter captures the status code for request logging.
type responseWriter struct {
	http.ResponseWriter
	code int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.code = code
	rw.ResponseWriter.WriteHeader(code)
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rw, r)
		slog.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.code,
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
	})
}
