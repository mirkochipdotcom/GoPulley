<div align="center">

<img src="web/static/img/logo-icon.svg" alt="GoPulley" width="120" />

# GoPulley

**Fast, secure, containerized enterprise file sharing**

*Self-hosted WeTransfer-style alternative with Active Directory integration*

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](https://go.dev)
[![HTMX](https://img.shields.io/badge/HTMX-2.0-3D72D7?logo=html5)](https://htmx.org)
[![SQLite](https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite)](https://sqlite.org)
[![Docker](https://img.shields.io/badge/Docker-alpine-2496ED?logo=docker)](https://docker.com)
[![License](https://img.shields.io/badge/license-GNU%20AGPLv3-green)](LICENSE)
[![Container](https://img.shields.io/badge/ghcr.io-gopulley-7c3aed?logo=github)](https://github.com/mirkochipdotcom/GoPulley/pkgs/container/gopulley)

</div>

---

Italian version: [README.it.md](README.it.md)

---

## What is GoPulley

GoPulley is an internal file-sharing application for enterprise and public organizations.
Authenticated users (Active Directory / LDAP) can upload files and share them with temporary links and configurable expiration.

The app runs in a **single lightweight container**, with no external runtime stack.

---

## Features

| Feature | Details |
|---|---|
| AD/LDAP authentication | Direct bind to Domain Controller, supports `ldap://` and `ldaps://` |
| Optional group restriction | Limit access to members of one AD group (`memberOf`) |
| Modern upload UI | Drag and drop workflow with HTMX, no full page refresh |
| Configurable expiration | 1 / 7 / 30 days / 1 year plus max constraints |
| Public links | Recipients can download using the link |
| Optional link password | Add password protection at share creation |
| Optional max downloads | Auto-expire links after N downloads |
| Chunked/resumable upload | Large files are uploaded in chunks and can resume |
| User upload quotas | Per-user storage quota (`USER_QUOTA_MB`) |
| Admin dashboard | Global file inventory and disk usage visibility |
| Automatic cleanup | Removes expired shares and stale upload sessions |
| Optional SHA-256 | Compute and show checksum for integrity verification |
| Single container deploy | Docker/Podman, SQLite embedded |

---

## Architecture

GoPulley is intentionally simple:
- `cmd/server/main.go`: HTTP server, routes, sessions, handlers
- `internal/auth/ldap.go`: LDAP bind and group checks
- `internal/database/sqlite.go`: schema + CRUD
- `internal/storage/file.go`: file persistence and streaming
- `web/templates/*`: server-side HTML templates
- `web/static/*`: CSS and vendored HTMX

Persistent data lives under `/data` in the container:
- SQLite database (`/data/gopulley.db`)
- uploaded files (`/data/uploads/...`)

---

## Quick start

### Prerequisites

- Podman 4.7+ (or Docker with Compose plugin)

### Start in 3 steps

```bash
# 1) Download runtime files
curl -O https://raw.githubusercontent.com/mirkochipdotcom/GoPulley/main/compose.yml
curl -O https://raw.githubusercontent.com/mirkochipdotcom/GoPulley/main/.env.example

# 2) Configure env
cp .env.example .env
# Edit .env for LDAP; keep LDAP_HOST=mock for local dev

# 3) Create data dir and start
mkdir -p ./data/uploads
podman compose up -d
```

Open `http://localhost:8080`.

Docker works the same way (`docker compose ...`).

---

## Podman rootless

GoPulley supports Podman rootless out of the box.
The container entrypoint detects when it is started as root inside the user namespace
(standard rootless behavior), fixes `/data` ownership, and drops privileges to the
`gopulley` user (UID 1001) before launching the application.

### Recommended: keep-id mode (cleanest approach)

With `keep-id` your host UID is preserved inside the container and the data directory
stays owned by you on the host filesystem.

```bash
# Download the Podman override file in addition to compose.yml
curl -O https://raw.githubusercontent.com/mirkochipdotcom/GoPulley/main/compose.podman.yml

# Export your UID/GID so compose.podman.yml can read them, then start
export UID=$(id -u) GID=$(id -g)
podman compose -f compose.yml -f compose.podman.yml up -d
```

### Standard rootless (no extra file needed)

If you just run `podman compose up -d` without the override, the entrypoint automatically
fixes `/data` ownership at startup. This works correctly, but the host `./data` directory
will be re-owned to a sub-UID (the Podman rootless user namespace mapping).
To browse the data directory from the host later, use `podman unshare ls ./data`.

---

## Data directory

By default, host data is mapped to `./data`.
Use `DATA_DIR` in `.env` to move DB/uploads to another disk or a mounted network path.

```env
DATA_DIR=./data
# DATA_DIR=/mnt/storage/gopulley
# DATA_DIR=/mnt/nas/gopulley
```

---

## Configuration

Copy `.env.example` to `.env` and adjust values.

### Important variables

- `SESSION_SECRET`
- `SECURE_COOKIES`
- `LDAP_HOST`, `LDAP_BASE_DN`, `LDAP_USER_DN_TEMPLATE`
- `LDAP_REQUIRED_GROUP`, `LDAP_ADMIN_GROUP`, `ADMIN_USERS`, `LDAP_TLS_SKIP_VERIFY`
- `MAX_GLOBAL_DAYS`, `MAX_UPLOAD_SIZE_MB`, `USER_QUOTA_MB`
- `UPLOAD_CHUNK_SIZE_MB`, `UPLOAD_SESSION_TTL_HOURS`, `MAX_UPLOAD_SESSIONS_PER_USER`
- `PUBLIC_BASE_URL`, `DATA_DIR`, `DB_PATH`, `UPLOAD_DIR`
- `ENABLE_SHA256`

### Upload behavior

- Chunk size defaults to 10 MB (`UPLOAD_CHUNK_SIZE_MB`)
- In-progress sessions are auto-expired (`UPLOAD_SESSION_TTL_HOURS`)
- Concurrent in-progress uploads per user are capped (`MAX_UPLOAD_SESSIONS_PER_USER`)

### Share protection options

- Optional password at upload time
- Optional max download count ("burn after N downloads")

### Email behavior

- SMTP link sharing is configured via `SMTP_SERVER`, `SMTP_PORT`, `SMTP_SECURITY`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`
- `SMTP_USER_AUTH=true`: uses authenticated AD user email/password for sending (where supported)
- `SMTP_USER_AUTH=false`: uses shared SMTP sender credentials and messages should be treated as **non-monitored mailbox** (no-reply)

### Session cookie security

To protect credentials stored in session cookies (when `SMTP_USER_AUTH=true`), GoPulley implements:

- **Dual-key encryption**: cookies encrypted with AES-256 using a key derived from `SESSION_SECRET`
- **Dynamic `Secure` flag**: automatic protocol detection via `X-Forwarded-Proto` header

#### HTTPS Reverse Proxy (recommended configuration)

GoPulley automatically detects if the original request was HTTPS by reading the `X-Forwarded-Proto` header set by the reverse proxy:

```nginx
# Nginx example
proxy_set_header X-Forwarded-Proto $scheme;
```

```yaml
# Traefik example (automatic with entryPoints)
labels:
  - "traefik.http.routers.gopulley.entrypoints=websecure"
  - "traefik.http.routers.gopulley.tls=true"
```

With this configuration:
- **Browser → Reverse Proxy**: HTTPS (TLS terminated at proxy)
- **Reverse Proxy → GoPulley**: HTTP local (trusted network)
- **Cookie `Secure` flag**: automatically `true` because `X-Forwarded-Proto: https`

#### Fallback configuration

If the `X-Forwarded-Proto` header is not present (direct access without reverse proxy), GoPulley uses the `SECURE_COOKIES` variable:

```env
# Direct HTTPS access to the app (no reverse proxy)
SECURE_COOKIES=true

# Direct HTTP access (local testing only - NOT for production)
SECURE_COOKIES=false
```

**Note**: in production with a properly configured HTTPS reverse proxy, `SECURE_COOKIES` is ignored and the `Secure` flag is set automatically.

### LDAP examples

UPN style (modern AD):

```env
LDAP_HOST=ldaps://dc.example.com:636
LDAP_BASE_DN=DC=example,DC=com
LDAP_USER_DN_TEMPLATE=%s@example.com
```

Classic DN style:

```env
LDAP_HOST=ldap://ldap.example.com:389
LDAP_BASE_DN=dc=example,dc=com
LDAP_USER_DN_TEMPLATE=uid=%s,ou=Users,dc=example,dc=com
```

---

## Production operations

```bash
# first run
podman compose up -d

# update to latest image
podman compose pull && podman compose up -d

# logs
podman compose logs -f

# stop
podman compose down
```

---

## Container images

Images are published automatically on GitHub Container Registry on tag push.

```bash
# latest
podman pull ghcr.io/mirkochipdotcom/gopulley:latest

# specific tag
podman pull ghcr.io/mirkochipdotcom/gopulley:0.9.8
```

---

## Build from source

Requires Go 1.22+ and gcc (`go-sqlite3` uses CGO).

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o gopulley ./cmd/server
```

Run local dev:

```bash
LDAP_HOST=mock SESSION_SECRET=dev-secret ./gopulley
```

---

## Internationalization

UI strings are served via i18n bundles.
Current supported locales in code:
- `en`
- `it`
- `es`
- `de`
- `fr`

Locale is resolved from `Accept-Language` with fallback to English.

---

## License

GNU AGPLv3 - see [LICENSE](LICENSE).
