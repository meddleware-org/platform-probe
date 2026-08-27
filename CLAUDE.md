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
   unexpected status makes the component `down` — never `operational`, and never a
   panic. A config reload that fails to parse keeps the previous good check set
   rather than blanking the page. A cycle always produces a complete snapshot.

5. **Health, not reachability.** Checks assert a real condition (expected status
   code, optional body substring) — e.g. the registry catalog endpoint, so a 200
   proves the whole chain, not just that a port answers.

6. **`/healthz` is liveness only.** It reports the process being up, independent of
   the probed dependencies, so Kubernetes never restarts the pod because an
   upstream is down.

## Aggregation

`rollup`: all children operational → operational; all down → down; anything mixed
→ degraded. Applied twice — components → group, groups → overall. Group and
component order follow the order in the checks config file.

## Code style

- Match `registry-auth-proxy` / `registry-token-service`: no global state,
  `log/slog`, graceful shutdown, `envOr` for optional config, `cmd/server/main.go`
  thin (routing + wiring only); logic lives in `internal/`.
- The binary is a pure API server — no embedded assets, no HTML. The status page
  UI is the separate `status-page` Vue SPA (in `workspace/apps/status-page/`).

## What not to do

- Do not add write endpoints or accept client input beyond simple GETs.
- Do not source health from unauthenticated client-side probes (the previous
  design) — that exposes endpoints and only tests reachability.
- Do not log probe URLs at info level or include them in any HTTP response.
- Do not add per-request probing "for freshness"; increase `PROBE_INTERVAL`
  resolution instead — clients read the cache.
