FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod ./
COPY src/ ./src/
RUN CGO_ENABLED=0 go build -o /fixture-project ./src

FROM alpine:3.21
COPY --from=builder /fixture-project /usr/local/bin/fixture-project
ENTRYPOINT ["fixture-project"]
