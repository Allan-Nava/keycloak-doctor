# CLAUDE.md — keycloak-doctor

**keycloak-doctor** (`github.com/Allan-Nava/keycloak-doctor`): CLI Go source-available (PolyForm Noncommercial 1.0.0) che audita la configurazione di un realm Keycloak — da file di export (offline, zero credenziali) o dall'Admin REST API di un server live. Zero dipendenze, solo stdlib. Filosofia: **non** è uno scanner né un policy engine generico; sono le regole che richiedono conoscenza di dominio su Keycloak, con razionale e remediation, in un binario che gira in CI.

## Regole di lavoro (SEMPRE)

- **Ogni commit = release taggata `vX.Y.Z`**: nuova sezione in `CHANGELOG.md` (Keep a Changelog, in italiano) + `git tag -a vX.Y.Z -m "Release X.Y.Z"`. Bump `minor` per novità sostanziali (nuove regole, nuove sorgenti, nuovi output), `patch` per fix. Senza chiederlo.
- **MAI `git push`** — lo fa sempre l'utente. MAI `Co-Authored-By` nei commit.
- **Gate prima di chiudere**: `go vet ./...` + `go test ./...` + `golangci-lint run` + `go run ./cmd/gen-docs` (deve essere un no-op) verdi. Serve **golangci-lint v2** (config schema v2): `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`, stessa versione pinnata in CI. **Va reinstallato dopo ogni major del Go locale**: un binario compilato con go1.25 va in panic caricando la stdlib di go1.26 (`file requires newer Go version`). Per lo stesso motivo la CI lo compila da sorgente con il toolchain del job invece di scaricare il binario pre-compilato.
- **Ogni regola nuova**: entra in `internal/rules/<categoria>.go`, con `ID` stabile `categoria/nome`, `Title`, `Rationale` (spiega il rischio, non ripete il titolo) e `Eval` funzione pura; test nel file `_test.go` della categoria; `go run ./cmd/gen-docs` per rigenerare `docs/rules.md`; voce nel `CHANGELOG.md`.
- **Gli id delle regole sono API**: chi mette il tool in CI pinna un id in `--skip` o in una suppression. Non rinominarli senza una major e una nota nel changelog.
- **Exit code semantics**: 0 anche con finding WARN/BAD (l'audit che gira È un successo); ≠0 solo per errori sistemici (sorgente illeggibile, credenziali che non funzionano, regola sconosciuta, flag errato). `--exit-on` per il gating CI. NON cambiare questa semantica.
- **Niente segreti** in fixture, test, doc o output. Le fixture usano `example-value-not-a-real-secret`, e c'è un test che verifica che nessun finding lo riporti.
- **Todo → `BACKLOG.md`** (sorgente unica, id stabili `KD-n`). Non sparpagliare TODO nei commenti. Il file è **machine-read**: `go run ./cmd/backlog check` lo valida e il workflow `Backlog` lo specchia in milestone/issue GitHub a ogni push su `main`. Va rispettata la forma dei bullet (`- **KD-n** — descrizione` sotto `## Open`/`## Done`, `- **vX.Y.Z** [(due YYYY-MM-DD)] — scopo. Items: KD-a, KD-b.` sotto `## Milestones`), e si modifica il file, non le issue: titolo, body, label e milestone di una issue sincronizzata vengono riscritti al sync successivo, e spostare un item sotto `## Done` è ciò che chiude la sua issue.
- **Documentare tutto, nello stesso commit**: ogni novità (regola, tool, workflow, asset) entra insieme alla sua documentazione — voce nel `CHANGELOG.md`, il documento in `docs/` o la sezione di README che la riguarda, i commenti che spiegano il *perché*, e i puntatori qui sotto. Mai "lo documento dopo": in questo repo la documentazione è una proiezione della cosa (`docs/rules.md` dal catalogo, il sito dai `.md`, le issue dal `BACKLOG.md`), e una doc scritta dopo descrive un tool che si è già mosso.
- **Lingua = inglese**: codice, commenti, test e tutto l'output user-facing (messaggi dei finding, `usage`, help dei flag, errori, README, docs). **Eccezione: il `CHANGELOG.md` resta in italiano.**

## Comandi

```bash
go build -o keycloak-doctor ./cmd/keycloak-doctor
go test ./...                 # tutto locale: fixture in testdata/ e httptest per l'Admin API
go vet ./...
golangci-lint run
go run ./cmd/gen-docs         # rigenera docs/rules.md dal catalogo (gate in CI)
go run ./cmd/backlog check    # valida BACKLOG.md (gate nel workflow Backlog)
go run ./cmd/gen-site         # genera il sito in ./site (gate in CI, deploy su Pages)

docker build --build-arg VERSION=x.y.z-local -t keycloak-doctor:local .   # ~6 MB, FROM scratch, uid 65532
docker run --rm -v "$PWD/testdata:/realm:ro" keycloak-doctor:local audit /realm/insecure-realm.json --no-color
brew style Formula/keycloak-doctor.rb   # lint della formula (brew fetch/install richiede un tap, non un path)

# Verifica visiva del sito (Chrome headless, niente dipendenze): screenshot + errori JS
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless --disable-gpu --hide-scrollbars --screenshot=out.png --window-size=1280,1400 "file://$PWD/site/index.html"
(cd site && python3 -m http.server 8731 &) # serve su localhost: secure context, quindi il JS gira tutto
"$CHROME" --headless --disable-gpu --enable-logging=stderr --v=0 --dump-dom http://localhost:8731/rules.html 2>&1 >/dev/null | grep CONSOLE
go run ./cmd/backlog sync --repo Allan-Nava/keycloak-doctor --dry-run  # cosa cambierebbe nel tracker

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
- `internal/site/` + `cmd/gen-site/` — sito della documentazione: renderer di un **subset** di Markdown (heading, paragrafi, code fence, tabelle, liste annidate, reference link, span inline) più layout/asset in `internal/site/assets/` via `embed`. Le pagine sono una proiezione di README/BACKLOG/SECURITY/COMMERCIAL/CHANGELOG e del catalogo compilato; il renderer è l'unico escaper tra un `.md` e la pagina servita, e c'è un test che lo verifica.
- `internal/backlog/` + `cmd/backlog/` — parser di `BACKLOG.md` e mirror idempotente su milestone/issue GitHub (stdlib, `httptest` nei test). Niente a che vedere con l'audit: è il plumbing del progetto, non entra nel binario `keycloak-doctor`.

## Trappole note / regole tecniche

- **`Attrs` coerce gli scalari a stringa di proposito**: gli export Keycloak non sono coerenti sui tipi degli attributi (una `"saml.server.signature": true` accanto a `"pkce.code.challenge.method": "S256"`). Un `map[string]string` fa fallire l'unmarshal dell'intero realm su un dettaglio di formato. Non "semplificarlo".
- **ERROR non è BAD**: ERROR significa "la regola non ha potuto essere valutata" (sezione non leggibile), non "target malato". Una regola su una sezione mancante deve riportare ERROR, mai OK: un punto cieco non può renderizzare come esito pulito.
- **Una regola che passa emette UN solo finding aggregato per realm**, non uno per oggetto: su un realm con 200 client il verde altrimenti sommerge i finding. C'è un test che lo verifica su tutto il catalogo.
- **Flag posizionale**: `flag` della stdlib si ferma al primo argomento non-flag, quindi il path posizionale di `audit` viene estratto **prima** del parse. Aggiungendo sottocomandi, replicare quel pattern.
- **I client disabilitati non producono finding** (non possono ottenere un token): filtrati in `scanClients`. Lo stesso per gli identity provider disabilitati e le esecuzioni `DISABLED` nei flow.
- **Le due sorgenti danno forme diverse per i flow** (export: liste per flow con `flowAlias`; API: lista già appiattita con `providerId`): `reachableAuthenticators` gestisce entrambe, con visited set contro i cicli.
- **`--insecure` esiste solo per i lab con chain self-signed** e va usato per il server Keycloak, non per silenziare un problema di certificati in produzione.
- **Il sito va guardato, non solo generato**: `qlmanage -t` per gli SVG del logo e Chrome headless per le pagine. Due trappole del *solo* controllo da `file://`: (1) `navigator.clipboard` non esiste fuori da un secure context, quindi i pulsanti "Copy" non compaiono — serve `http://localhost`; (2) Chrome headless ha un **floor di viewport a 500px**, così uno screenshot a `--window-size=390` è un crop di un layout a 500px e sembra tagliato anche quando non lo è. Per misurare davvero l'overflow: iniettare uno script che confronta `documentElement.scrollWidth` con `clientWidth`.
- **`rootMargin` di `IntersectionObserver` accetta solo px o %**, mai `rem`: con `rem` il costruttore lancia. Nel sito le tre feature JS (filtro, TOC attivo, copy) girano ognuna nel suo `try`, proprio perché un'eccezione in una non deve spegnere le altre.
- **L'immagine è `FROM scratch` e deve restarlo**: nessuna shell, nessun package manager, uid `65532`, filesystem non scrivibile. Il bundle di CA viene copiato dallo stage di build perché senza quello `--url https://…` non parla con nessun server; il workflow verifica utente non-root e assenza di shell, oltre a distinguere le due fixture.
- **Homebrew: niente `homebrew-core`** — accetta solo licenze OSI, e PolyForm Noncommercial non lo è (`license :cannot_represent` nella formula). Il tap è questo repo, quindi il `brew tap` **richiede l'URL** (il repo non si chiama `homebrew-*`). La formula usa `url … tag:/revision:` (git, non tarball) così non c'è nessuno sha256 da ricalcolare, e Homebrew rifiuta le formule fuori da un tap: in locale si può solo fare `brew style`, oppure `brew tap <user>/<name> file:///path/al/repo` per provarla davvero.
- Il campo `version` è iniettato dalla CI sui tag: non hardcodarlo.

## Puntatori

- Backlog: `BACKLOG.md` · CI: `.github/workflows/ci.yml` · Release: `.github/workflows/release.yml` · Backlog sync: `.github/workflows/backlog.yml` · Pages: `.github/workflows/pages.yml`
- Sito: <https://allan-nava.github.io/keycloak-doctor/>
- Catalogo regole generato: `docs/rules.md` · Fixture: `testdata/`
- Distribuzione: `Dockerfile` (+ `.dockerignore`, workflow `docker.yml` → `ghcr.io/allan-nava/keycloak-doctor`) · `Formula/keycloak-doctor.rb` (il tap **è** questo repo; il job `homebrew` di `release.yml` sposta `tag`/`revision` sull'ultima release)
- Brand: `docs/brand.md` · Logo: `docs/assets/logo.svg` + `docs/assets/favicon.svg` — unica copia, li leggono sia il README sia `cmd/gen-site` (che li serve accanto alle pagine). Il segno è shield + keyhole + pulsazione: **niente checkmark**, il tool non dà verdetti.
- Repo affini (stessa famiglia e stesse convenzioni): `~/projects/github.com/checkfleet`, `segcheck`, `nomad-lens`, `nats-lens`, `ansible-vars-lens`
