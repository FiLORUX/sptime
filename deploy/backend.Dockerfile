# SPTime Backend Dockerfile
# Multi-stage build for minimal production image

# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files first for better caching
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy source code
COPY backend/ ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o sptime \
    ./cmd/server

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 sptime && \
    adduser -u 1000 -G sptime -s /bin/sh -D sptime

# Create required directories
RUN mkdir -p /etc/sptime /var/lib/sptime /var/log/sptime && \
    chown -R sptime:sptime /etc/sptime /var/lib/sptime /var/log/sptime

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/sptime .

# Copy default config
COPY deploy/config.example.yaml /etc/sptime/config.yaml

# Set ownership
RUN chown sptime:sptime /app/sptime

# Switch to non-root user
USER sptime

# Expose ports
# 123: NTP (requires running as root or with CAP_NET_BIND_SERVICE)
# 4460: NTS-KE
# 319: PTP Event
# 320: PTP General
# 8080: Web API
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# Default command
ENTRYPOINT ["./sptime"]
CMD ["-config", "/etc/sptime/config.yaml"]
