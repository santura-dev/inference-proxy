# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') -s -w" \
    -o /inference-proxy ./cmd/inference-proxy/

# Runtime stage
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates curl

RUN addgroup -g 1000 inference && \
    adduser -u 1000 -G inference -s /bin/sh -D inference

COPY --from=builder /inference-proxy /usr/local/bin/
COPY --from=builder /app/config.yaml /etc/inference-proxy/config.yaml

RUN mkdir -p /var/log/inference-proxy && \
    chown -R inference:inference /var/log/inference-proxy

USER inference

EXPOSE 4000

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:4000/health || exit 1

ENTRYPOINT ["inference-proxy"]

ENV INFERENCE_PROXY_CONFIG=/etc/inference-proxy/config.yaml
ENV INFERENCE_PROXY_ADDR=:4000
