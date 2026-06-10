# ============================================
# Stage 1: Build Go binary
# ============================================
FROM golang:1.25-alpine AS builder

# Use domestic mirror for Alpine apk
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

WORKDIR /build

# Use domestic Go module proxy
ENV GOPROXY=https://goproxy.cn,direct

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

# Use domestic mirrors
# Debian apt: mirrors.tuna.tsinghua.edu.cn
RUN sed -i 's|deb.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources && \
    sed -i 's|security.debian.org|mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources

# PyPI: mirrors.aliyun.com
RUN pip config set global.index-url https://mirrors.aliyun.com/pypi/simple/ && \
    pip config set global.trusted-host mirrors.aliyun.com

# Install FFmpeg and common utilities
RUN apt-get update && \
    apt-get install -y --no-install-recommends --fix-missing \
        -o APT::Acquire::Retries=5 \
        -o APT::Acquire::https::Timeout=30 \
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
