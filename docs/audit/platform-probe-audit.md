# Security Audit — `platform-probe`

**Classification:** Internal security review
**Project:** `repos/platform-probe` — Go standard-library status probe behind `status.meddleware.co.uk/api/status`
**Project type:** Go service + image
**Template:** AUDIT_TEMPLATE.md (2026-10-08) + AUDIT_TEMPLATE_GO.md (2026-10-08) + AUDIT_TEMPLATE_IMG.md (2026-10-08)
Not triggered: AUTH (no token or credential is issued, verified or held; the mounted Secret holds probe targets, not credentials), PLATFORM (the Secret's custody and the cluster are audited with the workspace), PROXY (it forwards nothing), SUI_CLIENT and the chain lenses (no chain access).
**Go:** `go 1.26.6`, `toolchain go1.26.9` in `go.mod` (unchanged at 0.1.7); CI uses `go-version-file: go.mod`; the Dockerfile builder `golang:1.26.9-bookworm@sha256:d9c68c2c…453c` is checked against the `toolchain` line at build time
**Modules:** none (standard library only; no `go.sum`)
**Entry points:** `cmd/server` (one binary, listens on `:8080`, `-healthcheck` mode for the container `HEALTHCHECK`); routes `GET /api/status`, `OPTIONS /api/status`, `GET /healthz`
**Upstreams:** the internal health endpoints named in the mounted checks file (plain GET, no authentication, no credentials sent); none are named in the repository or the image
**Images:** `quay.io/meddleware-org/platform-probe:0.1.7@sha256:1ae74b672f1beed7a7e826250608bcb499da81cdc1c0f0ff6d5a94e9531c7e2a` (Docker Hub mirror, same digest; cosign keyless signature verified 2026-10-18 against the workflow-anchored identity `docker-publish.yml@refs/tags/v*`, `verify-digests.sh` 16/16; SPDX SBOM attestation, build provenance). Superseded 0.1.6 was `sha256:c85c5485…a2a7`
**Base images:** build `golang:1.26.9-bookworm@sha256:d9c68c2c…453c`; runtime `scratch` (the binary and the builder's CA bundle only)
**Runtime user:** `USER 65534:65534`   **Runtime FS:** read-only root; the only mount is the checks Secret, read-only
**Deployed by:** `k8s/base/apps/status/deployment.yaml` (namespace `apps`), overlay `k8s/clusters/meddleware-org/apps`; digest from `config/images.yaml`; probe targets from the SOPS-managed Secret `status-checks` (`status-checks-secret.enc.yaml`)
**Build args:** `VERSION` (link-time version), `TARGETOS`, `TARGETARCH`, and the image-label strings `VENDOR`, `DESCRIPTION`, `SOURCE_URL`, `DOCUMENTATION_URL`, `IMAGE_URL` — none secret
**Deployment status:** image `quay.io/meddleware-org/platform-probe` at 0.1.7 (`sha256:1ae74b67…7e2a`; release tag `v0.1.7` = `2f1ef48`; released, signature verified and deployed 2026-10-18; the rollout succeeded; 0.1.6 `sha256:c85c5485…` was deployed 2026-10-09) in `k8s/base/apps/status` (namespace `apps`); live `https://status.meddleware.co.uk/api/status` returns the topology-free snapshot (three groups including *Chain Access → Chain index*) with the security headers; after the 0.1.7 rollout the live snapshot read `overall: operational` with every component `operational` (re-read 2026-10-18), so the stricter 2xx default (F10) changed no component's state
**Review date:** 2026-09-18 (first pass) · re-verified 2026-10-03 · re-verified 2026-10-09 · fix wave 2026-10-18 (0.1.7, commit `bb33f0b`) · image build 2026-10-20 (0.1.8)
**Reviewer:** Internal review
**Severity ceiling:** Low — public but topology-free: it probes internal endpoints on a timer and serves only names and coarse statuses.
**Status:** fix wave 2026-10-18 (0.1.7, released and deployed 2026-10-18; signature verified; rollout succeeded)

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
- Defects found by this pass: **F10 (Low)**, **F11 (Info)** and **F14 (Info)** were DEFERRED to the next patch and are
  RESOLVED in 0.1.7 (2026-10-18, commit `bb33f0b`): a component without `expect_status` is now `operational` only
  for a 2xx (it read `operational` for any HTTP response, including a 5xx); the stale statements about where the
  probe targets live are corrected; the redirect refusal and the status handler are tested.
- Accepted or maintainer items: **F12** the README's cosign command pinned the repository, not the workflow
  (RESOLVED: the README has pinned `docker-publish.yml@refs/tags/v*` since 0.1.7, and the workspace's `verify-digests.sh`
  default became workflow-anchored on 2026-10-18); **F13** the registry mirror and credential inventory (DEFERRED, maintainer).

Fix wave 2026-10-18 (0.1.7: `go vet`, `go test -race` green, coverage checks 93.0%, config 100%, `cmd/server`
33.8%; golangci-lint v2.13.0 and govulncheck v1.8.0 clean on the pinned 1.26.9 toolchain; image built with
rootless podman and smoke-tested).

Image build 2026-10-20 (0.1.8): the release no longer emulates arm64 (the builder cross-compiles on the build host; no QEMU step). The pass found **F15 (Low, RESOLVED)**: the published `linux/arm64` image carried the amd64 binary (a `TARGETARCH=amd64` default overrode the platform value); the cluster is amd64, so nothing deployed was affected.

The severity ceiling stays Low. The live Secret is SOPS-encrypted and was not decrypted. The committed bootstrap
definitions (`bootstrap/profiles/single-node.sh`, `recover-secrets.sh`) and `OPERATOR_TASKS.md` give every one of the
five live components `expect_status: 200`, and the live snapshot read all `operational` on 2026-10-18, so the
2xx default of F10 was expected to change nothing on the page. **Verified after the 0.1.7 rollout (2026-10-18):** the status page API reads `overall: operational` with every component (Sign-in, Authorization, Registry API, Registry UI, Chain index) `operational`, so the stricter default (a check without `expect_status` is operational only for a 2xx) caused no component to change state.

## Threat model / trust boundaries

| Actor | Holds / proves | Can do | Bounded by |
| --- | --- | --- | --- |
| Anonymous client | requests | read the snapshot; CORS `*` | names and statuses only (I1); cached (I2); GET/OPTIONS only (I7) |
| Anonymous client (volume) | requests | try to amplify probes | probes run on the ticker only |
| Image or repo reader | the image | look for targets | targets live only in the mounted Secret |
| Config editor (maintainer) | the Secret | break the config; point probes at any URL; omit `expect_status` | last good config kept; failures read `down`; an omitted `expect_status` accepts only a 2xx and one outside 100..599 is rejected (F10); access is SOPS custody (workspace) |
| Probed service | its response | answer slowly, with a redirect, with a huge body | per-probe timeout, redirects not followed, 64 KiB body cap (I5) |
| Go toolchain, base image, CI publish job | the compiled code and the signed image | ship altered bytes | digest-pinned builder checked against the `go.mod` toolchain, pinned `govulncheck`, Trivy before cosign, keyless signature (F8, F9) |

## Severity scale

Critical / High / Medium / Low / Info / Positive.

## Scope

- **In scope (0.1.7):** `cmd/server/**`, `internal/checks/**`, `internal/config/**`, tests, `Dockerfile`,
  `.dockerignore`, workflows, `.github/dependabot.yml`, `SECURITY.md`, `README.md`, `llms.txt`, `AGENTS.md`, `CLAUDE.md`,
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
**Remediation / evidence (2026-10-18):** `NewProber` sets `CheckRedirect` to `http.ErrUseLastResponse`. Pinned
since 0.1.7 by `TestProbeRefusesRedirects`, which probes through `NewProber`'s client: a 302 reads `down` with and
without `expect_status`, the redirect target is never requested, and an explicit `expect_status: 302` is
`operational` (F14). A check without `expect_status` also reads `down` for a 3xx (F10).

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

**Severity:** Low   **Disposition:** RESOLVED (0.1.7, commit `bb33f0b`; Section D pre-testnet "any failure reads `down`")
**Where:** `internal/checks/checks.go` (`probe`, `statusAccepted`), `internal/checks/config.go` (`LoadFile`)
**Issue:** `expect_status` is optional in the JSON and defaults to `0`, which skipped the status comparison.
A component that omitted it read `operational` for any HTTP answer, including `500` and `503`, as long as
the connection succeeded (and, when `expect_body_contains` was also omitted, regardless of the body). The
README and `config.example.json` always show `expect_status`, but nothing rejected an entry without it. This
contradicted the invariants in `SECURITY.md` (3, "probe errors never render as operational") and `CLAUDE.md` (5).
**Impact:** a typo or omission in the Secret turned a health probe into a reachability probe on the public
status page, and an outage could show green. The maintainer edits the Secret, so this was a footgun, not an attack.
**Remediation / evidence:** the one default that keeps existing configs working and fails closed: with no
`expect_status` (`0`) only a 2xx is `operational` (`statusAccepted`); a 3xx (redirects are not followed, F3), 4xx
or 5xx reads `down`; an explicit `expect_status` stays an exact match. `LoadFile` rejects an `expect_status`
outside 100..599, keeping the last good config. Pinned by `TestProbeWithoutExpectStatusAcceptsOnly2xx` (200, 204,
299 operational; 301, 302, 400, 401, 404, 500, 503 down), `TestProbeExplicitExpectStatusIsExact`,
`TestLoadFileErrors` (99, 600, -1) and `TestLoadFileExpectStatus`; end to end in
`TestHandleStatusServesTheCachedSnapshot` (a 503 without `expect_status` reads `down`) and the 2026-10-18 image smoke
test. Rollout check: the live Secret is SOPS-encrypted and was not read; the committed bootstrap definitions
(`single-node.sh`, `recover-secrets.sh`) and `OPERATOR_TASKS.md` give all five live components
(Sign-in, Authorization, Registry API, Registry UI, Chain index) `expect_status: 200`, and the live page read
all `operational` on 2026-10-18, so no component was expected to change. A stricter rule (reject an entry without
`expect_status`) was not chosen: it would fail the whole reload, and at start-up leave the page empty, for one
omitted field. After the 0.1.7 rollout the page was confirmed unchanged (2026-10-18: all components operational), so no Secret change was needed; a component that read `down` would have omitted
`expect_status` and answered a non-2xx, and needed the explicit code added to the Secret.

### F11 — Stale statements about where the probe targets live

**Severity:** Info   **Disposition:** RESOLVED (0.1.7, commit `bb33f0b`; documentation)
**Where:** `internal/checks/checks.go` (package comment: "The internal probe URLs live here in code"),
`llms.txt` ("Checks are defined in code (internal/checks/checks.go)" and "`GET /` — the embedded status
page"), `README.md` ("endpoints stay in the binary"), `CLAUDE.md` (the status-page path
`workspace/apps/status-page/`); the workspace's `k8s/clusters/meddleware-org/apps/kustomization.yaml` header
still describes an embedded page (outside this repository)
**Issue:** the code loads targets only from `CHECKS_CONFIG_FILE` and serves no page; these texts said the
opposite. It is the same defect class as F1.
**Impact:** none on behaviour; a reader (or a model reading `llms.txt`) was misled about the central
invariant, and a contributor could reintroduce compiled-in targets in good faith.
**Remediation / evidence:** the `checks.go` package comment, `llms.txt` (no `GET /`, the `CHECKS_CONFIG_FILE`
variable and the runtime-loaded checks), the README (targets live in the mounted config) and `CLAUDE.md` (the
`repos/status-page` repository) are corrected in 0.1.7; `AGENTS.md` and `CHANGELOG.md` entries added. Not
fixed here: the header comment of the workspace's `k8s/clusters/meddleware-org/apps/kustomization.yaml` ("served at
GET / and GET /api/status. The page HTML is embedded in the binary, so there is no ConfigMap") is outside this
repository and still says so; the workspace should reword it (the status page is the separate `status-page` image).

### F12 — The published cosign command pins the repository, not the workflow

**Severity:** Info   **Disposition:** RESOLVED (README command 0.1.7, commit `bb33f0b`; workspace `verify-digests.sh` default 2026-10-18; was MITIGATED, before that ACCEPTED-RISK)
**Where:** `README.md` ("Verifying published images"); `bootstrap/images/verify-digests.sh` (workspace)
**Issue:** the command and the cluster check accepted any workflow identity of the repository. IMG lens:
anchor to the publish workflow and tag refs.
**Impact:** a workflow added by someone with write access could sign an image the check would accept.
**Remediation / evidence:** the README's `--certificate-identity-regexp` is now
`^https://github\.com/meddleware-org/platform-probe/\.github/workflows/docker-publish\.yml@refs/tags/v.*$`
(signing happens in `docker-publish.yml`, which runs on `v*` tags; re-read in `README.md` on 2026-10-18). The workspace script
`verify-digests.sh` was the remaining repository-wide part; on 2026-10-18 its default identity became
`^https://github\.com/meddleware-org/${name}/\.github/workflows/docker-publish\.yml@refs/tags/v` (`COSIGN_IDENTITY_REGEXP` still overrides it).
All 16 deployed and contingency images verified valid against it on 2026-10-18 (`STRICT=0 bash bootstrap/images/verify-digests.sh`,
0.1.7 included); 2026-10-09 (0.1.6) was checked with the earlier repository-wide identity. Nothing remains.

### F13 — Self-hosted registry mirror and registry credentials

**Severity:** Info   **Disposition:** DEFERRED (maintainer; `OPERATOR_TASKS.md` "Image registry credentials — record scope and rotation")
**Where:** `docker-publish.yml` `mirror-private` job (`continue-on-error: true`); `QUAY_TOKEN`, `DOCKERHUB_TOKEN`, `PRIVATE_REGISTRY_*`
**Issue:** the `mirror-private` job (a digest-preserving copy of the published, signed image, not a rebuild) fails without registry credentials; the registry tokens are
long-lived and not yet inventoried.
**Impact:** the mirror may lag; a leaked token could push an unsigned tag (the cluster pins digests and
verifies signatures, so it would not run).
**Remediation / evidence:** the mirror is listed as best-effort; the public job has no `continue-on-error`.
The credential inventory (scope, holder, expiry, rotation) is the maintainer item.

### F14 — Coverage gaps: redirect policy and the status handler

**Severity:** Info   **Disposition:** RESOLVED (0.1.7, commit `bb33f0b`; tests only)
**Where:** `internal/checks/checks_test.go` (`TestProbe`), `cmd/server/main_test.go`
**Issue:** `TestProbe` constructs `&Prober{client: &http.Client{}}`, bypassing the `CheckRedirect` set in
`NewProber`, so the F3 fix was unpinned; `handleStatus` (the JSON body, `Content-Type`, `Cache-Control:
no-store`) was exercised only through the header tests with a stub handler; `cmd/server` was at 19.6%.
**Impact:** a refactor could re-enable redirects or change the response shape without a test failing.
**Remediation / evidence:** `TestProbeRefusesRedirects` (through `NewProber`'s client, with a counter on the redirect
target) and `TestHandleStatusServesTheCachedSnapshot` (the real prober behind `newHandler(handleStatus(...))`: the
seeded `unknown` body, then the probed body, `Content-Type: application/json`, `Cache-Control: no-store`, the
security headers, only the fields `generated_at`, `overall` and `groups`, no host, port or path of the target, and no
extra probe from reading the status). Coverage at 0.1.7: checks 93.0%, config 100%, `cmd/server` 33.8% (the rest is
`main`'s wiring and shutdown).

### F15 — The published `linux/arm64` image carried an x86-64 binary (found 2026-10-20)

**Severity:** Low   **Disposition:** RESOLVED (0.1.8, commit pending)
**Where:** `Dockerfile` (`ARG TARGETARCH=amd64`, the `go build` line); `.github/workflows/docker-publish.yml` (QEMU)
**Issue:** the builder declared `ARG TARGETOS=linux` and `ARG TARGETARCH=amd64` and compiled with
`GOARCH=${TARGETARCH}`. The default overrode the platform value BuildKit would have supplied, so the arm64 half of the
multi-arch release (built under QEMU) compiled `GOARCH=amd64` too. Found 2026-10-20 while moving the build off QEMU:
the `linux/arm64` image of the published 0.1.7 (`sha256:1ae74b67…7e2a`) holds `/platform-probe` with the same SHA-256 as the
amd64 image (`96bf801a…`, ELF x86-64).
**Impact:** the `linux/arm64` image would fail with an exec format error on an arm64 node. The cluster is amd64, so
nothing deployed was affected; the signature, SBOM and scan all verified, because none of them runs the binary.
**Remediation / evidence:** 0.1.8: the builder is `FROM --platform=$BUILDPLATFORM` and cross-compiles; `TARGETOS` and
`TARGETARCH` have no default and the `RUN` aborts if either is empty; a build step asserts with `go version -m`
(reads the binary without running it) that its `GOARCH` equals `${TARGETARCH}`, so a mismatched architecture fails the
build on every platform (checked by hard-coding `GOARCH=amd64` and building `linux/arm64`: `is not built for arm64`);
`setup-qemu-action` is removed from the release. Proof (rootless podman, no binfmt handler registered, so emulation was
impossible): the amd64 image built from the 0.1.7 sources with `VERSION=v0.1.7` is byte-identical to the published
amd64 image (every file); the arm64 image holds an ELF aarch64 binary (`2f534b11…`; `go version -m`: `GOARCH=arm64`,
`GOOS=linux`, `CGO_ENABLED=0`, `-trimpath=true`) and the rest of its tree equals the published one.

## Section A — Invariant verification matrix

| # | Invariant | Enforced at | Proven by | Status |
| --- | --- | --- | --- | --- |
| I1 | Output is names and statuses only | `checks.go` snapshot types | `TestSnapshotJSONHasNoURLs`; live body read 2026-10-09 | HOLDS |
| I2 | Probes run only on the server's ticker | `Prober.Start`; handlers read the cached snapshot | source | HOLDS |
| I3 | A probe error, timeout or unexpected status reads `down` | `probe`, `statusAccepted` | `TestProbe` (502, body mismatch, unreachable), `TestProbeWithoutExpectStatusAcceptsOnly2xx` | HOLDS (F10) |
| I4 | A bad config keeps the last good one | `reload` | `TestReloadKeepsLastGoodOnBadEdit`, `TestRunOnceReloadsConfigDynamically` | HOLDS |
| I5 | Every probe and body is bounded | per-probe context timeout; 64 KiB cap | `TestProbe` | HOLDS (F4) |
| I6 | A cycle never outlasts its interval | `config.Load` | `TestLoadRejectsBadDurations` | HOLDS (F6) |
| I7 | Only GET (and HEAD) and OPTIONS on two routes; every response carries the security headers | `ServeMux` method patterns; `withSecurityHeaders` | `TestHandlerSecurityHeaders` (incl. 404, 405) | HOLDS (F7) |
| I8 | A health check asserts a real condition | an omitted `expect_status` accepts only a 2xx; a value outside 100..599 is rejected | `TestProbeWithoutExpectStatusAcceptsOnly2xx`, `TestLoadFileErrors` | HOLDS (F10) |
| I9 | Redirects are not followed | `NewProber` `CheckRedirect` | `TestProbeRefusesRedirects` | HOLDS (F3, F14) |

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
| IMG | Build host and emulation | HOLDS (0.1.8, F15) — the builder stage is `FROM --platform=$BUILDPLATFORM` and cross-compiles with `GOOS=$TARGETOS GOARCH=$TARGETARCH` (no default for either; the `RUN` aborts if one is empty; a `go version -m` step asserts the binary's `GOARCH` equals the target); `docker-publish.yml` has no `docker/setup-qemu-action` step, so no privileged binfmt step runs in the release; rootless podman build of both platforms with no binfmt handler: the amd64 image equals the published 0.1.7 in every file, the arm64 image holds an ELF aarch64 binary |
| IMG | Runtime user & filesystem | HOLDS — `USER 65534:65534`; the pod sets `runAsNonRoot`, read-only root, no privilege escalation, all capabilities dropped, `RuntimeDefault` seccomp, `automountServiceAccountToken: false`, probes on `/healthz`, requests 5m/16Mi and limits 100m/48Mi; ingress is default-deny with allowances for nginx-ingress and monitoring; no egress policy (the probe must reach other namespaces, whose own policies admit it) |
| IMG | Scan, SBOM and notices | HOLDS (F9) — Trivy on the published digest before cosign; SBOM attestation from the image; no third-party code is redistributed (standard library compiled in, as in static-server; the builder's CA bundle is the only copied file) |
| IMG | Verification command | HOLDS — the README pins `docker-publish.yml@refs/tags/v*` and the workspace cluster check pins the same workflow and tag refs since 2026-10-18 (F12) |
| IMG | Deployment pinning | HOLDS — digest `sha256:1ae74b67…7e2a` (0.1.7, rolled out 2026-10-18; was `sha256:c85c5485…` at 0.1.6) identical in `config/images.yaml` and the cluster kustomization; the base manifest's tag label (`0.1.2`) is overridden by the overlay digest and is cosmetic |

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

### C.1 Coverage grade — A− (checks 93.0%, config 100%, `cmd/server` 33.8%, 2026-10-18)

Tests: rollup and aggregation order, the snapshot JSON has no URLs, `probe` against OK/502/body-match/
unreachable servers, config load and reload (valid, errors, missing file, last-good kept), duration
validation, the handler's security headers and CORS, the redirect refusal through `NewProber`'s client, the
default-2xx rule and the `expect_status` range (F10, F3), and the status handler's body, content type, caching
and topology-freeness (F14). Not covered: `main` wiring, signal handling and `-healthcheck`. `go test -race` is
part of CI and of the release gate (F9).

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
- [x] any failure reads `down`, including a check without `expect_status` — F10 (0.1.7: only a 2xx passes without `expect_status`; `TestProbeWithoutExpectStatusAcceptsOnly2xx`; commit `bb33f0b`)
- [x] documentation matches the invariant that targets are not compiled in — F11 (0.1.7: package comment, `llms.txt`, README, `CLAUDE.md` corrected; the workspace kustomization header is outside the repo; commit `bb33f0b`)

### pre-mainnet

- [x] no additional gate (testnet-independent service)
- [x] each published platform's image holds that platform's binary, and the release builds without emulation — F15 (0.1.8: `FROM --platform=$BUILDPLATFORM`, no `TARGETARCH` default, the `go version -m` build step; commit pending)
- [ ] registry credential inventory and rotation (F13) — `OPERATOR_TASKS.md` "Image registry credentials"

## Cross-project themes

- **Topology hiding** — public status without internal names or URLs.
- **Supply chain & release integrity** — no dependencies; Go toolchain pinned and checked against the
  builder; pinned scanners; Trivy before signing; signed images with SBOM and provenance.
- **Deployment readiness** — Section D.

## Normative requirements (MUST / MUST NOT)

- **GO-M1–GO-M8** — hold (GO-M3 path confinement, GO-M4 token verification and GO-M6 rate limits N/A: no
  paths, no tokens, cached read-only endpoint; GO-M1 holds with the default header limit).
- **IMG-M1–IMG-M8** — hold; IMG-M8's published verification command pins the publish workflow and tag refs (F12), and since 2026-10-18 so does the workspace cluster check.

## Implementation suggestions (SHOULD / MAY)

- SHOULD reword the header comment of the workspace's `k8s/clusters/meddleware-org/apps/kustomization.yaml`
  (it still describes an embedded status page; F11, outside this repository).
- ~~SHOULD anchor the workspace's `verify-digests.sh` identity to `docker-publish.yml@refs/tags/v*` (F12).~~ Done 2026-10-18.
- MAY set `MaxHeaderBytes` explicitly (a few KiB) since no handler needs more.

## Open questions (`OQ#`)

- **OQ1** — Is the Secret directory read-only to UID 65534? (Decided 2026-10-03: yes — `readOnly:
  true`, `defaultMode: 0440`, `fsGroup: 65534`. Re-read 2026-10-09 in `deployment.yaml`: unchanged.)

## Risks

- **Probe-target drift** — targets are maintained by hand in the Secret.
- **Rollout of the 2xx default (F10)** — if the live Secret has a component without `expect_status` that answers a non-2xx, it would read `down` after 0.1.7 is deployed; the maintainer would then add the explicit code. The committed definitions and `OPERATOR_TASKS.md` give all five components `expect_status: 200`. **Checked after the rollout (2026-10-18):** all components `operational`; the risk did not materialise.
- **Stale snapshot** — the page shows `generated_at`; if the prober loop stopped, the cached snapshot would
  stay (the process exits on a failed listener, not on a stalled loop).

## Re-verification log

- 2026-09-18 — first-pass baseline (F1–F5).
- 2026-10-01 — 0.1.4: security headers on every response.
- 2026-10-03 — re-verified under AUDIT_TEMPLATE.md + GO + IMG (Phase 7): F6 RESOLVED (config tests);
  govulncheck pinned; OQ1 decided.
- 2026-10-08 — Lens dates reconciled with the registry (`check-template-dates.mjs`): base 2026-10-08, and SUI_CLIENT/GO 2026-10-08 and TS 2026-10-03 where cited. The changes (AUTH/PLATFORM/MCP/DB registered, the GO token row moved to AUTH, JSR in trusted publishing, layered injection guards) alter no disposition here.
- 2026-10-09 — re-verified at 0.1.6 (`48590c2`) against the code, the workflows, the manifests and the live endpoint. IMG date now 2026-10-08; front matter gains the Go and IMG fields and the deployment status (0.1.6, digest `sha256:c85c5485…`). F1–F6 re-checked (F3 evidence: the redirect policy is unpinned by a test; F4 body read only when a substring is expected). New: F7 RESOLVED (security headers, 0.1.4), F8 RESOLVED (Go 1.26.9 and the toolchain check, 0.1.5), F9 RESOLVED (release runs go-ci, Trivy before cosign, 0.1.6; the web-repo test gap is absent), F10 DEFERRED (omitted `expect_status` reads operational), F11 DEFERRED (stale texts), F14 DEFERRED (coverage), F12 ACCEPTED-RISK, F13 DEFERRED (maintainer). Counts: vet and `go test -race` green. The live endpoint now includes *Chain Access → Chain index*; the SOPS Secret was not decrypted. Section D gates ticked with evidence; unticked: two next-patch items (F10, F11) and the maintainer item F13.
- 2026-10-18 — fix wave at 0.1.7 (commit `bb33f0b`). F10 RESOLVED (a check without `expect_status` is operational only for a 2xx; `expect_status` outside 100..599 rejected; the live components were checked against the committed definitions, the live page and the public status API, not the encrypted Secret), F11 RESOLVED (four texts corrected; the workspace kustomization header is outside the repo and still stale), F14 RESOLVED (redirect and status-handler tests; coverage checks 93.0%, `cmd/server` 33.8%), F12 MITIGATED (README identity pinned to `docker-publish.yml@refs/tags/v*`; workspace script unchanged), F3 evidence updated (now pinned by a test). F13 stays DEFERRED (maintainer). Checks: `go vet`, `go test -race`, golangci-lint v2.13.0, govulncheck v1.8.0 on go1.26.9, a rootless podman image build and run with a local target. Section D: F10 and F11 ticked; F13 remains. Counts: Section D has one unticked gate (F13).
- 2026-10-18 — released and deployed (`v0.1.7` = `2f1ef48`, `0.1.7@sha256:1ae74b672f1beed7a7e826250608bcb499da81cdc1c0f0ff6d5a94e9531c7e2a`), verified live. Cosign keyless signature verified against the workflow-anchored identity (`docker-publish.yml@refs/tags/v*`, `verify-digests.sh` 16/16); rollout via `kubectl apply -k` (apps overlay) succeeded. The stricter default (F10: a check without `expect_status` is operational only for a 2xx) caused no component to change state: the live status API (`/api/status`, re-read 2026-10-18) reads `overall: operational` with all five components operational. F13 (maintainer) unchanged.
- 2026-10-18 — F12 (cosign identity) RESOLVED: the default identity in `bootstrap/images/verify-digests.sh` is now workflow-anchored (`^https://github\.com/meddleware-org/${name}/\.github/workflows/docker-publish\.yml@refs/tags/v`); all 16 deployed and contingency images verified valid against it (`STRICT=0 bash bootstrap/images/verify-digests.sh`), and the README `cosign verify` command is workflow-pinned (checked). F12 moves from MITIGATED to RESOLVED; nothing remains.
- 2026-10-19 — dependency-bot status checked (read-only): `.github/dependabot.yml` carries the `ignore` rules and the repository has no open Dependabot pull requests (`gh pr list`, 2026-10-19). Versions, findings and gates were not re-examined in this pass.
  - Dependabot policy encoded in .github/dependabot.yml (majors declined); open PRs merged 2026-10-19
  - `docker-publish.yml`: the private rebuild jobs are replaced by one `mirror-private` job that copies the published quay.io image (and its cosign artifacts) to the self-hosted registry by digest and fails unless the digest and the signature match (build once, promote by digest; committed locally, not yet released; the `PRIVATE_REGISTRY_*` credentials are still unset).
- 2026-10-20 — image build moved off QEMU in 0.1.8 (commit pending): the builder stage is `FROM --platform=$BUILDPLATFORM` and cross-compiles with `GOOS=$TARGETOS GOARCH=$TARGETARCH` (no default for either; the `RUN` aborts if one is empty; a `go version -m` step asserts the binary's `GOARCH` equals the target); `docker-publish.yml` has no `docker/setup-qemu-action` step, so no privileged binfmt step runs in the release. Proof with rootless podman (builder pinned by digest, no binfmt handler registered): the amd64 image built from the 0.1.7 sources equals the published 0.1.7 in every file; the arm64 image differs by design (F15). New: F15 RESOLVED (the published arm64 image held an x86-64 binary; the cluster is amd64, so nothing deployed was affected). Checks: `go vet`, `go test -race` on go1.26.9, `scripts/check-image-notices.sh` with podman where the repo has it. No other disposition changed.
