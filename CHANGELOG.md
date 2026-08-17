# Changelog

Tutte le modifiche rilevanti a questo progetto sono documentate qui.
Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e il progetto usa il [Semantic Versioning](https://semver.org/lang/it/).

## [0.1.0] - 2026-08-17

Prima release: audit della configurazione di un realm Keycloak, da file di export o da server live, in un solo binario Go senza dipendenze.

### Aggiunto

- **Motore di audit** (`internal/engine`): contratto `Finding` con `Status` OK/WARN/BAD/ERROR, ordinamento worst-first stabile, dedup, filtro per severità minima, `Summarize`/`Worst`. Nessun IO: le regole sono funzioni pure sul modello del realm.
- **Due sorgenti, un solo modello** (`internal/keycloak`): parser dell'export di un realm (oggetto singolo, array di realm o directory di export, saltando i file utenti) e client dell'Admin REST API (grant `client_credentials` o `password`), che assemblano la stessa forma parziale di `RealmRepresentation`.
- **Igiene delle credenziali**: `Scrub` elimina ogni valore di credenziale al caricamento (client secret, `bindCredential` LDAP, client secret dei broker) e ne conserva solo la presenza — nessun finding, output JSON o messaggio d'errore può stamparne uno. I segreti per l'API si passano per nome di variabile d'ambiente (`--client-secret-env`, `--password-env`), mai come valore di flag.
- **Sezioni non leggibili marcate, non ignorate**: una sezione che le credenziali non possono leggere (HTTP 403) non fa fallire il run, ma le regole su quella sezione riportano `ERROR — not evaluated`. Un punto cieco non può mai essere confuso con un esito pulito.
- **30 regole in 7 categorie** (`internal/rules`), ognuna con titolo e razionale nel binario:
  - `realm` (13): SSL richiesto, brute force detection, durata degli access token, durata delle sessioni SSO, scadenza dei token offline, rotazione dei refresh token, password policy (lunghezza, `notUsername`, iterazioni di hash secondo OWASP), self-registration senza verifica email, identità via email, OTP policy, eventi di audit, secondo fattore nel browser flow, realm disabilitato.
  - `client` (8): redirect URI con wildcard (severità in base alla portata: authority o solo path), redirect URI in HTTP non-loopback, web origins `*`, implicit flow, password grant (BAD sui client pubblici), PKCE S256 obbligatorio sui client pubblici, full scope (BAD sui service account), override della durata dei token.
  - `mapper` (2): mapper che copiano attributi con aspetto di credenziale in un claim, audience mapper verso un client terzo.
  - `idp` (2): broker con Trust email, endpoint del broker in HTTP.
  - `keys` (2): chiavi RSA sotto i 2048 bit, segreti HMAC sotto i 32 byte.
  - `federation` (2): LDAP senza TLS, LDAP senza verifica del certificato.
  - `source` (1): quanto materiale credenziale porta con sé la sorgente audita.
- **CLI a sottocomandi** (stdlib `flag`, niente cobra): `audit` (con path posizionale come forma breve di `--file`), `rules`, `version`. Selezione delle regole con `--only`/`--skip` per id o categoria, con errore su selettore inesistente: un audit ridotto per una typo è il modo peggiore di mentire.
- **Output** (`internal/output`): `text` per il terminale (worst-first, remediation sotto ogni finding, colori opzionali), `markdown` (summary, "Needs attention", tabella completa) e `json` come contratto di gating (`worst`, `summary`, finding con id di regola stabile).
- **Semantica degli exit code**: 0 anche con finding WARN/BAD, `--exit-on warn|bad|error` con `--exit-code N` per il gating in CI, non-zero solo per errori sistemici.
- **Documentazione generata**: `docs/rules.md` prodotto da `go run ./cmd/gen-docs` dal catalogo compilato, con gate in CI che la rigenerazione sia un no-op.
- **Test**: suite completa su fixture locali (`testdata/insecure-realm.json`, `testdata/hardened-realm.json`) e server `httptest` per l'Admin API — nessun test tocca la rete o un Keycloak reale.

[0.1.0]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.0
