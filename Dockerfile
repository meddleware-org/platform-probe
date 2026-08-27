# ── Build stage ───────────────────────────────────────────────────────────────
# Base pinned by digest for reproducible builds; the tag is kept for readability.
# To bump: docker buildx imagetools inspect golang:1.26-bookworm --format '{{.Manifest.Digest}}'
FROM golang:1.26-bookworm@sha256:6ef6e30f0ea5c384f6d111cf856e024e3086bbdcb1779da3f3b3fbba0aea53d2 AS builder

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

# Stdlib-only module: there is no go.sum and `go mod download` is a no-op, but
# copying go.mod first keeps the dependency layer cacheable if deps are ever added.
COPY go.mod ./
RUN go mod download

COPY . .

# Fully static binary: no libc, no CGO — runs in scratch.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/platform-probe \
    ./cmd/server

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
