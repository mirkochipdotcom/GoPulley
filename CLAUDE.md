# GoPulley

File sharing self-hosted in Go (net/http + htmx, SQLite via CGO, auth LDAP/AD).
Immagine pubblicata su `ghcr.io/mirkochipdotcom/gopulley`.

## Sviluppo

- `go-sqlite3` richiede CGO: su Windows senza gcc i test vanno lanciati in container:
  `docker run --rm -v "$(pwd -W):/src" -w /src -e CGO_ENABLED=1 golang:1.27-alpine sh -c 'apk add -q gcc musl-dev && go test ./... -race'`
- `LDAP_HOST=mock` (default) accetta qualsiasi credenziale: il codice di `go-ldap` non è coperto dai test.
- htmx è vendorizzato in `web/static/js/htmx-<versione>.min.js` (versione nel nome del file, letta da `upstream-check.yml`).
  Per aggiornarlo: sostituire il file e i riferimenti in `web/templates/*.html`.

## CI/CD

| Workflow | Scopo |
|---|---|
| `test.yml` | build, vet, `go test -race`, Trivy fs (bloccante) |
| `build.yml` | build `load` → smoke → Trivy immagine (bloccante, `CRITICAL,HIGH`) → push. Mai push su PR. Cron lunedì 04:00 UTC ripubblica `:latest` da main |
| `ci-gate.yml` | job `CI gate`: required check del ruleset di main, insieme a `test` |
| `dependabot-automerge.yml` | patch/minor → `gh pr merge --auto --squash` con token GitHub App; major / tipo vuoto / sensibili → label `revisione-manuale` |
| `update-behind.yml` | aggiorna una PR dependabot BEHIND alla volta (ruleset strict). Trigger: push su main, fine di ogni `CI gate`, cron orario |
| `upstream-check.yml` | issue `upstream` per dipendenze invisibili a dependabot (htmx, binario Trivy) |

Tag immagine: `:main` (push/cron/dispatch su main), `:sha-<short>` (rollback), `:vX.Y.Z` + semver sui tag `v*`,
`:latest` solo su tag `v*` o su schedule/dispatch da main. Footer UI: tag `v*` oppure ultimo tag raggiungibile.

### Decisioni e gotcha

- **Workflow con `paths:` non possono essere required check**: se non partono la PR resta bloccata. Per questo
  esiste `CI gate` (senza filtri) che attende e valuta tutti gli altri run/check del commit (logica in `.github/ci-gate.jq`,
  esclude se stesso, gli eventi `dynamic` e i check dell'app `github-actions`, tiene solo l'esecuzione più recente).
  Aggiungendo un nuovo workflow su PR non serve toccare il ruleset.
- Required check del ruleset: `CI gate` + `test`. Il gate vede solo i workflow partiti: `test` è richiesto anche per nome
  così, se `test.yml` venisse disabilitato o rinominato, le PR restano bloccate invece di passare. `test.yml` non deve
  mai avere filtri `paths:` (un required check che non parte blocca la PR per sempre).
- **Azioni fatte con `GITHUB_TOKEN` non avviano altri workflow** (merge, push, update-branch). Merge e update-branch usano
  il token della GitHub App (`actions/create-github-app-token`, input `client-id`, `app-id` è deprecato).
- I workflow avviati da PR dependabot vedono **solo i secret Dependabot**: `APP_CLIENT_ID`/`APP_PRIVATE_KEY` vanno in entrambi.
- Dipendenze sensibili (mai auto-merge): `github.com/go-ldap/ldap/v3` — regex in `SENSITIVE_DEPS` di
  `dependabot-automerge.yml` e `exclude-patterns` del gruppo gomod in `dependabot.yml`: tenerli allineati.
- `gorilla/sessions`, `x/crypto` (bcrypt) e `go-sqlite3` sono in auto-merge perché coperti da test di compatibilità con
  valori golden (`cmd/server/compat_test.go`, `internal/database/sqlite_compat_test.go`). Se un test golden fallisce dopo
  un bump, il bump rompe cookie/hash/DB esistenti in produzione: non rigenerare i golden senza capire perché.
- `dependabot.yml`: `cooldown.semver-major-days` solo su gomod; su docker e github-actions invalida il file (errore visibile
  solo nel check `.github/dependabot.yml` di una PR che lo tocca). Gruppi solo minor/patch, gruppo dedicato `codeql`.
- Basi Docker (`golang:*-alpine`, `alpine:*`) flottanti: le patch arrivano col rebuild settimanale, che passa da smoke e Trivy.
- Action pinnate per commit SHA con commento `# vX.Y.Z`, release più vecchie del cooldown (7 giorni). SHA da
  `gh api repos/<action>/git/ref/tags/<tag>` verificando `object.type == "commit"`.
- Versione Trivy: `TRIVY_VERSION` in `build.yml` e `version:` in `test.yml` vanno tenute allineate.
- `actionlint` prima di committare workflow: `docker run --rm -v "$(pwd -W):/repo" -w /repo rhysd/actionlint:1.7.7`.

### Deploy

Produzione: stack Portainer collegato a git su `:latest`. Serve "Re-pull image" (+ "Force redeployment"), altrimenti il
`:latest` settimanale non viene scaricato quando il repo non cambia.
