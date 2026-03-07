## Plan: Chunk Upload con Resume

Implementare un nuovo flusso upload interamente chunked con pre-check preventivo, sessione di upload server-side e completamento atomico finale. L'approccio consigliato e' `check-upload -> init -> status -> chunk -> complete` con supporto resume immediato, chunk size default 10 MB, e dismissione del submit monolitico lato dashboard (solo chunk UI), mantenendo comunque il vecchio endpoint `/upload` come compatibilita' backend non usata dal client.

**Steps**
1. Definire contratto API e payload JSON per tutte le rotte chunked, includendo codici errore standardizzati e shape risposta progress. Dipendenza: nessuna.
2. Estendere persistenza SQLite con tabella `uploads_in_progress` e metodi DB dedicati (create session, update chunk state, list received chunks, finalize, abort, cleanup stale). Dipendenza: 1.
3. Implementare endpoint `POST /api/check-upload` autenticato per validare `MaxUploadSizeMB`, quota utente (`GetUserTotalBytes`) e vincoli base file prima dell'inizializzazione. Dipendenza: 1.
4. Implementare endpoint `POST /api/upload/init` autenticato che crea sessione upload (token, metadata, opzioni days/password/max_downloads), hash password se presente e cartella temporanea in `UploadDir/.chunks/{session}`. Dipendenza: 1,2.
5. Implementare endpoint `GET /api/upload/{session}/status` per resume (chunk ricevuti, bytes ricevuti, chunk_size, total_chunks). Dipendenza: 2,4.
6. Implementare endpoint `POST /api/upload/{session}/chunk/{index}` con idempotenza per chunk index, validazione range chunk, write su file temporaneo chunk, update progress DB e blocco upload cross-user. Dipendenza: 2,4.
7. Implementare endpoint `POST /api/upload/{session}/complete` che verifica chunk mancanti, ricompone file finale in percorso definitivo, crea share DB (`CreateShare`), avvia SHA256 async se attivo, elimina temporanei e chiude sessione in modo atomico (best effort con rollback/cleanup). Dipendenza: 2,4,6.
8. Implementare endpoint `DELETE /api/upload/{session}` per abort manuale e cleanup completo risorse temporanee. Dipendenza: 2,4.
9. Aggiornare cleanup job server per includere sessioni upload stale (`uploads_in_progress.expires_at`) e cancellazione chunk temporanei associati. Dipendenza: 2.
10. Aggiornare routing HTTP in `main.go` per nuove rotte `/api/*` sotto `requireAuth` e mantenere separazione netta tra API JSON e handler HTML. Dipendenza: 3-9.
11. Refactor frontend dashboard: sostituire submit HTMX upload con orchestratore JS chunked (check -> init -> upload loop -> complete), mantenendo UI corrente (dropzone, progress bar, toast, refresh lista). Dipendenza: 1,3-7.
12. Implementare resume client: su errore rete o reload, interrogare `/status`, riprendere dai chunk mancanti, con retry esponenziale per singolo chunk e pulsante annulla che invoca `DELETE /api/upload/{session}`. Dipendenza: 5,6,11.
13. Uniformare gestione errori UX (quota, file troppo grande, sessione scaduta, chunk corrotto, complete fallito) con messaggi leggibili e stato coerente della progress bar. Dipendenza: 11,12.
14. Hardening: lock per session upload concorrenti, limiti su numero sessioni per utente, sanitizzazione filename, controllo content-length per chunk, e protezioni anti path traversal durante compose finale. Dipendenza: 4-7.

**Parallelism / Dependencies**
1. Fase backend core (steps 2-10) blocca il frontend finale, ma `check-upload` (step 3) e migrazione DB (step 2) possono avanzare in parallelo.
2. `status` (step 5) e `abort` (step 8) possono essere implementati in parallelo dopo `init` + persistenza base.
3. UI chunked (step 11) puo' iniziare con mock API ma va finalizzata dopo `chunk` e `complete`.
4. Hardening (step 14) puo' procedere in parallelo ai test manuali finali.

**Relevant files**
- `c:\Users\mirko.daddiego\Documents\filesharing\cmd\server\main.go` - aggiunta nuove rotte API, nuovi handler JSON, aggiornamento `startCleanupJob`, riuso simboli `requireAuth`, `getUsername`, `handleUpload` come riferimento.
- `c:\Users\mirko.daddiego\Documents\filesharing\internal\database\sqlite.go` - nuova migrazione/tabella `uploads_in_progress`, CRUD stato chunk, finalize transaction boundaries.
- `c:\Users\mirko.daddiego\Documents\filesharing\internal\storage\file.go` - helper per temp chunk path, compose finale, cleanup sicuro directory temporanee, validazioni path.
- `c:\Users\mirko.daddiego\Documents\filesharing\internal\config\config.go` - eventuali config aggiuntive (`UPLOAD_CHUNK_SIZE_MB`, TTL upload session).
- `c:\Users\mirko.daddiego\Documents\filesharing\web\templates\dashboard.html` - sostituzione submit upload con orchestratore chunk JS, integrazione progress/resume/abort.
- `c:\Users\mirko.daddiego\Documents\filesharing\web\static\css\style.css` - eventuali stati UI aggiuntivi per progress avanzato/retry/errore.
- `c:\Users\mirko.daddiego\Documents\filesharing\README.md` - aggiornamento API e comportamento upload.

**Verification**
1. Test API check: chiamata autenticata a `/api/check-upload` con file entro quota e oltre quota; verificare codici HTTP e payload reason.
2. Test init/chunk/complete happy path con file grande (>100MB): progress fino a 100%, share visibile in dashboard, download funzionante.
3. Test resume: interrompere rete a meta', riaprire pagina, ripresa da chunk mancanti via `/status`, complete senza duplicazioni.
4. Test idempotenza chunk: reinviare stesso chunk index e verificare nessuna doppia contabilizzazione bytes.
5. Test abort: avviare upload, annullare, verificare eliminazione temp files e stato DB in-progress rimosso.
6. Test cleanup stale: forzare sessione scaduta, eseguire job cleanup, verificare rimozione DB + filesystem.
7. Test regressione opzioni upload: `days`, `password`, `max_downloads` propagate correttamente fino a share finale.
8. Test sicurezza: tentativi chunk su sessione di altro utente devono dare 403; filename con path traversal deve essere neutralizzato.

**Decisions**
- API scelta: `init/chunk/complete` con endpoint aggiuntivi `check-upload`, `status`, `abort`.
- Resume incluso nella prima release.
- Chunk size default: 10 MB.
- UI dashboard: solo flusso chunked lato client (niente fallback al submit monolitico).
- Endpoint legacy `/upload` mantenuto disponibile lato server per compatibilita' temporanea ma non usato dalla dashboard.

**Scope boundaries**
- Incluso: chunked upload autenticato, pre-check quota/dimensione, resume, abort, cleanup stale, integrazione UI dashboard.
- Escluso: protocollo TUS, upload multi-file simultaneo, checksum per chunk con crittografia avanzata end-to-end, API pubbliche anonime.

**Further Considerations**
1. Persistenza stato resume lato client: preferenza raccomandata `localStorage` con chiave per file fingerprint + size + lastModified; alternativa `IndexedDB` se serve robustezza superiore.
2. Strategia compose finale: raccomandata ricomposizione server-side sequenziale su file unico per ridurre RAM; opzionale parallel merge non consigliata su SQLite + disco locale.
3. Limiti operativi: raccomandato tetto sessioni upload in-progress per utente (es. 3) e timeout inattivita' (es. 24h) configurabile.
