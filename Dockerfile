# ── Stage 1: Builder ────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

# gcc + musl-dev needed for go-sqlite3 (CGO)
RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# Cache dependencies layer
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Compile: static binary, strip debug info for minimal size
RUN CGO_ENABLED=1 GOOS=linux \
  go build \
  -ldflags="-s -w -extldflags '-static'" \
  -trimpath \
  -o we-share \
  ./cmd/server

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
# alpine gives us CA certs (needed for LDAPS) and a minimal libc
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata \
  && adduser -D -u 1001 weshare \
  && mkdir -p /data/uploads \
  && chown -R weshare:weshare /data

WORKDIR /app

# Copy compiled binary
COPY --from=builder /build/we-share .

# Copy web assets (templates + static files)
COPY --chown=weshare:weshare web/ ./web/

# Data volumes: SQLite DB and uploaded files
# Mount these as Docker/Podman volumes in production
VOLUME ["/data"]

# Run as non-root
USER weshare

EXPOSE 8080

ENTRYPOINT ["/app/we-share"]
