# SemTeams Builder Sandbox — minimal image for R3.6.1.a (ADR-032).
# Toolchain (Java/Maven, Go, Node, Python, protoc) lands in R3.6.1.c.
# This image only contains the Go binary + alpine runtime so the
# workspace lifecycle endpoints can be exercised end-to-end before
# toolchain weight gets added.
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/semteams/sandbox/ cmd/semteams/sandbox/
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /sandbox ./cmd/semteams/sandbox/

FROM alpine:latest

RUN apk add --no-cache ca-certificates wget

# Non-root user owns the workspace volume.
RUN addgroup -S -g 1000 sandbox && \
    adduser -S -u 1000 -G sandbox -h /home/sandbox sandbox && \
    mkdir -p /workspace && \
    chown -R sandbox:sandbox /workspace

COPY --from=builder /sandbox /usr/local/bin/sandbox

USER sandbox
WORKDIR /workspace
EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8090/health || exit 1

ENTRYPOINT ["/usr/local/bin/sandbox"]
CMD ["--addr", ":8090", "--workspace", "/workspace"]
