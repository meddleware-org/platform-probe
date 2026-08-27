# Security Policy

## Scope

This policy covers security issues in:

- The Go binary (`cmd/server`, `internal/*`) — information disclosure,
  request handling, or similar
- The container image build (`Dockerfile`)
- The published images at `quay.io/meddleware-org/platform-probe` and
  `docker.io/meddleware/platform-probe`

It does not cover the services being probed (report those to their own projects),
the Go standard library (report to the [Go security team](https://go.dev/security)),
or operator misconfiguration of the deployment or network policies.

## Security model (invariants)

The status page is **public**, so its defining property is that it discloses only
capability-level health, never internal topology. A report demonstrating any of the
following is in scope and treated as high severity:

1. **No topology disclosure.** `GET /api/status` (and the page) must expose only
   capability groups, component names, and coarse statuses. Internal probe URLs,
   hostnames, ports, tech-stack fingerprints, and error text must never appear in
   any response. Probe targets exist only in the compiled binary.
2. **Server-side, cached probing.** Clients cannot trigger a probe; they read a
   cached snapshot. Probe traffic to internal services is fixed at one cycle per
   `PROBE_INTERVAL` and cannot be amplified by request volume.
3. **Fail closed to `down`.** Probe errors never render as `operational` and never
   crash the service.
4. **Read-only surface.** The service accepts only simple GETs and takes no client
   input that influences probing.

## Supported versions

Only the latest published image tag receives security fixes.

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Report by emailing **<security@meddleware.co.uk>** with a description, impact, and
reproduction steps (or a proof-of-concept), and the image tag or commit SHA tested.
You will receive an acknowledgement within **3 business days** and a resolution plan
within **14 days** for confirmed issues. Critical issues (CVSS ≥ 9.0) are prioritised
for same-day acknowledgement.

## Disclosure

Once a fix is released, a security advisory will be published on the GitHub
repository. Reporters may be credited by name unless they prefer to remain anonymous.
