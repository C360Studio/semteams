# SemTeams Builder Sandbox — toolchain image (R3.6.1.c, ADR-032 §8).
#
# Provides Java (Maven + Gradle), Go, Node.js, Python, protoc + Go
# protobuf plugin. The image is the *environment*; the LLM produces
# every artifact (pom.xml, OSGi metadata, sources, tests). No
# scaffolding is shipped per ADR-032 Decision #17 (bare seed).
ARG GO_VERSION=1.26
ARG GO_FULL_VERSION=1.26.3

# ============================================================================
# Builder stage — compiles the Go sandbox binary + the protoc-gen-go plugin.
# ============================================================================
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/semteams/sandbox/ cmd/semteams/sandbox/
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /sandbox ./cmd/semteams/sandbox/

# protoc-gen-go (Go protobuf plugin). Pinned to v1.34.2.
RUN GOBIN=/out go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2

# ============================================================================
# Runtime stage — Ubuntu 24.04 with the full toolchain.
# ============================================================================
FROM ubuntu:24.04

ARG GO_FULL_VERSION
ENV DEBIAN_FRONTEND=noninteractive

# System packages, Java 21 + Maven, Python 3, protoc, healthcheck deps.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential git curl wget jq unzip ca-certificates \
    openjdk-21-jdk-headless maven \
    python3 python3-pip python3-venv \
    protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

# Stable JAVA_HOME symlink that doesn't depend on architecture suffix
# (Debian's openjdk-21-jdk-headless installs to .../java-21-openjdk-{amd64,arm64}).
# execCommand sets JAVA_HOME=/usr/lib/jvm/java-21-openjdk so mvn/gradle work
# on both arches without arch-specific Go code.
RUN JDK_PATH="$(dirname "$(dirname "$(readlink -f "$(which javac)")")")" && \
    ln -sfn "$JDK_PATH" /usr/lib/jvm/java-21-openjdk

# Go 1.25 — matches semteams's go.mod toolchain pin.
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/go${GO_FULL_VERSION}.linux-${ARCH}.tar.gz" | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}" \
    GOPATH=/go

# Gradle 8.14.
RUN curl -fsSL https://services.gradle.org/distributions/gradle-8.14-bin.zip -o /tmp/gradle.zip \
    && unzip -q /tmp/gradle.zip -d /opt \
    && rm /tmp/gradle.zip \
    && ln -s /opt/gradle-8.14/bin/gradle /usr/local/bin/gradle
ENV GRADLE_HOME=/opt/gradle-8.14

# Node.js 22 LTS.
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# @devcontainers/cli — sandbox manager (ADR-043 Layer 2) shells out
# to `devcontainer up`/`devcontainer exec` to materialize per-profile
# environments. Pinned for reproducibility per
# ADR-043 §Risks #3 ("@devcontainers/cli velocity"); operators bump
# in lockstep with the canonical-profile catalog in
# .devcontainer/<name>/devcontainer.json and the catalog `Version`
# field in cmd/semteams/sandboxmanager/catalog_builtin.go.
#
# Installed globally so the CLI resolves on $PATH for both the
# sandbox process (running probes) and the chain-scoped bash tool
# (running coordinator-emitted commands). npm cache pre-warmed in
# the build step; runtime install would race against concurrent
# sandbox invocations.
ARG DEVCONTAINERS_CLI_VERSION=0.87.0
RUN npm install -g @devcontainers/cli@${DEVCONTAINERS_CLI_VERSION} \
    && npm cache clean --force

# Docker CLI (client only — daemon stays on the host).
# When the sandbox runs with --docker-mode=dood (ADR-034 §addendum #2),
# the host's /var/run/docker.sock is bind-mounted and chains use this
# `docker` binary (and Testcontainers libs that shell out to it) to
# spawn sibling containers on the host's daemon.
#
# Static binary from Docker's official mirror keeps the image lean
# vs. apt-installing docker.io (which pulls daemon + containerd we
# never run). Pinned to a stable channel; bump on Docker Engine
# major releases when the wire protocol changes. Both amd64 and arm64
# tarballs are published from the same path.
ARG DOCKER_CLI_VERSION=27.5.1
RUN ARCH=$(dpkg --print-architecture) \
    && case "$ARCH" in \
        amd64) DOCKER_ARCH=x86_64 ;; \
        arm64) DOCKER_ARCH=aarch64 ;; \
        *) echo "unsupported arch: $ARCH"; exit 1 ;; \
    esac \
    && curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_CLI_VERSION}.tgz" -o /tmp/docker.tgz \
    && tar -xzf /tmp/docker.tgz -C /tmp docker/docker \
    && install -o root -g root -m 0755 /tmp/docker/docker /usr/local/bin/docker \
    && rm -rf /tmp/docker /tmp/docker.tgz

# Docker Buildx plugin. devcontainer-cli shells `docker buildx version`
# early in `devcontainer up` and bails if missing — required for any
# profile that has `features:` (which is all of ours; features get
# layered onto the base image via a build step). Without buildx, the
# first-time provisioning of any profile fails before the container
# starts; smoke #autoresearch-run2 (2026-06-02) surfaced this when the
# svelte-ui@v1 profile was tried for the first time on a fresh sandbox
# image. Pre-cached images skipped the build and masked the issue, so
# it only fires on cache misses.
#
# Buildx is a Docker CLI plugin (separate binary at
# /usr/local/lib/docker/cli-plugins/), distributed via GitHub releases.
# Pinned; bump as needed.
ARG DOCKER_BUILDX_VERSION=v0.34.1
RUN ARCH=$(dpkg --print-architecture) \
    && mkdir -p /usr/local/lib/docker/cli-plugins \
    && curl -fsSL "https://github.com/docker/buildx/releases/download/${DOCKER_BUILDX_VERSION}/buildx-${DOCKER_BUILDX_VERSION}.linux-${ARCH}" \
        -o /usr/local/lib/docker/cli-plugins/docker-buildx \
    && chmod 0755 /usr/local/lib/docker/cli-plugins/docker-buildx

# Sandbox user owns /workspace, /sandbox-cwds, and the cache mount points.
# These are mount targets (compose mounts shared named volumes / a host
# bind here r/w); pre-creating + chowning them so a fresh deploy works
# without manual fix-up if the volume is created empty. Docker initializes
# named-volume mount points to root:root by default when the target dir
# doesn't exist in the image — pre-creating with the right ownership is
# the standard workaround. /sandbox-cwds is the ADR-043 PR 4.5 review C2
# sandbox cwd root (named volume sandbox-agentic-workspaces; isolated from
# the SEMTEAMS_TENANT_ROOT host bind used by the sandbox-manager).
RUN useradd -m -s /bin/bash -U sandbox \
    && mkdir -p /workspace /sandbox-cwds /go/pkg/mod /home/sandbox/.m2 /home/sandbox/.npm /home/sandbox/.gradle \
    && chown -R sandbox:sandbox /workspace /sandbox-cwds /go /home/sandbox/.m2 /home/sandbox/.npm /home/sandbox/.gradle

# Binaries from the builder stage.
COPY --from=builder /sandbox /usr/local/bin/sandbox
COPY --from=builder /out/protoc-gen-go /usr/local/bin/protoc-gen-go

USER sandbox
WORKDIR /workspace
EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8090/health || exit 1

ENTRYPOINT ["/usr/local/bin/sandbox"]
CMD ["--addr", ":8090", "--workspace", "/workspace"]
