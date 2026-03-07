<div align="center">

<img src="web/static/img/logo-icon.svg" alt="GoPulley" width="120" />

# GoPulley

**Fast, secure, containerized enterprise file sharing**

*Self-hosted WeTransfer-style alternative with Active Directory integration*

</div>

---

## What is GoPulley

GoPulley is an internal file-sharing application for enterprise and public environments.
Authenticated users (Active Directory / LDAP) can upload files and share them with temporary links and configurable expiration.

It runs in a **single lightweight container** with no external runtime dependencies.

---

## Features

- AD/LDAP authentication with optional group restriction
- Drag & drop upload UI (HTMX)
- Configurable expiration and optional max-download limit
- Optional password-protected links
- Optional SHA-256 computation and display
- Automatic cleanup for expired shares and stale upload sessions
- Single-container deployment (Docker/Podman)

---

## Quick start

```bash
curl -O https://raw.githubusercontent.com/mirkochipdotcom/GoPulley/main/compose.yml
curl -O https://raw.githubusercontent.com/mirkochipdotcom/GoPulley/main/.env.example
cp .env.example .env
mkdir -p ./data/uploads
podman compose up -d
```

Open **http://localhost:8080**.

---

## Configuration

Copy `.env.example` to `.env` and adjust values for your environment.

Important variables:
- `SESSION_SECRET`
- `LDAP_HOST`, `LDAP_BASE_DN`, `LDAP_USER_DN_TEMPLATE`
- `LDAP_REQUIRED_GROUP`, `LDAP_ADMIN_GROUP`, `ADMIN_USERS`
- `MAX_GLOBAL_DAYS`, `MAX_UPLOAD_SIZE_MB`, `USER_QUOTA_MB`
- `PUBLIC_BASE_URL`, `DATA_DIR`, `DB_PATH`, `UPLOAD_DIR`
- `ENABLE_SHA256`

---

## Build from source

Go 1.22+ and gcc are required (`go-sqlite3` uses CGO).

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o gopulley ./cmd/server
```

---

## License

GNU AGPLv3 - see [LICENSE](LICENSE).
