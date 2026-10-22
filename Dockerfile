# ── Build stage ───────────────────────────────────────────────────────────────
# Base pinned by digest for reproducible builds; the tag is kept for readability.
# To bump: docker buildx imagetools inspect golang:<go.mod toolchain>-bookworm --format '{{.Manifest.Digest}}'
# The builder runs on the BUILD host's architecture and cross-compiles (pure Go, CGO off), so no arm64 step runs
# under emulation. Nothing the builder executes is target-specific.
FROM --platform=$BUILDPLATFORM golang:1.26.10-bookworm@sha256:632dff7351799a5a9954a48dc06670ade72ee44e8c351cdc0e5a57af2fea21af AS builder

ARG VERSION=dev
# Set by BuildKit/podman from --platform; a plain builder that leaves them empty fails the guard below.
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Stdlib-only module: there is no go.sum and `go mod download` is a no-op, but
# copying go.mod first keeps the dependency layer cacheable if deps are ever added.
COPY go.mod ./

# The binary is built with the toolchain go.mod names, which is the one CI's govulncheck checks: a builder
# image left on another patch fails here instead of shipping an unchecked stdlib.
RUN grep -q "^toolchain $(go env GOVERSION)$" go.mod || { echo "builder $(go env GOVERSION) != go.mod toolchain"; exit 1; }
RUN go mod download

COPY . .

# Fully static binary: no libc, no CGO — runs in scratch.
RUN : "${TARGETOS:?build with BuildKit or podman (sets TARGETOS)}" "${TARGETARCH:?build with BuildKit or podman (sets TARGETARCH)}" \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/platform-probe \
    ./cmd/server

# The compiled binary must be the target architecture (go version -m reads it without running it): an
# image of one architecture holding another's binary would pass every other check and fail only on a node.
RUN [ "$(go version -m /out/platform-probe | sed -n 's/^[[:space:]]*build[[:space:]]*GOARCH=//p')" = "${TARGETARCH}" ] \
    || { echo "/out/platform-probe is not built for ${TARGETARCH}"; exit 1; }

# ── Runtime stage ─────────────────────────────────────────────────────────────
# scratch: zero OS footprint. The binary is fully self-contained.
FROM scratch

ARG VERSION=dev
ARG VENDOR="Meddleware"
ARG DESCRIPTION="Server-side, cached, capability-grouped platform health probe for the Meddleware platform."
ARG SOURCE_URL="https://github.com/meddleware-org/platform-probe"
ARG DOCUMENTATION_URL="https://github.com/meddleware-org/platform-probe#readme"
ARG IMAGE_URL="https://quay.io/meddleware-org/platform-probe"

LABEL org.opencontainers.image.title="platform-probe" \
      org.opencontainers.image.description="${DESCRIPTION}" \
      org.opencontainers.image.url="${IMAGE_URL}" \
      org.opencontainers.image.source="${SOURCE_URL}" \
      org.opencontainers.image.documentation="${DOCUMENTATION_URL}" \
      org.opencontainers.image.vendor="${VENDOR}" \
      org.opencontainers.image.licenses="0BSD" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=builder /out/platform-probe /platform-probe
# CA bundle so the probe can dial HTTPS endpoints if ever configured; the
# in-cluster defaults (plain HTTP ClusterIP) do not require it.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Non-root nobody: the binary needs no filesystem writes and no privileges.
USER 65534:65534

EXPOSE 8080

ENV PORT=8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/platform-probe", "-healthcheck"]

ENTRYPOINT ["/platform-probe"]
