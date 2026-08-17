# CLAUDE.md — keycloak-doctor

**keycloak-doctor** (`github.com/Allan-Nava/keycloak-doctor`): CLI Go source-available (PolyForm Noncommercial 1.0.0) che audita la configurazione di un realm Keycloak — da file di export (offline, zero credenziali) o dall'Admin REST API di un server live. Zero dipendenze, solo stdlib. Filosofia: **non** è uno scanner né un policy engine generico; sono le regole che richiedono conoscenza di dominio su Keycloak, con razionale e remediation, in un binario che gira in CI.

## Regole di lavoro (SEMPRE)

- **Ogni commit = release taggata `vX.Y.Z`**: nuova sezione in `CHANGELOG.md` (Keep a Changelog, in italiano) + `git tag -a vX.Y.Z -m "Release X.Y.Z"`. Bump `minor` per novità sostanziali (nuove regole, nuove sorgenti, nuovi output), `patch` per fix. Senza chiederlo.
- **MAI `git push`** — lo fa sempre l'utente. MAI `Co-Authored-By` nei commit.
- **Gate prima di chiudere**: `go vet ./...` + `go test ./...` + `golangci-lint run` + `go run ./cmd/gen-docs` (deve essere un no-op) verdi. Serve **golangci-lint v2** (config schema v2): `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`, stessa versione pinnata in CI.
- **Ogni regola nuova**: entra in `internal/rules/<categoria>.go`, con `ID` stabile `categoria/nome`, `Title`, `Rationale` (spiega il rischio, non ripete il titolo) e `Eval` funzione pura; test nel file `_test.go` della categoria; `go run ./cmd/gen-docs` per rigenerare `docs/rules.md`; voce nel `CHANGELOG.md`.
- **Gli id delle regole sono API**: chi mette il tool in CI pinna un id in `--skip` o in una suppression. Non rinominarli senza una major e una nota nel changelog.
- **Exit code semantics**: 0 anche con finding WARN/BAD (l'audit che gira È un successo); ≠0 solo per errori sistemici (sorgente illeggibile, credenziali che non funzionano, regola sconosciuta, flag errato). `--exit-on` per il gating CI. NON cambiare questa semantica.
- **Niente segreti** in fixture, test, doc o output. Le fixture usano `example-value-not-a-real-secret`, e c'è un test che verifica che nessun finding lo riporti.
- **Todo → `BACKLOG.md`** (sorgente unica, id stabili `KD-n`). Non sparpagliare TODO nei commenti.
- **Lingua = inglese**: codice, commenti, test e tutto l'output user-facing (messaggi dei finding, `usage`, help dei flag, errori, README, docs). **Eccezione: il `CHANGELOG.md` resta in italiano.**

## Comandi

```bash
go build -o keycloak-doctor ./cmd/keycloak-doctor
go test ./...                 # tutto locale: fixture in testdata/ e httptest per l'Admin API
go vet ./...
golangci-lint run
go run ./cmd/gen-docs         # rigenera docs/rules.md dal catalogo (gate in CI)

./keycloak-doctor audit testdata/insecure-realm.json --no-color
./keycloak-doctor audit testdata/hardened-realm.json --output markdown
./keycloak-doctor rules --only client
```

## Architettura

- `internal/engine/` — contratto dell'audit: `Status` (OK/WARN/BAD/ERROR), `Finding` (Rule/Realm/Target/Status/Message/Remediation), `Result`, `SortFindings` (worst-first, stabile), `Dedup`, `MinSeverity`, `Summarize`, `Worst`. **Nessun IO**: a differenza di un prober non c'è runner con timeout, la rete sta nel loader.
- `internal/keycloak/` — modello parziale di `RealmRepresentation` con i nomi JSON di Keycloak, più le due sorgenti: `LoadFile` (oggetto singolo / array / directory di export) e `Admin` (Admin REST API, `client_credentials` o `password`). `Scrub` elimina i valori delle credenziali e ne tiene solo la presenza; `Realm.Missing` registra le sezioni che le credenziali non hanno potuto leggere.
- `internal/rules/` — il catalogo: un file per categoria (`realm`, `client`, `mapper`, `broker`→`idp`, `keys`, `federation`, `source`), registry in `rules.go` (`All`, `Select`, `Audit`), helper dei finding in `finding.go`, helper di dominio in `uri.go` / `policy.go` / `flow.go` / `format.go`.
- `internal/output/` — renderer: `Text` (terminale), `Markdown` (report/PR comment), `JSON` (contratto di gating). I renderer vedono solo finding, mai il modello del realm.
- `cmd/keycloak-doctor/` — CLI a sottocomandi (`audit`, `rules`, `version`), `version` iniettata con `-ldflags "-X main.version=..."`.
- `cmd/gen-docs/` — genera `docs/rules.md` dal catalogo compilato.

## Trappole note / regole tecniche

- **`Attrs` coerce gli scalari a stringa di proposito**: gli export Keycloak non sono coerenti sui tipi degli attributi (una `"saml.server.signature": true` accanto a `"pkce.code.challenge.method": "S256"`). Un `map[string]string` fa fallire l'unmarshal dell'intero realm su un dettaglio di formato. Non "semplificarlo".
- **ERROR non è BAD**: ERROR significa "la regola non ha potuto essere valutata" (sezione non leggibile), non "target malato". Una regola su una sezione mancante deve riportare ERROR, mai OK: un punto cieco non può renderizzare come esito pulito.
- **Una regola che passa emette UN solo finding aggregato per realm**, non uno per oggetto: su un realm con 200 client il verde altrimenti sommerge i finding. C'è un test che lo verifica su tutto il catalogo.
- **Flag posizionale**: `flag` della stdlib si ferma al primo argomento non-flag, quindi il path posizionale di `audit` viene estratto **prima** del parse. Aggiungendo sottocomandi, replicare quel pattern.
- **I client disabilitati non producono finding** (non possono ottenere un token): filtrati in `scanClients`. Lo stesso per gli identity provider disabilitati e le esecuzioni `DISABLED` nei flow.
- **Le due sorgenti danno forme diverse per i flow** (export: liste per flow con `flowAlias`; API: lista già appiattita con `providerId`): `reachableAuthenticators` gestisce entrambe, con visited set contro i cicli.
- **`--insecure` esiste solo per i lab con chain self-signed** e va usato per il server Keycloak, non per silenziare un problema di certificati in produzione.
- Il campo `version` è iniettato dalla CI sui tag: non hardcodarlo.

## Puntatori

- Backlog: `BACKLOG.md` · CI: `.github/workflows/ci.yml` · Release: `.github/workflows/release.yml`
- Catalogo regole generato: `docs/rules.md` · Fixture: `testdata/`
- Repo affini (stessa famiglia e stesse convenzioni): `~/projects/github.com/checkfleet`, `segcheck`, `nomad-lens`, `nats-lens`, `ansible-vars-lens`
