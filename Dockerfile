# ============================================
# Stage 1: Build Go binary
# ============================================
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-X main.Version=$(cat VERSION 2>/dev/null || echo docker)" \
    -o moneyprinterFaster ./cmd/server

# ============================================
# Stage 2: Runtime with FFmpeg + Python + edge-tts
# ============================================
FROM python:3.12-slim

# Install FFmpeg and common utilities
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ffmpeg \
        fonts-noto-cjk \
        ca-certificates \
        curl && \
    rm -rf /var/lib/apt/lists/*

# Install edge-tts (auto-installed on first run, but pre-install for speed)
RUN pip install --no-cache-dir edge-tts

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/moneyprinterFaster .

# Copy default config
COPY config.example.toml ./config.toml

# Create data directory
RUN mkdir -p data

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8080/api/v1/status || exit 1

# Run
CMD ["./moneyprinterFaster", "-config", "config.toml"]
