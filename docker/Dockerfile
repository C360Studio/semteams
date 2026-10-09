# SemTeams Production Dockerfile
#
# Multi-stage build for minimal, secure production images
# Supports multi-arch: linux/amd64, linux/arm64
#
# Build from repository root:
#   docker build -f docker/Dockerfile -t semteams:dev .
#
# Build with buildx for multi-arch:
#   docker buildx build -f docker/Dockerfile \
#     --platform linux/amd64,linux/arm64 -t semteams:dev .
#
# Tagged images are published only by .github/workflows/publish.yml, which
# passes VERSION, COMMIT_SHA and BUILD_DATE so `semteams --version` reports
# the tag.

# Build arguments
ARG GO_VERSION=1.26
ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_DATE=unknown

# ============================================================================
# Builder Stage
# ============================================================================
# The builder runs on the build platform and cross-compiles for the target
# (CGO is off), so multi-arch builds do not compile Go under emulation.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go module files
COPY go.mod go.sum ./

# Download dependencies (cached layer)
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked go mod download

# Copy source code
COPY . .

# Build arguments for version info
ARG VERSION
ARG COMMIT_SHA
ARG BUILD_DATE
ARG TARGETOS
ARG TARGETARCH

# Build main binary with version information
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT_SHA} \
      -X main.buildDate=${BUILD_DATE}" \
    -o /build/semteams-bin \
    ./cmd/semteams

# ============================================================================
# Production Stage
# ============================================================================
FROM alpine:latest AS production

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    wget

# Create non-root user and group
RUN addgroup -S -g 1000 semteams && \
    adduser -S -u 1000 -G semteams -h /app semteams

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder --chown=semteams:semteams /build/semteams-bin /app/semteams

# Copy example configs (optional, users should mount their own)
COPY --from=builder --chown=semteams:semteams /build/configs /app/configs

# ADR-043 PR 4.4 — copy the canonical .devcontainer/<profile>/
# devcontainer.json files into the production image. The
# sandboxmanager.BuiltinCatalog reads them at boot from
# SEMTEAMS_SANDBOX_REPO_ROOT (default /app), and SandboxAPIRunner
# passes the absolute paths to `devcontainer up --config <path>`
# invoked inside the sandbox container — which mounts /app/.devcontainer
# ro from this image via a shared compose volume (see
# ui/docker-compose.agentic-e2e.yml + production compose).
COPY --from=builder --chown=semteams:semteams /build/.devcontainer /app/.devcontainer

# Create config directory for mounted configs. The /etc/semstreams path and
# the SEMSTREAMS_* env prefix are a live contract with the compose files and
# the product shell (ADR-029); renaming them is a separate contract change.
RUN mkdir -p /etc/semstreams && \
    chown semteams:semteams /etc/semstreams

# Create data directory
RUN mkdir -p /data && \
    chown semteams:semteams /data

# Create the agent-emit artifacts directory tree owned by the runtime
# user. Docker semantics: when a named volume is mounted onto an empty
# directory in the image, Docker copies the directory's content + perms
# into the volume on first use — so this chown propagates to the
# `agentic-artifacts` named volume even though the chown happens at
# build time. emit-tools (research/plan/spec) write here at runtime;
# without this seeding the volume defaults to root:root and writes
# fail with EACCES.
RUN mkdir -p /artifacts/research /artifacts/plans /artifacts/specs && \
    chown -R semteams:semteams /artifacts

# Switch to non-root user
USER semteams

# Expose ports
# HTTP API, Metrics, and UDP input
EXPOSE 8080
EXPOSE 9090
EXPOSE 14550/udp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Set entrypoint
ENTRYPOINT ["/app/semteams"]

# Default command: use mounted config or fall back to example
# Users should mount their config at /etc/semstreams/config.json
CMD ["--config", "/etc/semstreams/config.json"]

# Labels
ARG VERSION
ARG COMMIT_SHA
ARG BUILD_DATE
LABEL org.opencontainers.image.title="SemTeams" \
      org.opencontainers.image.description="Always-on program manager for a configurable portfolio" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT_SHA}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/C360Studio/semteams" \
      org.opencontainers.image.vendor="C360" \
      org.opencontainers.image.licenses="MIT"
