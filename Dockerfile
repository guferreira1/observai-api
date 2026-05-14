# syntax=docker/dockerfile:1.7
# Multi-stage build for ObservAI API.
#
# Stage 1 compiles a static binary with the Go toolchain.
# Stage 2 ships a minimal runtime image with CA certificates and the binary.

ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.20

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

ARG VERSION=dev

WORKDIR /src

RUN apk add --no-cache git ca-certificates

# Cache modules separately so source-only changes do not invalidate the layer.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ENV CGO_ENABLED=0 \
    GOOS=linux

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/observai-api \
        ./cmd/observai-api

FROM alpine:${ALPINE_VERSION} AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S observai \
    && adduser -S observai -G observai \
    && mkdir -p /etc/observai /app/agents /app/migrations \
    && chown -R observai:observai /etc/observai /app

WORKDIR /app

COPY --from=builder /out/observai-api /usr/local/bin/observai-api
COPY --chown=observai:observai agents/ /app/agents/
COPY --chown=observai:observai migrations/ /app/migrations/

USER observai

ENV OBSERVAI_API_PORT=8080 \
    OBSERVAI_PROMPTS_DIR=/app/agents \
    OBSERVAI_MIGRATIONS_DIR=/app/migrations

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/observai-api"]
