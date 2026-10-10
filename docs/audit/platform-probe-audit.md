# Security Audit — `platform-probe`

**Classification:** Internal security review
**Project:** `repos/platform-probe` — Go standard-library status probe behind `status.meddleware.co.uk/api/status`
**Project type:** Go service + image
**Template:** AUDIT_TEMPLATE.md (2026-10-08) + AUDIT_TEMPLATE_GO.md (2026-10-08) + AUDIT_TEMPLATE_IMG.md (2026-10-08)
Not triggered: AUTH (no token or credential is issued, verified or held; the mounted Secret holds probe targets, not credentials), PLATFORM (the Secret's custody and the cluster are audited with the workspace), PROXY (it forwards nothing), SUI_CLIENT and the chain lenses (no chain access).
**Go:** `go 1.26.6`, `toolchain go1.26.9` in `go.mod`; CI uses `go-version-file: go.mod`; the Dockerfile builder `golang:1.26.9-bookworm@sha256:d9c68c2c…453c` is checked against the `toolchain` line at build time
**Modules:** none (standard library only; no `go.sum`)
**Entry points:** `cmd/server` (one binary, listens on `:8080`, `-healthcheck` mode for the container `HEALTHCHECK`); routes `GET /api/status`, `OPTIONS /api/status`, `GET /healthz`
**Upstreams:** the internal health endpoints named in the mounted checks file (plain GET, no authentication, no credentials sent); none are named in the repository or the image
**Images:** `quay.io/meddleware-org/platform-probe:0.1.6@sha256:c85c5485…a2a7` (Docker Hub mirror; cosign keyless, SPDX SBOM attestation, build provenance; verified 2026-10-09, `verify-digests.sh` 16/16)
**Base images:** build `golang:1.26.9-bookworm@sha256:d9c68c2c…453c`; runtime `scratch` (the binary and the builder's CA bundle only)
**Runtime user:** `USER 65534:65534`   **Runtime FS:** read-only root; the only mount is the checks Secret, read-only
**Deployed by:** `k8s/base/apps/status/deployment.yaml` (namespace `apps`), overlay `k8s/clusters/meddleware-org/apps`; digest from `config/images.yaml`; probe targets from the SOPS-managed Secret `status-checks` (`status-checks-secret.enc.yaml`)
**Build args:** `VERSION` (link-time version), `TARGETOS`, `TARGETARCH`, and the image-label strings `VENDOR`, `DESCRIPTION`, `SOURCE_URL`, `DOCUMENTATION_URL`, `IMAGE_URL` — none secret
**Deployment status:** image `quay.io/meddleware-org/platform-probe` at 0.1.6 (`sha256:c85c5485…`, deployed 2026-10-09) in `k8s/base/apps/status` (namespace `apps`); live `https://status.meddleware.co.uk/api/status` returns the topology-free snapshot (three groups including *Chain Access → Chain index*) with the security headers
**Review date:** 2026-09-18 (first pass) · re-verified 2026-10-03 · re-verified 2026-10-09
**Reviewer:** Internal review
**Severity ceiling:** Low — public but topology-free: it probes internal endpoints on a timer and serves only names and coarse statuses.
**Status:** re-verified 2026-10-09

---

## Executive summary

The probe reads its targets from a mounted Secret, probes them on a ticker with a per-probe
timeout, and serves a cached snapshot of names and statuses — never URLs, hosts or error text.
Clients cannot trigger probes. A bad config edit keeps the last good one; any probe error fails to `down`.

All first-pass findings are resolved, adjudicated or positive, and the open question is closed (the
Secret is mounted read-only, mode 0440). Earlier passes found:

- **F6 (Info, RESOLVED)** — the configuration package had no tests; defaults and duration validation
  (including "timeout shorter than interval") are now covered (100%).
- `govulncheck` is pinned in CI (static-server audit F6).

Re-verified 2026-10-09 (0.1.6; `go vet` and `go test -race` green, coverage checks 92.5%, config 100%,
`cmd/server` 19.6%; the live endpoint and its headers read the same day):

- **F7 (Low, RESOLVED 0.1.4)** — every response, including 404 and 405, now carries JSON-API security
  headers (`Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `nosniff`, `DENY`,
  `no-referrer`, CORP `cross-origin`); CORS for `/api/status` is kept. Tested and seen live.
- **F8 (Low, RESOLVED 0.1.5)** — `govulncheck` found eleven `net/http` and `net/textproto` advisories
  (GO-2026-6607…6617) in Go 1.26.7; the binary is built with Go 1.26.9, `go.mod` names the toolchain and
  the Dockerfile fails the build if the pinned builder differs.
- **F9 (Low, RESOLVED 0.1.6)** — the release runs the full Go CI workflow (lint, vet, race tests,
  govulncheck, filesystem scan) and the published image is scanned before cosign signs it. The known web-repo
  gap (the reusable CI workflow lacking the unit tests) is **not present here**: `go-ci.yml` runs
  `go test -race`, and the release `verify` job calls it.
- Defects found by this pass that are **not yet fixed** (code changes are outside this alignment; each is a
  small change for the next patch release): **F10 (Low, DEFERRED)** a component without `expect_status`
  reads `operational` for any HTTP response, including a 5xx; **F11 (Info, DEFERRED)** stale statements about
  where the probe targets live (a package comment, `llms.txt`, the README); **F14 (Info, DEFERRED)** coverage
  gaps (the redirect policy and the status handler are untested).
- Accepted or maintainer items: **F12** the README's cosign command pins the repository, not the workflow
  (ACCEPTED-RISK); **F13** the registry mirror and credential inventory (DEFERRED, maintainer).

The severity ceiling stays Low. The live Secret is SOPS-encrypted and was not decrypted: that every live
component sets `expect_status` (F10) is not verified here.

## Threat model / trust boundaries

| Actor | Holds / proves | Can do | Bounded by |
| --- | --- | --- | --- |
| Anonymous client | requests | read the snapshot; CORS `*` | names and statuses only (I1); cached (I2); GET/OPTIONS only (I7) |
| Anonymous client (volume) | requests | try to amplify probes | probes run on the ticker only |
| Image or repo reader | the image | look for targets | targets live only in the mounted Secret |
| Config editor (maintainer) | the Secret | break the config; point probes at any URL; omit `expect_status` | last good config kept; failures read `down`; an omitted `expect_status` is lenient (F10); access is SOPS custody (workspace) |
| Probed service | its response | answer slowly, with a redirect, with a huge body | per-probe timeout, redirects not followed, 64 KiB body cap (I5) |
| Go toolchain, base image, CI publish job | the compiled code and the signed image | ship altered bytes | digest-pinned builder checked against the `go.mod` toolchain, pinned `govulncheck`, Trivy before cosign, keyless signature (F8, F9) |

## Severity scale

Critical / High / Medium / Low / Info / Positive.

## Scope

- **In scope (0.1.6):** `cmd/server/**`, `internal/checks/**`, `internal/config/**`, tests, `Dockerfile`,
  `.dockerignore`, workflows, `.github/dependabot.yml`, `SECURITY.md`, `README.md`, `llms.txt`,
  `k8s/base/apps/status/deployment.yaml` and `networkpolicy.yaml` (read-only).
- **Out of scope:** the probed services; Secret custody (SOPS, `SECRETS.md`).
- **Environment (2026-10-09):** `go vet`; `go test -race -cover` (checks 92.5%, config 100%, `cmd/server`
  19.6%) with Go 1.27.1 locally (CI uses the 1.26.9 toolchain); `govulncheck` v1.8.0 is the CI gate (not
  installed locally, so not re-run here; 0.1.5 records the move to 1.26.9 after it flagged 1.26.7); live `GET`, `POST` and `OPTIONS`
  of `status.meddleware.co.uk/api/status`.

## Findings

### F1 — `SECURITY.md` said targets were compiled in

**Severity:** Info   **Disposition:** RESOLVED — they live only in the mounted Secret.
**Remediation / evidence (2026-10-09):** `SECURITY.md` and `CLAUDE.md` are accurate; three other texts still
say the opposite (F11).

### F2 — Late bind failure exits from a goroutine

**Severity:** Low   **Disposition:** ADJUDICATED — the pod restarts; nothing to drain.
**Remediation / evidence (2026-10-09):** unchanged: `ListenAndServe` failing other than `ErrServerClosed`
logs and `os.Exit(1)` (`cmd/server/main.go`).

### F3 — Redirects followed

**Severity:** Low   **Disposition:** RESOLVED — `CheckRedirect` returns the 3xx, which fails the
expected-200 check.
**Remediation / evidence (2026-10-09):** `NewProber` sets `CheckRedirect` to `http.ErrUseLastResponse`. No
test pins it: `TestProbe` builds the `Prober` with a plain `http.Client` (F14).

### F4 — Response bodies bounded

**Severity:** Positive — 64 KiB `LimitReader`. Re-verified 2026-10-09: the body is read only when
`expect_body_contains` is set (otherwise it is closed unread), and the read is capped at 64 KiB.

### F5 — CORS `*`

**Severity:** Info   **Disposition:** ADJUDICATED — read-only, uncredentialed, topology-free.
**Remediation / evidence (2026-10-09):** `TestStatusKeepsCORS`; the live response carries
`Access-Control-Allow-Origin: *`, `Allow-Methods: GET, OPTIONS`, `Allow-Headers: Accept, Content-Type`.

### F6 — Configuration untested

**Severity:** Info   **Disposition:** RESOLVED (`platform-probe` main)
**Where:** `internal/config`
**Issue / impact:** the validation that keeps a probe cycle shorter than its interval had no test.
**Remediation / evidence:** `config_test.go` covers defaults and every rejected duration (100%).

### F7 — No security headers on the API responses

**Severity:** Low   **Disposition:** RESOLVED (0.1.4, `e71f0c9`)
**Where:** `cmd/server/main.go` (`withSecurityHeaders`)
**Issue / impact:** the JSON API served no `Content-Security-Policy`, `nosniff`, framing or referrer
headers, so a browser could sniff or frame a response and leak a referrer.
**Remediation / evidence:** the wrapper sets `Content-Security-Policy: default-src 'none';
frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy:
no-referrer` and `Cross-Origin-Resource-Policy: cross-origin` (cross-origin reads of the status stay
allowed) on every response, including mux 404 and 405. Pinned by `TestHandlerSecurityHeaders` (five paths
and methods). Live 2026-10-09: the same five headers on `GET` and `OPTIONS`; `POST` returns 405; HSTS comes from
the edge.

### F8 — Go standard-library advisories in 1.26.7

**Severity:** Low   **Disposition:** RESOLVED (0.1.5 `c9e5f24`; 0.1.6 `48590c2`)
**Where:** `go.mod`, `Dockerfile`
**Issue:** `govulncheck` reported eleven `net/http` and `net/textproto` advisories (GO-2026-6607 to
GO-2026-6617) in Go 1.26.7, the version the `golang:1.26` builder and CI had resolved.
**Impact:** the server is `net/http`-based and public, so the standard-library advisories applied to it.
**Remediation / evidence:** `go.mod` names `toolchain go1.26.9`; the Dockerfile builder is
`golang:1.26.9-bookworm` pinned by digest and the build fails if `go env GOVERSION` differs from the
`toolchain` line, so the compiler is the one CI's `govulncheck` checks; CI reads the version from `go.mod`.
The digest is bumped through Dependabot's Docker group (the module has no dependencies for the `gomod`
ecosystem).

### F9 — Release gate and image scan

**Severity:** Low   **Disposition:** RESOLVED (0.1.6, `48590c2`)
**Where:** `.github/workflows/docker-publish.yml`, `.github/workflows/go-ci.yml`
**Issue:** the image release ran its own, smaller gate, and the published image was not scanned before it
was signed.
**Impact:** a tag could ship what CI would have refused.
**Remediation / evidence:** `go-ci.yml` has a `workflow_call` trigger and four jobs (golangci-lint v2.13.0
with the default linters, `go vet` plus `go test -v -race -coverprofile`, `govulncheck` v1.8.0, a Trivy
filesystem scan for vulnerabilities, misconfiguration and secrets at CRITICAL/HIGH with `exit-code: 1`);
the release `verify` job calls it and the public publish job `needs` it. The public job builds
`linux/amd64,linux/arm64` with `provenance: mode=max`, runs Trivy on the pushed digest (CRITICAL/HIGH,
fixable only, `exit-code: 1`) before `cosign sign`, then an SPDX SBOM attestation and build provenance for
quay.io and Docker Hub, with no `continue-on-error`. The only `continue-on-error` is the self-hosted mirror
(F13). Not applicable here: the lockfile and `/THIRD_PARTY_LICENSES` additions of the web repos (no
modules, no bundled npm code).

### F10 — A component without `expect_status` reads operational for any response

**Severity:** Low   **Disposition:** DEFERRED (next patch release; Section D pre-testnet "any failure reads `down`")
**Where:** `internal/checks/checks.go` (`probe`: `if c.ExpectStatus != 0 && …`), `internal/checks/config.go`
(`LoadFile` validates name and URL only)
**Issue:** `expect_status` is optional in the JSON and defaults to `0`, which skips the status comparison.
A component that omits it reads `operational` for any HTTP answer, including `500` and `503`, as long as
the connection succeeds (and, when `expect_body_contains` is also omitted, regardless of the body). The
README and `config.example.json` always show `expect_status`, and the tests that load files do so with it,
but nothing rejects an entry without it. This contradicts the invariants in `SECURITY.md` (3, "probe errors
never render as operational") and `CLAUDE.md` (5, "health, not reachability").
**Impact:** a typo or omission in the Secret turns a health probe into a reachability probe on the public
status page, and an outage could show green. The maintainer edits the Secret, so this is a footgun, not an
attack. Whether any live component omits it was not checked (the Secret is encrypted).
**Remediation / evidence:** make `LoadFile` reject (or default to `200`) an `expect_status` outside
100..599, and add a test (`TestLoadFileErrors`, `TestProbe`).

### F11 — Stale statements about where the probe targets live

**Severity:** Info   **Disposition:** DEFERRED (next patch release; documentation)
**Where:** `internal/checks/checks.go:5` (package comment: "The internal probe URLs live here in code"),
`llms.txt` ("Checks are defined in code (internal/checks/checks.go)" and "`GET /` — the embedded status
page"), `README.md` ("endpoints stay in the binary"), `CLAUDE.md` (the status-page path
`workspace/apps/status-page/`); the workspace's `k8s/clusters/meddleware-org/apps/kustomization.yaml` header
still describes an embedded page (out of scope)
**Issue:** the code loads targets only from `CHECKS_CONFIG_FILE` and serves no page; these texts say the
opposite. It is the same defect class as F1.
**Impact:** none on behaviour; a reader (or a model reading `llms.txt`) is misled about the central
invariant, and a contributor could reintroduce compiled-in targets in good faith.
**Remediation / evidence:** correct the four texts; the invariant itself is stated correctly in
`SECURITY.md`, `AGENTS.md` and the other half of the package comment.

### F12 — The published cosign command pins the repository, not the workflow

**Severity:** Info   **Disposition:** ACCEPTED-RISK
**Where:** `README.md` ("Verifying published images": `--certificate-identity-regexp
'https://github.com/meddleware-org/platform-probe/.*'`); `bootstrap/images/verify-digests.sh` (workspace)
**Issue:** the command and the cluster check accept any workflow identity of the repository. IMG lens:
anchor to the publish workflow and tag refs.
**Impact:** a workflow added by someone with write access could sign an image the check would accept.
**Remediation / evidence:** the repository is the signing boundary; anchoring to
`docker-publish.yml@refs/tags/v*` is a `COSIGN_IDENTITY_REGEXP` override in the workspace script and a
one-line README change. The deployed digest verified 2026-10-09 (16/16).

### F13 — Self-hosted registry mirror and registry credentials

**Severity:** Info   **Disposition:** DEFERRED (maintainer; `OPERATOR_TASKS.md` "Image registry credentials — record scope and rotation")
**Where:** `docker-publish.yml` private mirror jobs (`continue-on-error: true`); `QUAY_TOKEN`, `DOCKERHUB_TOKEN`, `PRIVATE_REGISTRY_*`
**Issue:** the mirror jobs fail without registry credentials and never sign; the registry tokens are
long-lived and not yet inventoried.
**Impact:** the mirror may lag; a leaked token could push an unsigned tag (the cluster pins digests and
verifies signatures, so it would not run).
**Remediation / evidence:** the mirror is listed as best-effort; the public job has no `continue-on-error`.
The credential inventory (scope, holder, expiry, rotation) is the maintainer item.

### F14 — Coverage gaps: redirect policy and the status handler

**Severity:** Info   **Disposition:** DEFERRED (next patch release; tests only)
**Where:** `internal/checks/checks_test.go` (`TestProbe`), `cmd/server/main_test.go`
**Issue:** `TestProbe` constructs `&Prober{client: &http.Client{}}`, bypassing the `CheckRedirect` set in
`NewProber`, so the F3 fix is unpinned; `handleStatus` (the JSON body, `Content-Type`, `Cache-Control:
no-store`) is exercised only through the header tests with a stub handler; `cmd/server` is at 19.6%.
**Impact:** a refactor could re-enable redirects or change the response shape without a test failing.
**Remediation / evidence:** a test with an `httptest` server that answers 302 through `NewProber`'s client,
and a `handleStatus` test of the full body (the earlier suggestion).

## Section A — Invariant verification matrix

| # | Invariant | Enforced at | Proven by | Status |
| --- | --- | --- | --- | --- |
| I1 | Output is names and statuses only | `checks.go` snapshot types | `TestSnapshotJSONHasNoURLs`; live body read 2026-10-09 | HOLDS |
| I2 | Probes run only on the server's ticker | `Prober.Start`; handlers read the cached snapshot | source | HOLDS |
| I3 | A probe error, timeout or unexpected status reads `down` | `probe` | `TestProbe` (502, body mismatch, unreachable) | HOLDS — except an omitted `expect_status` (I8) |
| I4 | A bad config keeps the last good one | `reload` | `TestReloadKeepsLastGoodOnBadEdit`, `TestRunOnceReloadsConfigDynamically` | HOLDS |
| I5 | Every probe and body is bounded | per-probe context timeout; 64 KiB cap | `TestProbe` | HOLDS (F4) |
| I6 | A cycle never outlasts its interval | `config.Load` | `TestLoadRejectsBadDurations` | HOLDS (F6) |
| I7 | Only GET (and HEAD) and OPTIONS on two routes; every response carries the security headers | `ServeMux` method patterns; `withSecurityHeaders` | `TestHandlerSecurityHeaders` (incl. 404, 405) | HOLDS (F7) |
| I8 | A health check asserts a real condition | `expect_status` is optional (`0` skips the comparison) | none | GAP — see F10 |
| I9 | Redirects are not followed | `NewProber` `CheckRedirect` | none | HOLDS (code-only) — F3, F14 |

### Lens categories

| Lens | Category | Status |
| --- | --- | --- |
| GO | HTTP server hygiene | HOLDS — `ReadHeaderTimeout` 10 s, `WriteTimeout` 15 s, `IdleTimeout` 60 s; `MaxHeaderBytes` not set (the 1 MiB default); no handler reads a body; methods allow-listed by the mux (405); no panic path on attacker-reachable input |
| GO | Outbound calls | HOLDS (I5) — per-probe context deadline (`PROBE_TIMEOUT` 5 s < `PROBE_INTERVAL` 15 s), no TLS override, redirects refused, body read capped |
| GO | Input & path handling | N/A — no paths served, no client input used; the checks file is operator-controlled and parsed with `DisallowUnknownFields` |
| GO | AuthN / AuthZ | N/A — public, read-only, topology-free |
| GO | Secrets | HOLDS — targets only in the read-only Secret mount (0440, fsGroup 65534); the URL is never logged (debug logs carry the component name) |
| GO | Client identity | N/A — no rate limit or audit key |
| GO | Concurrency | HOLDS — `go test -race` green; the snapshot is an immutable value swapped through `atomic.Pointer`; the checks slice is guarded by a mutex; one goroutine per check per cycle, joined before the next |
| GO | Graceful shutdown | HOLDS — SIGTERM/SIGINT stop the prober and call `Server.Shutdown` with a 10 s deadline |
| GO | Health & version endpoints | HOLDS — `/healthz` is `{"status":"ok"}`, independent of the probes; there is no `/version` (the build version is logged at start) |
| GO | Error handling | HOLDS — errors are checked (`errcheck` is in golangci-lint's defaults); responses carry no error text |
| GO | CI | HOLDS (F9) — golangci-lint (default linters, no config file), vet, race tests with coverage, pinned `govulncheck`, Trivy filesystem scan |
| IMG | Base images, build context, reproducible build, no secrets | HOLDS — builder digest-pinned and checked against the toolchain; `.dockerignore` excludes Markdown files, `.git`, `.github` and `.env*`; stdlib-only, `CGO_ENABLED=0`, `-trimpath`, version injected at link time; no secret in any `ARG`/`ENV`; only the binary and the CA bundle reach `scratch` |
| IMG | Runtime user & filesystem | HOLDS — `USER 65534:65534`; the pod sets `runAsNonRoot`, read-only root, no privilege escalation, all capabilities dropped, `RuntimeDefault` seccomp, `automountServiceAccountToken: false`, probes on `/healthz`, requests 5m/16Mi and limits 100m/48Mi; ingress is default-deny with allowances for nginx-ingress and monitoring; no egress policy (the probe must reach other namespaces, whose own policies admit it) |
| IMG | Scan, SBOM and notices | HOLDS (F9) — Trivy on the published digest before cosign; SBOM attestation from the image; no third-party code is redistributed (standard library compiled in, as in static-server; the builder's CA bundle is the only copied file) |
| IMG | Verification command | GAP accepted — the identity pins the repository (F12) |
| IMG | Deployment pinning | HOLDS — digest `sha256:c85c5485…` identical in `config/images.yaml` and the cluster kustomization; the base manifest's tag label (`0.1.2`) is overridden by the overlay digest and is cosmetic |

## Section B — Supply-chain, publish-authority & capability matrix

### B.1 Dependency & CVE risk

| Dependency | Pinned version | Liveness dependency? | CVE / audit status | Notes |
| --- | --- | --- | --- | --- |
| Go standard library | Go 1.26.9 (`go.mod` toolchain; builder digest) | everything | `govulncheck` v1.8.0 in CI; the 1.26.7 advisories fixed (F8) | no modules |
| `golang:1.26.9-bookworm` | digest-pinned | build | Trivy filesystem scan in CI; the CA bundle is copied from it | Dependabot Docker group |
| Probed internal services | the checks file | each reads `down` if unreachable | — | fail to `down` |
| Tooling in CI | golangci-lint v2.13.0, govulncheck v1.8.0, Trivy v0.74.0 | no | — | pinned; actions pinned by SHA |

### B.2 Publish authority, capabilities & secret custody

| Authority / secret | Where held | Custody | Gates | Rotation |
| --- | --- | --- | --- | --- |
| `status-checks` Secret | cluster (SOPS-encrypted source `status-checks-secret.enc.yaml`, age) | maintainer | probe targets | on change |
| `QUAY_TOKEN`, `DOCKERHUB_TOKEN`, `PRIVATE_REGISTRY_*` | GitHub secrets | long-lived robot accounts (inventory: `OPERATOR_TASKS.md`) | image push | F13 |
| image signing | GitHub Actions | cosign keyless | images | n/a |

CI & release integrity: actions pinned by SHA (workflows read 2026-10-09); explicit `permissions:` per
workflow and job (`id-token` and `attestations` only on the signing job); image release = full CI via
`workflow_call` + Trivy + cosign + SPDX SBOM attestation + provenance, no `continue-on-error` on the public
path (F9); Dependabot weekly and grouped for Docker and Actions (`.github/dependabot.yml`); no test-only
build mode exists; no job spends real funds.

## Section C — Test-coverage & hermetic/live split

### C.1 Coverage grade — A− (checks 92.5%, config 100%, `cmd/server` 19.6%, 2026-10-09)

Tests: rollup and aggregation order, the snapshot JSON has no URLs, `probe` against OK/502/body-match/
unreachable servers, config load and reload (valid, errors, missing file, last-good kept), duration
validation, and the handler's security headers and CORS (stub handler). Not covered: the redirect refusal
and the status handler's body (F14), an omitted `expect_status` (F10). `go test -race` is part of CI and of
the release gate (F9).

### C.2 Hermetic vs. live paths

| Path | Hermetic? | Deferred to | Tracking |
| --- | --- | --- | --- |
| Probing and serving | yes | — | `go test` |
| Real targets, the mounted Secret | no | deployment | status page live sweep; live read 2026-10-09 (all components `operational`) |

## Section D — Deployment-readiness gates

### pre-localnet

- [x] builds; tests, vet, lint, govulncheck green (CI; vet and race tests re-run 2026-10-09)

### pre-testnet

- [x] deployed; Secret mounted read-only; `SECURITY.md` accurate (F1)
- [x] image: digest-pinned builder, `scratch`, non-root, restricted pod, probes and limits, deployed by digest, signed with SBOM and provenance, scanned before signing (F9)
- [x] every test project runs in CI (`go test -race` in `go-ci.yml`, called by the release)
- [ ] any failure reads `down`, including a check without `expect_status` — F10 (next patch)
- [ ] documentation matches the invariant that targets are not compiled in — F11 (next patch)

### pre-mainnet

- [x] no additional gate (testnet-independent service)
- [ ] registry credential inventory and rotation (F13) — `OPERATOR_TASKS.md` "Image registry credentials"

## Cross-project themes

- **Topology hiding** — public status without internal names or URLs.
- **Supply chain & release integrity** — no dependencies; Go toolchain pinned and checked against the
  builder; pinned scanners; Trivy before signing; signed images with SBOM and provenance.
- **Deployment readiness** — Section D.

## Normative requirements (MUST / MUST NOT)

- **GO-M1–GO-M8** — hold (GO-M3 path confinement, GO-M4 token verification and GO-M6 rate limits N/A: no
  paths, no tokens, cached read-only endpoint; GO-M1 holds with the default header limit).
- **IMG-M1–IMG-M8** — hold; IMG-M8's verification command pins the repository only (F12).

## Implementation suggestions (SHOULD / MAY)

- SHOULD reject an `expect_status` outside 100..599 in `LoadFile` (F10).
- SHOULD test the redirect refusal through `NewProber`'s client and the `handleStatus` body (F14).
- SHOULD correct the four stale texts (F11).
- MAY set `MaxHeaderBytes` explicitly (a few KiB) since no handler needs more.

## Open questions (`OQ#`)

- **OQ1** — Is the Secret directory read-only to UID 65534? (Decided 2026-10-03: yes — `readOnly:
  true`, `defaultMode: 0440`, `fsGroup: 65534`. Re-read 2026-10-09 in `deployment.yaml`: unchanged.)

## Risks

- **Probe-target drift** — targets are maintained by hand in the Secret.
- **Lenient check entries** — an entry without `expect_status` can show green during an outage (F10).
- **Stale snapshot** — the page shows `generated_at`; if the prober loop stopped, the cached snapshot would
  stay (the process exits on a failed listener, not on a stalled loop).

## Re-verification log

- 2026-09-18 — first-pass baseline (F1–F5).
- 2026-10-01 — 0.1.4: security headers on every response.
- 2026-10-03 — re-verified under AUDIT_TEMPLATE.md + GO + IMG (Phase 7): F6 RESOLVED (config tests);
  govulncheck pinned; OQ1 decided.
- 2026-10-08 — Lens dates reconciled with the registry (`check-template-dates.mjs`): base 2026-10-08, and SUI_CLIENT/GO 2026-10-08 and TS 2026-10-03 where cited. The changes (AUTH/PLATFORM/MCP/DB registered, the GO token row moved to AUTH, JSR in trusted publishing, layered injection guards) alter no disposition here.
- 2026-10-09 — re-verified at 0.1.6 (`48590c2`) against the code, the workflows, the manifests and the live endpoint. IMG date now 2026-10-08; front matter gains the Go and IMG fields and the deployment status (0.1.6, digest `sha256:c85c5485…`). F1–F6 re-checked (F3 evidence: the redirect policy is unpinned by a test; F4 body read only when a substring is expected). New: F7 RESOLVED (security headers, 0.1.4), F8 RESOLVED (Go 1.26.9 and the toolchain check, 0.1.5), F9 RESOLVED (release runs go-ci, Trivy before cosign, 0.1.6; the web-repo test gap is absent), F10 DEFERRED (omitted `expect_status` reads operational), F11 DEFERRED (stale texts), F14 DEFERRED (coverage), F12 ACCEPTED-RISK, F13 DEFERRED (maintainer). Counts: vet and `go test -race` green. The live endpoint now includes *Chain Access → Chain index*; the SOPS Secret was not decrypted. Section D gates ticked with evidence; unticked: two next-patch items (F10, F11) and the maintainer item F13.
