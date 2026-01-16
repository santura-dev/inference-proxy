# Build stage
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Install git for go build
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -s -w" \
    -o /litellm-proxy ./cmd/litellm-proxy/

# Runtime stage
FROM alpine:3.19 AS runtime

# Install ca-certificates for HTTPS connections
RUN apk add --no-cache ca-certificates curl

# Create non-root user
RUN addgroup -g 1000 litellm && \
    adduser -u 1000 -G litellm -s /bin/sh -D litellm

# Copy binary from builder
COPY --from=builder /litellm-proxy /usr/local/bin/

# Copy default config
COPY --from=builder /app/config.yaml /etc/litellm/config.yaml

# Create directories
RUN mkdir -p /var/log/litellm && \
    chown -R litellm:litellm /var/log/litellm

# Switch to non-root user
USER litellm

# Expose ports
EXPOSE 4000 4001

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:4000/health || exit 1

# Run the proxy
ENTRYPOINT ["litellm-proxy"]

# Default environment variables
ENV LITELLM_CONFIG=/etc/litellm/config.yaml
ENV LITELLM_ADDR=:4000
ENV LITELLM_LOG_LEVEL=info
