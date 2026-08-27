# platform-probe

A small, self-contained **status page** for the Meddleware platform. It probes the
platform's internal health endpoints **server-side** on a timer, caches an
aggregated, **capability-grouped** snapshot, and serves that snapshot to browsers.
Clients only ever read the cache — a cheap O(1) request — so probe load is fixed
regardless of how many people are watching, and the page never discloses the
cluster's internal topology.

- **Runtime base:** `scratch` (zero OS footprint, no shell, no package manager)
- **Language:** Go (standard library only — no third-party dependencies)
- **Non-root:** runs as UID/GID `65534`
- **License:** BSD Zero Clause ([0BSD](LICENSE))

## Why it works this way

The previous status page probed each service **from the browser**. That exposed
every internal endpoint and health path in page source, scaled load with visitors,
and — using opaque `no-cors` fetches — could only tell whether a port answered, not
whether the service was actually healthy (a 502 showed green).

This service fixes all three: probing is server-side (endpoints stay in the
binary), cached (clients read a snapshot), and condition-based (a check asserts a
real status code / body, so it reflects end-to-end health). See [SECURITY.md](SECURITY.md).

## How it works

```
                    every PROBE_INTERVAL (default 15s)
platform-probe ───────────────────────────────────────▶ internal /health endpoints
      │  caches aggregated snapshot                        (your configured
      ▼                                                      check targets)
  GET /api/status  ◀──────  Browser  (polls the cheap cached snapshot)
```

Each cycle probes all checks concurrently, maps each to `operational`/`down`, then
rolls up: components → group, groups → overall (all ok → operational, all down →
down, mixed → degraded). Point a check at an endpoint that fans out to its
downstream dependencies (rather than a shallow liveness endpoint) so a `200` proves
the whole chain — the end-to-end signal that mere reachability misses.

## Endpoints

| Endpoint | Description |
| --- | --- |
| `GET /api/status` | Cached JSON snapshot (topology-free) |
| `GET /healthz` | Liveness probe (`200`; independent of probed dependencies) |

The binary also supports `-healthcheck`, which dials `/healthz` and exits `0`/`1`
(used by the container `HEALTHCHECK`; Kubernetes uses its own probes).

### Snapshot shape

```json
{
  "generated_at": "2026-08-23T15:00:00Z",
  "overall": "operational",
  "groups": [
    { "name": "Identity & Access", "status": "operational",
      "components": [ { "name": "Sign-in", "status": "operational" },
                      { "name": "Authorization", "status": "operational" } ] }
  ]
}
```

Status vocabulary: `operational | degraded | down` (`unknown` before the first
cycle). No URLs, hostnames, or error text ever appear.

## Configuration

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `PORT` | no | `8080` | HTTP listen port |
| `PROBE_INTERVAL` | no | `15s` | Time between probe cycles (and snapshot freshness) |
| `PROBE_TIMEOUT` | no | `5s` | Per-probe timeout; must be `< PROBE_INTERVAL` |
| `CHECKS_CONFIG_FILE` | no | `/etc/platform-probe/checks.json` | Path to the checks file, reloaded every cycle |
| `LOG_LEVEL` | no | `info` | `debug \| info \| warn \| error` |

### Checks configuration (external, dynamic)

The checks are **not** compiled into the binary — this repo and image are public, so
baking in the real targets would disclose exactly what is probed. Instead they are
loaded at runtime from `CHECKS_CONFIG_FILE` and **re-read every probe cycle**, so
editing the file updates the checks live (no rebuild, no restart). In the cluster the
file is a **SOPS-encrypted Secret** mounted as a directory; the only checks committed
here are the dummy `example.com` entries in [config.example.json](config.example.json).

```json
{
  "groups": [
    { "name": "Identity & Access", "components": [
        { "name": "Sign-in", "url": "http://<internal>/health/ready", "expect_status": 200 } ] }
  ]
}
```

Each component takes `name`, `url`, `expect_status`, and optional
`expect_body_contains`. Group and component order is preserved on the page. A config
that fails to parse is ignored — the last good set keeps running.

## Verifying published images

Images are multi-arch, carry an SBOM + SLSA provenance, and are signed with keyless
[cosign](https://docs.sigstore.dev/):

```bash
cosign verify \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp 'https://github.com/meddleware-org/platform-probe/.*' \
  quay.io/meddleware-org/platform-probe:<tag>
```

## Building

```bash
go build -o platform-probe ./cmd/server     # local binary
go test -race ./...                          # tests
docker build --build-arg VERSION=v0.1.0 \
  -t quay.io/meddleware-org/platform-probe:v0.1.0 .
```

## Contributing / development

See [AGENTS.md](AGENTS.md) for the package layout and how to add a check, and
[CLAUDE.md](CLAUDE.md) for the architectural invariants. CI runs `golangci-lint`,
`go vet`, race tests, `govulncheck`, and a Trivy scan; a `v*` tag triggers a signed
multi-registry release.
