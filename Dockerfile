# ── Stage 1: Builder ────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

# gcc + musl-dev needed for go-sqlite3 (CGO)
RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# Cache dependencies layer
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Compile: static binary, strip debug info for minimal size
# Inject version via ldflags (defaults to 'dev' if not provided)
ARG VERSION=1.0.30
ARG VERSION_SUFFIX=""
RUN CGO_ENABLED=1 GOOS=linux \
  go build \
  -ldflags="-s -w -extldflags '-static' -X main.AppVersion=${VERSION}${VERSION_SUFFIX}" \
  -trimpath \
  -o gopulley \
  ./cmd/server

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
# alpine gives us CA certs (needed for LDAPS) and a minimal libc
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata \
  && adduser -D -u 1001 gopulley \
  && mkdir -p /data/uploads \
  && chown -R gopulley:gopulley /data

WORKDIR /app

# Container-friendly defaults for paths
ENV DB_PATH=/data/gopulley.db
ENV UPLOAD_DIR=/data/uploads

# Copy compiled binary
COPY --from=builder /build/gopulley .

# Copy web assets (templates + static files)
COPY --chown=gopulley:gopulley web/ ./web/

# Data volumes: SQLite DB and uploaded files
# Mount these as Docker/Podman volumes in production
VOLUME ["/data"]

# Run as non-root
USER gopulley

EXPOSE 8080

ENTRYPOINT ["/app/gopulley"]
