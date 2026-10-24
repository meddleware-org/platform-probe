# Changelog

All notable changes to platform-probe are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.10] - 2026-10-24

### Changed

- Comments and `llms.txt` name the status UI `status-ui` (workspace naming convention, ADR-0003). No code change.

## [0.1.9] - 2026-10-22

### Security

- Go toolchain 1.26.10 (CVE-2026-111943 / GO-2026-6809: crypto/x509 accepted paths through untrusted intermediates; Trivy flags the 1.26.9 standard library in the published binary). Builder image digest updated.

## [0.1.8] - 2026-10-20

### Changed

- **Image build: native cross-compile, no QEMU.** The builder stage is `FROM --platform=$BUILDPLATFORM` and
  compiles for the target with `GOOS=$TARGETOS GOARCH=$TARGETARCH` (`CGO_ENABLED=0`, `-trimpath` and the
  ldflags unchanged), so the release no longer emulates arm64 and `docker-publish.yml` no longer has the
  `docker/setup-qemu-action` step (a privileged binfmt step). `TARGETOS`/`TARGETARCH` have no default any more:
  a builder that does not set them fails the build instead of silently producing the wrong architecture.
  A build step also asserts, with `go version -m` (it reads the binary without running it), that the compiled
  binary's `GOARCH` equals the target architecture.
  Proof (rootless podman, builder pinned by digest): the new `linux/amd64` image built from the 0.1.7 sources with
  `VERSION=v0.1.7` is **byte-identical to the published 0.1.7** (`/platform-probe` `96bf801a…`, same CA bundle).
  No behaviour change on amd64, the architecture the cluster runs.

### Fixed

- **The published `linux/arm64` image carried an x86-64 binary.** The Dockerfile declared `ARG TARGETARCH=amd64`,
  and that default overrode the platform value, so 0.1.7 and earlier compiled `GOARCH=amd64` in the arm64 build
  too: the arm64 image's `/platform-probe` is byte-identical to the amd64 one (`96bf801a…`, ELF x86-64) and would
  not start on an arm64 node. With no default the arm64 image now contains an ELF aarch64 binary
  (`go version -m`: `GOARCH=arm64`, `GOOS=linux`, `CGO_ENABLED=0`, `-trimpath=true`). The cluster is amd64, so
  nothing deployed was affected.

## [0.1.7] - 2026-10-18

### Security

- A check without `expect_status` is now `operational` only for a 2xx response. Before, it read
  `operational` for any HTTP answer, including a 5xx, so an omitted value turned a health check into a
  reachability check. Redirects are still not followed, so a 3xx also reads `down` unless it is the
  expected status. `expect_status` set explicitly is still an exact match.
- `expect_status` outside 100..599 is rejected when the checks file is loaded (the last good config is kept).

### Fixed

- Stale statements that the probe targets are compiled in (package comment, `llms.txt`, README) now say
  they are loaded at runtime from `CHECKS_CONFIG_FILE`; `llms.txt` no longer lists a `GET /` page the
  binary does not serve.

### Added

- Tests for the redirect refusal (through `NewProber`'s client), the default-2xx rule, the
  `expect_status` range and the `GET /api/status` handler (body, content type, `no-store`, no topology).

### Changed

- The README's cosign command pins the publish workflow and tag refs
  (`docker-publish.yml@refs/tags/v*`), not just the repository.

## [0.1.6] - 2026-10-09

### Security

- The release runs the full Go CI workflow (lint, vet, race tests, govulncheck, filesystem scan) and the image is scanned before it is signed (fixable CRITICAL/HIGH fail).

## [0.1.5] - 2026-10-09

### Security

- Built with Go 1.26.9: govulncheck found eleven net/http and net/textproto advisories in 1.26.7 (GO-2026-6607 to GO-2026-6617). go.mod names the toolchain and the Dockerfile builder is pinned to the same patch and checks it.

## [0.1.4] - 2026-10-01

### Security

- Every response carries headers for a JSON-only API: `Content-Security-Policy: default-src 'none';
  frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy: no-referrer` and `Cross-Origin-Resource-Policy: cross-origin` (cross-origin reads of
  `/api/status` stay allowed). This includes 404 and 405 responses.

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
