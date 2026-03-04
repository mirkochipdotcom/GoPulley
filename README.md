<div align="center">

# 🪝 GoPulley

**Condivisione file aziendale — veloce, sicura, containerizzata**

*Alternativa self-hosted a WeTransfer, integrata con Active Directory*

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](https://go.dev)
[![HTMX](https://img.shields.io/badge/HTMX-2.0-3D72D7?logo=html5)](https://htmx.org)
[![SQLite](https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite)](https://sqlite.org)
[![Docker](https://img.shields.io/badge/Docker-alpine-2496ED?logo=docker)](https://docker.com)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

</div>

---

## Cos'è GoPulley

GoPulley è un'applicazione di **file sharing interno** pensata per ambienti aziendali e pubblici. Permette agli utenti autenticati via **Active Directory / LDAP** di caricare file e condividerli tramite link temporanei, con scadenza automatica configurabile.

Tutto gira in un **singolo container** leggero (~25 MB), senza dipendenze esterne: niente PHP, niente Nginx separato, niente database server.

---

## Caratteristiche

| Feature | Dettaglio |
|---|---|
| 🔐 **Login AD/LDAP** | Bind diretto sul Domain Controller, supporto `ldap://` e `ldaps://` |
| 👥 **Restrizione per gruppo** | Accesso limitato ai membri di un gruppo AD specifico (`memberOf`) |
| 📤 **Upload drag & drop** | Interfaccia moderna, senza refresh di pagina (HTMX) |
| ⏱️ **Scadenza configurabile** | 1 / 7 / 14 / 30 giorni o valore custom |
| 🔗 **Link pubblici** | Chiunque con il link può scaricare, senza login |
| 🗑️ **Pulizia automatica** | Goroutine elimina file e record scaduti ogni ora |
| 🐳 **Single container** | Multi-stage Docker/Podman, immagine finale ~25 MB |
| 🌙 **Dark mode** | UI moderna, glassmorphism, Inter font, zero npm |

---

## Architettura

```
┌─────────────────────────────────────────────┐
│              Container GoPulley             │
│                                             │
│  ┌──────────────────────────────────────┐   │
│  │         Binario Go (~12 MB)          │   │
│  │  • HTTP server (net/http)            │   │
│  │  • HTMX handler                      │   │
│  │  • LDAP client + group check         │   │
│  │  • Cleanup goroutine (ogni 1h)       │   │
│  └──────────────────────────────────────┘   │
│                                             │
│  ┌────────────┐   ┌──────────────────────┐  │
│  │ SQLite DB  │   │  File Storage        │  │
│  │ /data/*.db │   │  /data/uploads/uuid/ │  │
│  └────────────┘   └──────────────────────┘  │
│         ↑                   ↑               │
│         └──── Volume /data ─┘               │
└─────────────────────────────────────────────┘
         ↑ LDAP/LDAPS
   Domain Controller (AD)
```

```
filesharing/
├── cmd/server/main.go              # Entrypoint: HTTP server, route, sessioni
├── internal/
│   ├── auth/ldap.go                # LDAP bind + verifica gruppo memberOf
│   ├── config/config.go            # Lettura variabili d'ambiente
│   ├── database/sqlite.go          # Schema SQLite, CRUD condivisioni
│   └── storage/file.go             # I/O file, streaming download
├── web/
│   ├── templates/
│   │   ├── login.html              # Pagina di login (HTMX)
│   │   ├── dashboard.html          # Upload + lista condivisioni
│   │   └── download.html           # Pagina pubblica download
│   └── static/
│       ├── css/style.css           # Design system dark-mode (vanilla CSS)
│       └── js/htmx.min.js          # HTMX 2.0.4 (vendored)
├── .env.example                    # Template configurazione
├── Dockerfile                      # Multi-stage build
└── go.mod
```

---

## Quick Start

### Prerequisiti

- Docker o Podman

### Avvio in modalità sviluppo (mock LDAP)

```bash
git clone https://github.com/mirkochipdotcom/GoPulley.git
cd GoPulley

docker build -t gopulley:latest .

docker run -d \
  --name gopulley \
  -p 8080:8080 \
  -e SESSION_SECRET=dev-secret-locale \
  -e LDAP_HOST=mock \
  -v gopulley-data:/data \
  gopulley:latest
```

Apri il browser su **http://localhost:8080** — in mock mode qualsiasi username/password è accettata.

---

## Configurazione

Copia `.env.example` in `.env` e adatta i valori:

```bash
cp .env.example .env
```

### Variabili d'ambiente

| Variabile | Obbligatoria | Default | Descrizione |
|---|---|---|---|
| `SESSION_SECRET` | ✅ | — | Chiave HMAC per i cookie di sessione. Genera con `openssl rand -hex 32` |
| `LDAP_HOST` | ✅ | `mock` | URL del Domain Controller. Es: `ldaps://dc.esempio.it:636` |
| `LDAP_BASE_DN` | ✅ | — | Base DN del dominio. Es: `DC=esempio,DC=it` |
| `LDAP_USER_DN_TEMPLATE` | ✅ | — | Template DN utente. Es: `%s@esempio.it` |
| `LDAP_BIND_DN` | ⬜ | — | DN account di servizio per ricerche di gruppo |
| `LDAP_BIND_PASSWORD` | ⬜ | — | Password account di servizio |
| `LDAP_REQUIRED_GROUP` | ⬜ | — | CN del gruppo AD richiesto per l'accesso |
| `MAX_GLOBAL_DAYS` | ⬜ | `30` | Durata massima di una condivisione (giorni) |
| `MAX_UPLOAD_SIZE_MB` | ⬜ | `2048` | Dimensione massima per file singolo (MB) |
| `DB_PATH` | ⬜ | `/data/gopulley.db` | Percorso database SQLite |
| `UPLOAD_DIR` | ⬜ | `/data/uploads` | Cartella per i file caricati |
| `APP_PORT` | ⬜ | `8080` | Porta HTTP del server |

### Configurazione LDAP / Active Directory

**Stile UPN (Active Directory moderno)**
```env
LDAP_HOST=ldaps://dc.esempio.it:636
LDAP_BASE_DN=DC=esempio,DC=it
LDAP_USER_DN_TEMPLATE=%s@esempio.it
```

**Stile DN classico (OpenLDAP)**
```env
LDAP_HOST=ldap://ldap.esempio.it:389
LDAP_BASE_DN=dc=esempio,dc=it
LDAP_USER_DN_TEMPLATE=uid=%s,ou=Users,dc=esempio,dc=it
```

**Restrizione per gruppo AD**
```env
# Solo i membri di questo gruppo possono accedere
LDAP_REQUIRED_GROUP=NOME-DEL-GRUPPO

# Se gli utenti non hanno permessi di ricerca, aggiungi un service account
LDAP_BIND_DN=CN=srv-gopulley,OU=ServiceAccounts,DC=esempio,DC=it
LDAP_BIND_PASSWORD=password-servizio
```

> Il match sul gruppo è **case-insensitive** e cerca `CN=NOME-DEL-GRUPPO` nel campo `memberOf` dell'utente. Non è necessario specificare il DN completo del gruppo.

---

## Avvio in produzione (Podman)

```bash
podman run -d \
  --name gopulley \
  --restart unless-stopped \
  -p 8080:8080 \
  --env-file /etc/gopulley/.env \
  -v gopulley-data:/data \
  gopulley:latest
```

Oppure con variabili esplicite:

```bash
podman run -d \
  --name gopulley \
  -p 8080:8080 \
  -e SESSION_SECRET=$(openssl rand -hex 32) \
  -e LDAP_HOST=ldaps://dc.azienda.it:636 \
  -e LDAP_BASE_DN=DC=azienda,DC=it \
  -e LDAP_USER_DN_TEMPLATE=%s@azienda.it \
  -e LDAP_REQUIRED_GROUP=NOME-GRUPPO \
  -e MAX_GLOBAL_DAYS=30 \
  -v gopulley-data:/data \
  gopulley:latest
```

---

## Build da sorgente

Richiede Go 1.22+ e gcc (per `go-sqlite3`).

```bash
# Linux / macOS
CGO_ENABLED=1 go build -ldflags="-s -w" -o gopulley ./cmd/server

# Windows
set CGO_ENABLED=1
go build -ldflags="-s -w" -o gopulley.exe ./cmd/server
```

Avvio locale:
```bash
LDAP_HOST=mock SESSION_SECRET=dev-secret ./gopulley
# oppure su Windows:
$env:LDAP_HOST="mock"; $env:SESSION_SECRET="dev-secret"; .\gopulley.exe
```

---

## Flussi applicativi

### Login
1. L'utente inserisce username e password
2. Go tenta il bind LDAP con `LDAP_USER_DN_TEMPLATE` compilato
3. Se `LDAP_REQUIRED_GROUP` è configurato, ricerca `memberOf` e verifica l'appartenenza al gruppo
4. In caso di successo, genera un cookie di sessione (HttpOnly, SameSite=Lax)

### Upload
1. L'utente trascina un file nella drop zone e seleziona la scadenza
2. Go valida che `giorni ≤ MAX_GLOBAL_DAYS` e `size ≤ MAX_UPLOAD_SIZE_MB`
3. Salva il file in `/data/uploads/<uuid>/<nome-originale>`
4. Scrive in SQLite: token UUID, percorso, scadenza, uploader
5. HTMX mostra il link di condivisione inline, senza refresh di pagina

### Download (pubblica)
- `GET /d/<token>` — pagina pubblica con nome file, dimensione, scadenza e pulsante download
- `GET /download/<token>` — streaming diretto del file con `Content-Disposition: attachment`
- Link scaduti mostrano un errore graceful invece di un 404 generico

### Pulizia automatica
```
ogni 1 ora:
  SELECT * FROM shares WHERE expires_at < NOW()
  → os.RemoveAll(filepath.Dir(filePath))   # rimuove la cartella uuid
  → DELETE FROM shares WHERE token = ?
```

---

## Schema database

```sql
CREATE TABLE shares (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    token         TEXT     NOT NULL UNIQUE,   -- UUID pubblico del link
    file_path     TEXT     NOT NULL,          -- percorso fisico sul disco
    original_name TEXT     NOT NULL,          -- nome originale del file
    size_bytes    INTEGER  NOT NULL,
    uploader      TEXT     NOT NULL,          -- username AD
    created_at    DATETIME NOT NULL,
    expires_at    DATETIME NOT NULL,
    downloaded    INTEGER  NOT NULL DEFAULT 0
);
```

---

## Sicurezza

- I cookie di sessione usano HMAC-SHA256 (`gorilla/securecookie`) — il `SESSION_SECRET` deve essere lungo almeno 32 byte random
- Tutte le condivisioni sono verificate per ownership prima dell'eliminazione (403 altrimenti)
- File caricati salvati in directory UUID isolate — nessuna collisione di nomi possibile
- Il container gira come utente non-root `weshare` (uid 1001)
- Nessuna shell nel container finale (alpine minimo)
- Il `.env` è escluso dal repository via `.gitignore`

---

## Licenza

MIT — vedi [LICENSE](LICENSE)