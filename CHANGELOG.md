# Changelog

All notable changes to platform-probe are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.1] - 2026-09-05

### Fixed

- Added `Access-Control-Allow-Origin: *`, `Access-Control-Allow-Methods: GET, OPTIONS`,
  and `Access-Control-Allow-Headers: Accept, Content-Type` response headers to
  `GET /api/status` and `OPTIONS /api/status`. The endpoint is public and read-only;
  cross-origin consumers such as the dashboard `StatusWidget` were previously blocked
  by the browser's CORS policy.

## [0.1.0] - 2026-08-24

### Added

- Initial server-side JSON API: a prober goroutine probes internal health
  endpoints every `PROBE_INTERVAL` (default 15s), aggregates results into a
  capability-grouped snapshot, and caches it; clients read the cached snapshot at
  `GET /api/status` (an O(1) read — no probing on the request path).
- Runtime-loaded, dynamically-reloaded checks: check definitions (group/component
  names, URLs, conditions) are read from `CHECKS_CONFIG_FILE` and re-read every
  probe cycle, so **no probe targets are compiled into the binary** — the public
  repo/image ships only a dummy `config.example.json`. A config that fails to parse
  is ignored (last-good set kept), so a bad edit never blanks the page. In the
  cluster the file is a SOPS-encrypted Secret mounted as a directory, so editing it
  updates the checks live with no rebuild or restart.
- Condition-based checks (expected status code + optional body substring). The
  "Registry API" check hits the auth-proxy catalog endpoint so a 200 reflects the
  whole registry auth chain, not mere reachability.
- Topology-free snapshot: `/api/status` exposes only groups, component names, and
  coarse statuses (`operational | degraded | down`); internal URLs live in code and
  never reach clients. The status page UI is the separate `status-page` Vue SPA.
- `-healthcheck` CLI flag and container `HEALTHCHECK`; `/healthz` liveness that is
  independent of probed dependencies.
- Standard-library unit tests: rollup, aggregation order, snapshot-has-no-URLs,
  probe conditions/fail-safe, and an httptest end-to-end cache update.
- Reproducible Dockerfile (`scratch`, non-root 65534, base pinned by digest) with
  full OCI labels.
- CI (golangci-lint, `go vet`, race tests, govulncheck, Trivy) and a signed,
  multi-arch, multi-registry release pipeline (cosign + SBOM + SLSA provenance).
- Operator documentation: `README.md`, `AGENTS.md`, `CLAUDE.md`, `SECURITY.md`,
  `.env.example`, `llms.txt`.
