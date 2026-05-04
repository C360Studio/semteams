# SemTeams Builder Sandbox — toolchain image (R3.6.1.c, ADR-032 §8).
#
# Provides Java (Maven + Gradle), Go, Node.js, Python, protoc + Go
# protobuf plugin. The image is the *environment*; the LLM produces
# every artifact (pom.xml, OSGi metadata, sources, tests). No
# scaffolding is shipped per ADR-032 Decision #17 (bare seed).
ARG GO_VERSION=1.25
ARG GO_FULL_VERSION=1.25.3

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

# Sandbox user owns /workspace and the cache mount points. Cache dirs
# are mount targets (compose mounts a shared named volume here r/w);
# pre-creating + chowning them so a fresh deploy works without manual
# fix-up if the volume is created empty.
RUN useradd -m -s /bin/bash -U sandbox \
    && mkdir -p /workspace /go/pkg/mod /home/sandbox/.m2 /home/sandbox/.npm /home/sandbox/.gradle \
    && chown -R sandbox:sandbox /workspace /go /home/sandbox/.m2 /home/sandbox/.npm /home/sandbox/.gradle

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
