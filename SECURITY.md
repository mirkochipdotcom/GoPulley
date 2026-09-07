# Security Policy

## Segnalazione di una vulnerabilità

Se scopri una vulnerabilità di sicurezza in GoPulley, **non aprire una issue pubblica**.

Segnala privatamente via [GitHub Security Advisories](https://github.com/mirkochipdotcom/GoPulley/security/advisories/new) — visibile solo al maintainer finché non viene risolta e pubblicata.

## Versioni supportate

Solo l'ultima versione taggata riceve fix di sicurezza. Nessun supporto a versioni precedenti.

## Cosa aspettarsi

- Nessun bug bounty: progetto personale open-source.
- Le vulnerabilità nelle dipendenze sono monitorate automaticamente (Dependabot + Trivy su go.mod/go.sum ad ogni push/PR + Trivy sull'immagine Docker ad ogni release, risultati nella tab [Security](https://github.com/mirkochipdotcom/GoPulley/security)).
