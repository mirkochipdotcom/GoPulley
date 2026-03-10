<div align="center">

<img src="web/static/img/logo-icon.svg" alt="GoPulley" width="120" />

# GoPulley

**Condivisione file aziendale veloce, sicura e containerizzata**

*Alternativa self-hosted in stile WeTransfer con integrazione Active Directory*

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](https://go.dev)
[![HTMX](https://img.shields.io/badge/HTMX-2.0-3D72D7?logo=html5)](https://htmx.org)
[![SQLite](https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite)](https://sqlite.org)
[![Docker](https://img.shields.io/badge/Docker-alpine-2496ED?logo=docker)](https://docker.com)
[![License](https://img.shields.io/badge/license-GNU%20AGPLv3-green)](LICENSE)
[![Container](https://img.shields.io/badge/ghcr.io-gopulley-7c3aed?logo=github)](https://github.com/mirkochipdotcom/GoPulley/pkgs/container/gopulley)

</div>

---

Versione inglese: [README.md](README.md)

---

## Cos'e GoPulley

GoPulley e un'applicazione di file sharing interno per contesti enterprise e pubblici.
Gli utenti autenticati (Active Directory / LDAP) possono caricare file e condividerli con link temporanei e scadenza configurabile.

L'app gira in un **singolo container leggero**, senza stack runtime esterno.

---

## Funzionalita

| Funzionalita | Dettaglio |
|---|---|
| Autenticazione AD/LDAP | Bind diretto al Domain Controller, supporto `ldap://` e `ldaps://` |
| Restrizione gruppo opzionale | Accesso limitato ai membri di un gruppo AD (`memberOf`) |
| Upload moderno | Drag and drop con HTMX, senza refresh completo |
| Scadenza configurabile | 1 / 7 / 30 giorni / 1 anno con limiti globali |
| Link pubblici | I destinatari scaricano con il link |
| Password opzionale sul link | Protezione configurabile in fase di upload |
| Limite massimo download | Auto-scadenza dopo N download |
| Upload chunked/riprendibile | I file grandi sono caricati a blocchi con resume |
| Quote utente | Limite spazio per utente (`USER_QUOTA_MB`) |
| Dashboard admin | Vista globale file e utilizzo disco |
| Pulizia automatica | Rimuove share scadute e sessioni upload stale |
| SHA-256 opzionale | Checksum in pagina download per verifica integrita |
| Deploy single-container | Docker/Podman con SQLite embedded |

---

## Architettura

Struttura principale:
- `cmd/server/main.go`: HTTP server, route, sessioni, handler
- `internal/auth/ldap.go`: bind LDAP e check gruppi
- `internal/database/sqlite.go`: schema + CRUD
- `internal/storage/file.go`: salvataggio e streaming file
- `web/templates/*`: template HTML server-side
- `web/static/*`: CSS e HTMX vendorizzato

I dati persistenti sono sotto `/data` nel container:
- database SQLite (`/data/gopulley.db`)
- file caricati (`/data/uploads/...`)

---

## Quick start

### Prerequisiti

- Podman 4.7+ (oppure Docker con plugin Compose)

### Avvio in 3 passi

```bash
# 1) Scarica i file runtime
curl -O https://raw.githubusercontent.com/mirkochipdotcom/GoPulley/main/compose.yml
curl -O https://raw.githubusercontent.com/mirkochipdotcom/GoPulley/main/.env.example

# 2) Configura env
cp .env.example .env
# Modifica .env per LDAP; lascia LDAP_HOST=mock per dev locale

# 3) Crea cartella dati e avvia
mkdir -p ./data/uploads
podman compose up -d
```

Apri `http://localhost:8080`.

Con Docker il flusso e identico (`docker compose ...`).

---

## Podman rootless

GoPulley supporta Podman rootless nativamente.
L'entrypoint del container rileva quando viene avviato come root nel user namespace
(comportamento rootless standard), corregge i permessi di `/data` e poi abbassa i
privilegi all'utente `gopulley` (UID 1001) prima di avviare l'applicazione.

### Consigliato: modalità keep-id (approccio più pulito)

Con `keep-id` il tuo UID host viene preservato all'interno del container e la directory
dati rimane di tua proprietà sul filesystem host.

```bash
# Scarica il file di override Podman oltre a compose.yml
curl -O https://raw.githubusercontent.com/mirkochipdotcom/GoPulley/main/compose.podman.yml

# Esporta il tuo UID/GID in modo che compose.podman.yml possa leggerli, poi avvia
export UID=$(id -u) GID=$(id -g)
podman compose -f compose.yml -f compose.podman.yml up -d
```

### Rootless standard (nessun file extra necessario)

Se esegui semplicemente `podman compose up -d` senza l'override, l'entrypoint corregge
automaticamente i permessi di `/data` all'avvio. Funziona correttamente, ma la directory
host `./data` verrà riassegnata a un sub-UID (la mappatura del user namespace rootless di
Podman). Per navigare la directory dati dall'host in seguito, usa `podman unshare ls ./data`.

---

## Directory dati

Di default i dati host sono mappati su `./data`.
Usa `DATA_DIR` in `.env` per spostare DB/uploads su altro disco o share montata.

```env
DATA_DIR=./data
# DATA_DIR=/mnt/storage/gopulley
# DATA_DIR=/mnt/nas/gopulley
```

---

## Configurazione

Copia `.env.example` in `.env` e personalizza i valori.

### Variabili importanti

- `SESSION_SECRET`
- `SECURE_COOKIES`
- `LDAP_HOST`, `LDAP_BASE_DN`, `LDAP_USER_DN_TEMPLATE`
- `LDAP_REQUIRED_GROUP`, `LDAP_ADMIN_GROUP`, `ADMIN_USERS`, `LDAP_TLS_SKIP_VERIFY`
- `MAX_GLOBAL_DAYS`, `MAX_UPLOAD_SIZE_MB`, `USER_QUOTA_MB`
- `UPLOAD_CHUNK_SIZE_MB`, `UPLOAD_SESSION_TTL_HOURS`, `MAX_UPLOAD_SESSIONS_PER_USER`
- `PUBLIC_BASE_URL`, `DATA_DIR`, `DB_PATH`, `UPLOAD_DIR`
- `ENABLE_SHA256`

### Comportamento upload

- Chunk size di default 10 MB (`UPLOAD_CHUNK_SIZE_MB`)
- Sessioni upload in corso con scadenza automatica (`UPLOAD_SESSION_TTL_HOURS`)
- Limite upload concorrenti per utente (`MAX_UPLOAD_SESSIONS_PER_USER`)

### Opzioni protezione share

- Password opzionale in fase di upload
- Limite massimo download opzionale ("burn after N downloads")

### Comportamento email

GoPulley invia le notifiche email in base alla configurazione SMTP nel file `.env`:

- `SMTP_HOST`, `SMTP_PORT`: server SMTP e porta
- `SMTP_SECURITY`: `auto`, `starttls`, `ssl`, `none`
- `SMTP_FROM`: mittente visualizzato
- `SMTP_USER_AUTH`: controlla il tipo di autenticazione SMTP

Quando `SMTP_USER_AUTH=true`:

- GoPulley usa credenziali SMTP dell'utente autenticato (se fornite dal flusso applicativo/relay)
- il mittente e la tracciabilita sono personali per utente

Quando `SMTP_USER_AUTH=false`:

- GoPulley usa l'account SMTP condiviso configurato lato server
- la casella usata per l'invio puo essere considerata "no-reply" / non presidiata

Nota operativa: in ambienti enterprise e consigliato usare relay SMTP interni con TLS obbligatorio e policy antispam lato infrastruttura.

### Sicurezza cookie sessione

Per proteggere le credenziali memorizzate nella sessione (quando `SMTP_USER_AUTH=true`), GoPulley implementa:

- **Crittografia dual-key**: cookie crittografati con AES-256 usando chiave derivata da `SESSION_SECRET`
- **Flag `Secure` dinamico**: rilevamento automatico del protocollo tramite `X-Forwarded-Proto` header

#### Reverse Proxy HTTPS (configurazione raccomandata)

GoPulley rileva automaticamente se la richiesta originale era HTTPS leggendo l'header `X-Forwarded-Proto` impostato dal reverse proxy:

```nginx
# Esempio Nginx
proxy_set_header X-Forwarded-Proto $scheme;
```

```yaml
# Esempio Traefik (automatico con entryPoints)
labels:
  - "traefik.http.routers.gopulley.entrypoints=websecure"
  - "traefik.http.routers.gopulley.tls=true"
```

Con questa configurazione:
- **Browser → Reverse Proxy**: HTTPS (TLS terminato sul proxy)
- **Reverse Proxy → GoPulley**: HTTP locale (rete fidata)
- **Cookie flag `Secure`**: automaticamente `true` perché `X-Forwarded-Proto: https`

#### Configurazione fallback

Se l'header `X-Forwarded-Proto` non è presente (accesso diretto senza reverse proxy), GoPulley usa la variabile `SECURE_COOKIES`:

```env
# Accesso diretto HTTPS all'app (senza reverse proxy)
SECURE_COOKIES=true

# Accesso diretto HTTP (solo per test locali - NON in produzione)
SECURE_COOKIES=false
```

**Nota**: in produzione con reverse proxy HTTPS correttamente configurato, `SECURE_COOKIES` viene ignorato e il flag `Secure` è impostato automaticamente.

### Esempi LDAP

Stile UPN (AD moderno):

```env
LDAP_HOST=ldaps://dc.example.com:636
LDAP_BASE_DN=DC=example,DC=com
LDAP_USER_DN_TEMPLATE=%s@example.com
```

Stile DN classico:

```env
LDAP_HOST=ldap://ldap.example.com:389
LDAP_BASE_DN=dc=example,dc=com
LDAP_USER_DN_TEMPLATE=uid=%s,ou=Users,dc=example,dc=com
```

---

## Operazioni in produzione

```bash
# primo avvio
podman compose up -d

# aggiornamento immagine
podman compose pull && podman compose up -d

# log
podman compose logs -f

# stop
podman compose down
```

---

## Immagini container

Le immagini vengono pubblicate automaticamente su GitHub Container Registry al push dei tag.

```bash
# latest
podman pull ghcr.io/mirkochipdotcom/gopulley:latest

# tag specifico
podman pull ghcr.io/mirkochipdotcom/gopulley:0.9.8
```

---

## Build da sorgente

Richiede Go 1.22+ e gcc (`go-sqlite3` usa CGO).

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o gopulley ./cmd/server
```

Avvio locale:

```bash
LDAP_HOST=mock SESSION_SECRET=dev-secret ./gopulley
```

---

## Internazionalizzazione

Le stringhe UI usano bundle i18n.
Lingue supportate nel codice:
- `en`
- `it`
- `es`
- `de`
- `fr`

La lingua viene risolta da `Accept-Language` con fallback inglese.

---

## Licenza

GNU AGPLv3 - vedi [LICENSE](LICENSE).
