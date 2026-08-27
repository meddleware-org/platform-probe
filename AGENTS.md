# AGENTS.md — platform-probe

## Purpose

A public status page that reflects **end-to-end health** of the Meddleware
platform's user-facing capabilities. It probes internal health endpoints
server-side on a timer, caches a capability-grouped snapshot, and serves that
snapshot to browsers. Clients never probe; they read the cache.

## Request / probe flow

```
Prober goroutine (every PROBE_INTERVAL):
  probe all checks concurrently (per-probe PROBE_TIMEOUT)
    → component status: condition met ? operational : down
  rollup components → group, groups → overall
  store immutable Snapshot in an atomic cache

Browser (status-page SPA):
  GET /api/status  → cached Snapshot (O(1); no probing on the request path)
  GET /healthz     → 200 liveness (independent of probed dependencies)
```

## Package layout

```
cmd/server/main.go            HTTP server, routing, graceful shutdown, -healthcheck
internal/config/config.go     env config: PORT, PROBE_INTERVAL, PROBE_TIMEOUT, CHECKS_CONFIG_FILE, LOG_LEVEL
internal/checks/config.go     runtime checks config: JSON schema + LoadFile (validate, order)
internal/checks/checks.go     prober (per-cycle reload), probe, aggregation, cache
internal/checks/*_test.go     rollup, order, JSON-has-no-URLs, probe, reload, fail-safe, config parse
config.example.json           dummy example config (no real targets)
```

## Adding or changing a check

Checks are **not** in the source — they are supplied at runtime via
`CHECKS_CONFIG_FILE` (in the cluster, a SOPS-encrypted Secret). Edit that config,
not the code. Each component:

```json
{ "name": "Registry API",
  "url": "http://<service>.<namespace>.svc.cluster.local:<port>/<health-path>",
  "expect_status": 200,
  "expect_body_contains": "" }
```

- `name` and the enclosing group `name` are the only fields that reach clients;
  keep them capability-oriented (never a hostname).
- Prefer a check that exercises a real dependency path (e.g. an endpoint that
  fans out to downstream services) over a shallow liveness endpoint, so the status
  reflects health, not just reachability.
- The service re-reads the file every probe cycle, so edits take effect live.
- If the target is in another namespace, the status pod needs a NetworkPolicy
  ingress allowance there (see the platform repo's `k8s/base/*/networkpolicy.yaml`).
  The netpol ports and the checks config must stay in sync — both live only in the
  private platform repo, so neither leaks publicly.
- `config.example.json` documents the shape with dummy `example.com` targets; never
  put real targets in this (public) repo.

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | HTTP listen port |
| `PROBE_INTERVAL` | `15s` | Time between probe cycles (snapshot freshness) |
| `PROBE_TIMEOUT` | `5s` | Per-probe timeout; must be `< PROBE_INTERVAL` |
| `CHECKS_CONFIG_FILE` | `/etc/platform-probe/checks.json` | Checks file, reloaded each cycle |
| `LOG_LEVEL` | `info` | `debug\|info\|warn\|error` |

## Build, test, run

```bash
go build ./... && go vet ./... && go test -race ./...
PROBE_INTERVAL=15s PORT=8080 go run ./cmd/server
curl -s localhost:8080/api/status | jq .
docker build --build-arg VERSION=v0.1.0 -t quay.io/meddleware-org/platform-probe:v0.1.0 .
```

## Invariants (see CLAUDE.md)

- No probe targets compiled in: checks come only from `CHECKS_CONFIG_FILE`
  (reloaded each cycle); the public repo/image ships only `config.example.json`.
- `/api/status` is topology-free: no URLs, hostnames, ports, or error text.
- Server-side + cached: never probe on the request path.
- Fail safe to `down` on any probe error/timeout/mismatch; a bad config reload
  keeps the last-good set; never panic.
- `/healthz` is process liveness only.
