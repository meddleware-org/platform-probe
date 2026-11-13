# CLAUDE.md — platform-probe

## Identity

A small, public-facing status page. It probes the platform's internal health
endpoints **server-side** on a timer, caches an aggregated, capability-grouped
snapshot, and serves that snapshot to browsers. The page is public (reached
through the Cloudflare tunnel), so its output must never leak internal topology.

## Architectural invariants

1. **The snapshot is topology-free.** `GET /api/status` returns only capability
   groups, component names, and coarse statuses. It must never include internal
   URLs, hostnames, ports, tech-stack fingerprints, or error text.

2. **No probe targets are compiled in.** The repo and image are public, so the
   check definitions (names, URLs, conditions) must NOT be hardcoded — otherwise
   anyone who clones the repo or unpacks the image sees exactly what is probed.
   Checks are loaded at runtime from `CHECKS_CONFIG_FILE` (a mounted SOPS Secret
   in the cluster) and reloaded every cycle. The only checks in the repo are the
   dummy `example.com` entries in `config.example.json`. Never reintroduce a
   `DefaultChecks` with real targets.

3. **Probing is server-side and cached.** Browsers only ever read the cached
   snapshot — a cheap O(1) read. Never probe on the request path; never let a
   client trigger a probe. Probe load is fixed (one cycle per `PROBE_INTERVAL`)
   regardless of visitor count.

4. **Fail safe to `down` (and keep last-good config).** A probe error, timeout, or
   unexpected status makes the component `down` (a check without `expect_status` accepts
   only a 2xx; redirects are not followed, so a 3xx is `down` unless expected) — never `operational`, and never a
   panic. A config reload that fails to parse keeps the previous good check set
   rather than blanking the page. A cycle always produces a complete snapshot.

5. **Health, not reachability.** Checks assert a real condition (expected status
   code, optional body substring) — e.g. the registry catalog endpoint, so a 200
   proves the whole chain, not just that a port answers.

6. **`/healthz` is liveness only.** It reports the process being up, independent of
   the probed dependencies, so Kubernetes never restarts the pod because an
   upstream is down.

## Flow and layout

```text
Prober goroutine (every PROBE_INTERVAL):
  probe all checks concurrently (per-probe PROBE_TIMEOUT)
    -> component status: condition met ? operational : down
  rollup components -> group, groups -> overall
  store an immutable Snapshot in an atomic cache
Browser (status-ui SPA): GET /api/status (cached Snapshot) and GET /healthz (liveness)
```

```text
cmd/server/main.go            HTTP server, routing, graceful shutdown, -healthcheck (used by the container HEALTHCHECK)
internal/config/config.go     env config
internal/checks/config.go     runtime checks config: JSON schema + LoadFile (validate, order)
internal/checks/checks.go     prober (per-cycle reload), probe, aggregation, cache
*_test.go                     rollup, order, JSON-has-no-URLs, probe (default 2xx, redirects refused), reload, fail-safe, config
                              parse, security headers, CORS, the status handler body
config.example.json           dummy example config (no real targets)
```

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | HTTP listen port |
| `PROBE_INTERVAL` | `15s` | Time between probe cycles (snapshot freshness) |
| `PROBE_TIMEOUT` | `5s` | Per-probe timeout; must be `< PROBE_INTERVAL` |
| `CHECKS_CONFIG_FILE` | `/etc/platform-probe/checks.json` | Checks file, reloaded each cycle |
| `LOG_LEVEL` | `info` | `debug\|info\|warn\|error` |

```bash
go build ./... && go vet ./... && go test -race ./...
PROBE_INTERVAL=15s PORT=8080 go run ./cmd/server
curl -s localhost:8080/api/status | jq .
docker build --build-arg VERSION=v0.1.0 -t quay.io/meddleware-org/platform-probe:v0.1.0 .
```

## Adding or changing a check

Edit the runtime config (in the cluster, a SOPS-encrypted Secret), not the code. Each component:

```json
{ "name": "Registry API",
  "url": "http://<service>.<namespace>.svc.cluster.local:<port>/<health-path>",
  "expect_status": 200,
  "expect_body_contains": "" }
```

- `expect_status` is the exact status required (100..599); omit it and only a `2xx` passes. Redirects are not followed, so a
  `3xx` fails unless it is the expected code.
- `name` and the enclosing group `name` are the only fields that reach clients; keep them capability-oriented (never a hostname).
- Prefer a check that exercises a real dependency path (an endpoint that fans out to downstream services) over a shallow
  liveness endpoint.
- The service re-reads the file every probe cycle, so edits take effect live.
- If the target is in another namespace, the status pod needs a NetworkPolicy ingress allowance there. The netpol ports and the
  checks config must stay in sync; both live only in the private deployment repository, so neither leaks publicly.

## Aggregation

`rollup`: all children operational → operational; all down → down; anything mixed
→ degraded. Applied twice — components → group, groups → overall. Group and
component order follow the order in the checks config file.

## Code style

- Match `registry-auth-proxy` / `registry-token-service`: no global state,
  `log/slog`, graceful shutdown, `envOr` for optional config, `cmd/server/main.go`
  thin (routing + wiring only); logic lives in `internal/`.
- The binary is a pure API server — no embedded assets, no HTML. The status page
  UI is the separate `status-ui` Vue SPA (the satellite repo `repos/status-ui`).

## What not to do

- Do not add write endpoints or accept client input beyond simple GETs.
- Do not source health from unauthenticated client-side probes (the previous
  design) — that exposes endpoints and only tests reachability.
- Do not log probe URLs at info level or include them in any HTTP response.
- Do not add per-request probing "for freshness"; increase `PROBE_INTERVAL`
  resolution instead — clients read the cache.
