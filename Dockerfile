# syntax=docker/dockerfile:1.4

# ============================================================================
# Stage 1: Builder - Compile all Go binaries
# ============================================================================
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy dependency files first for better caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Install swag for Swagger generation
RUN --mount=type=cache,target=/go/pkg/mod \
    go install github.com/swaggo/swag/cmd/swag@latest

# Copy source code
COPY . .

# Generate Swagger documentation
RUN swag init -g cmd/api/main.go -o internal/api/docs --parseDependency --parseInternal

# Build all binaries with optimizations
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=$(cat VERSION 2>/dev/null || echo 'dev')" \
    -trimpath \
    -o /build/bin/dnstestergo ./cmd/dnstestergo && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /build/bin/dnstestergo-server ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /build/bin/dnstestergo-worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /build/bin/dnstestergo-query ./cmd/query

# ============================================================================
# Stage 2: Dev - Development environment for testing
# ============================================================================
FROM golang:1.25-alpine AS dev

WORKDIR /app

RUN apk add --no-cache git ca-certificates make

# Copy go.mod for dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Default command runs server in memory mode
CMD ["go", "run", "./cmd/api", "server", "--host", "0.0.0.0"]

# ============================================================================
# Stage 3: Server - API server runtime
# ============================================================================
FROM alpine:3.19 AS server

WORKDIR /app

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget && \
    addgroup -g 1000 dnstester && \
    adduser -D -u 1000 -G dnstester dnstester

# Copy binary and config
COPY --from=builder /build/bin/dnstestergo-server /usr/local/bin/
COPY --from=builder /build/conf/config.example.yaml /app/conf/config.yaml

# Set ownership
RUN chown -R dnstester:dnstester /app

USER dnstester

EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:5000/health || exit 1

ENTRYPOINT ["dnstestergo-server", "server"]
CMD ["--config", "/app/conf/config.yaml"]

# ============================================================================
# Stage 4: Worker - Async task processor
# ============================================================================
FROM alpine:3.19 AS worker

WORKDIR /app

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1000 dnstester && \
    adduser -D -u 1000 -G dnstester dnstester

# Copy binary and config
COPY --from=builder /build/bin/dnstestergo-worker /usr/local/bin/
COPY --from=builder /build/conf/config.example.yaml /app/conf/config.yaml

# Set ownership
RUN chown -R dnstester:dnstester /app

USER dnstester

# Redis URL will be passed via --redis flag from docker-compose
ENTRYPOINT ["dnstestergo-worker", "worker"]
CMD ["--config", "/app/conf/config.yaml"]

# ============================================================================
# Stage 5: Query - CLI tool for DNS queries
# ============================================================================
FROM alpine:3.19 AS query

WORKDIR /app

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -g 1000 dnstester && \
    adduser -D -u 1000 -G dnstester dnstester

# Copy binaries
COPY --from=builder /build/bin/dnstestergo-query /usr/local/bin/
COPY --from=builder /build/bin/dnstestergo /usr/local/bin/
COPY --from=builder /build/conf/config.example.yaml /app/conf/config.yaml

# Set ownership
RUN chown -R dnstester:dnstester /app

USER dnstester

ENTRYPOINT ["dnstestergo"]
CMD ["--help"]

# ============================================================================
# Stage 6: All - All-in-one image (dev/test)
# ============================================================================
FROM alpine:3.19 AS all

WORKDIR /app

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget && \
    addgroup -g 1000 dnstester && \
    adduser -D -u 1000 -G dnstester dnstester

# Copy ALL binaries
COPY --from=builder /build/bin/dnstestergo /usr/local/bin/
COPY --from=builder /build/bin/dnstestergo-server /usr/local/bin/
COPY --from=builder /build/bin/dnstestergo-worker /usr/local/bin/
COPY --from=builder /build/bin/dnstestergo-query /usr/local/bin/
COPY --from=builder /build/conf/config.example.yaml /app/conf/config.yaml

# Set ownership
RUN chown -R dnstester:dnstester /app

USER dnstester

EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:5000/health || exit 1

# Default: start server in memory mode
ENTRYPOINT ["dnstestergo-server", "server"]
CMD ["--config", "/app/conf/config.yaml"]
