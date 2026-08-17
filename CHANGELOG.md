# Changelog

Tutte le modifiche rilevanti a questo progetto sono documentate qui.
Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e il progetto usa il [Semantic Versioning](https://semver.org/lang/it/).

## [0.1.4] - 2026-08-17

Restyling del sito della documentazione. Il binario `keycloak-doctor` è identico alla 0.1.0.

### Aggiunto

- **Hero sulla home**: mark, nome, tagline e tre azioni (`Browse the 30 rules`, `Install`, `Audit an export`) su una banda con i colori del brand. Il conteggio delle regole nel bottone viene dal catalogo compilato, non da una stringa. Le pagine senza hero tengono il titolo normale — e un test verifica che l'`<h1>` non compaia due volte.
- **Reference a card in griglia**: le regole diventano una griglia responsive con il bordo accentato, l'id in monospace, hover sollevato e `:target` evidenziato quando si arriva da un link diretto. Le severità sono pill colorate, il campo di filtro è sticky sotto l'header e mostra `n of 30 rules` mentre si scrive.
- **Sommario laterale attivo**: la voce della sezione che si sta leggendo si evidenzia (`IntersectionObserver`), e `scroll-padding-top` fa sì che un'anchor non finisca sotto l'header sticky.
- **Pulsanti "Copy"** sui blocchi di codice, con le `$ ` dei prompt rimosse dal testo copiato: un transcript si incolla per eseguirlo.
- **Restyling generale**: palette di interfaccia ripensata per chiaro e scuro (con `color-scheme` dichiarato), header sticky con blur e sottolineatura della pagina corrente, tabelle in un contenitore con bordo e hover di riga, code block con scrollbar sottile e font leggermente più stretto, `:focus-visible` visibile su tutto, e rispetto di `prefers-reduced-motion`. Layout più largo (74rem) e colonna di lettura limitata a 54rem.

### Corretto

- **Autolink**: `<https://…>` nel Markdown viene reso come link — prima finiva in pagina come testo con le parentesi angolari escapate (si vedeva nel README pubblicato).
- **`IntersectionObserver` con `rootMargin` in `rem`**: il costruttore lancia, e l'eccezione spegneva anche i pulsanti "Copy". Ora il margine è in px e le tre feature JS girano ognuna nel suo `try`, così una che non parte non porta giù le altre. Trovato guardando la console di Chrome headless, non rileggendo il codice.
- **Overflow orizzontale del hero su schermo stretto**: il `flex-basis` del blocco di testo era più larga di un telefono. Documentata in `CLAUDE.md` anche la trappola che l'aveva mascherata (headless ha un floor di viewport a 500px, quindi uno screenshot a 390px è un crop).

## [0.1.3] - 2026-08-17

Identità visiva. Il binario `keycloak-doctor` è identico alla 0.1.0.

### Aggiunto

- **Logo** (`docs/assets/logo.svg`) e **favicon** (`docs/assets/favicon.svg`): shield (a cosa serve il tool), keyhole (di chi è la configurazione che legge) e una pulsazione che lo attraversa (cosa produce: una diagnosi con razionale e fix). **Niente checkmark**: una spunta promette un verdetto che il tool non dà — `OK` significa "la regola è girata ed è passata", mai "questo realm è sicuro". SVG scritti a mano, nessun riferimento esterno, nessun font, nessun raster: dal favicon alla slide, poche centinaia di byte. La variante favicon è lo stesso segno con il dettaglio che una tab da 16px non risolve tolto via.
- **Brand guidelines** (`docs/brand.md`, pagina *Brand* del sito): il nome (`keycloak-doctor`, minuscolo, monospace, e non è un prodotto Keycloak/Red Hat), il significato dei tre elementi, i tre colori con i loro hex, cosa fare e cosa non fare, e come rigenerare un'anteprima.
- **Mark nel sito e nel README**: `cmd/gen-site` copia i due file accanto alle pagine (`Site.Files`), il layout li usa come `<link rel="icon">` e nell'header di ogni pagina, e il README apre con lo stesso file. Una sola copia del segno: non esiste un secondo posto da aggiornare. Test: ogni pagina generata ha favicon e mark, un nome di file con un path dentro viene rifiutato, e il blocco HTML iniziale del README non finisce nel corpo della pagina.
- Chiude **KD-19**.

## [0.1.2] - 2026-08-17

Sito della documentazione su GitHub Pages. Il binario `keycloak-doctor` è identico alla 0.1.0.

### Aggiunto

- **Sito della documentazione** (`internal/site`, `cmd/gen-site`, workflow `Pages`): <https://allan-nava.github.io/keycloak-doctor/>, rigenerato e ripubblicato a ogni push su `main`. Chiude **KD-17**.
- **Ogni pagina è una proiezione del repo**: overview da `README.md`, roadmap da `BACKLOG.md`, security, uso commerciale, changelog — e la **rule reference generata dal catalogo compilato nel binario**, non da una copia a mano che finirebbe per descrivere regole che il tool non ha più. Un link relativo a un `.md` viene riscritto sulla pagina che lo serve; ogni altro link relativo (LICENSE, un workflow, un file sorgente) punta al repo su GitHub.
- **Renderer Markdown proprio** (`internal/site/markdown.go`), solo stdlib: heading con anchor, paragrafi, code fence, tabelle (che scrollano nel loro box), liste annidate con continuazioni, thematic break, link reference (`[0.1.1]` in fondo al changelog diventa un link alla release) e span inline, incluso il bold che avvolge un code span. Il subset è un tetto deliberato: l'input è la nostra documentazione, e così il sito resta un `go run` senza toolchain. Il renderer è l'unico escaper tra un `.md` e la pagina servita — c'è un test che prova a farci passare `<script>` da paragrafo, heading, lista, tabella, code fence e link.
- **Reference filtrabile**: campo di ricerca su id, titolo e razionale delle 30 regole, con le categorie vuote che si nascondono. È l'unico script del sito, la pagina funziona anche senza. Layout responsive, tema chiaro/scuro secondo `prefers-color-scheme`, nessun font remoto, nessuna dipendenza: un CSS e uno JS serviti dallo stesso host.
- **Gate in CI**: `go run ./cmd/gen-site` gira su ogni push e pull request, così una modifica che rompe il generatore fallisce in CI e non sul deploy di Pages.

## [0.1.1] - 2026-08-17

Solo plumbing del progetto: il binario `keycloak-doctor` è identico alla 0.1.0.

### Aggiunto

- **Milestone `v0.2.0`** — «Pull-request gating»: rendere l'audit usabile come required check sul repo che contiene la definizione del realm, non solo come report da leggere in terminale. Raccoglie KD-10 (output SARIF), KD-11 (GitHub Action), KD-12 (baseline file), KD-13 (suppression con scadenza). Le milestone si dichiarano nella nuova sezione `## Milestones` di `BACKLOG.md`.
- **`BACKLOG.md` diventa machine-read** (`internal/backlog`, `cmd/backlog`): parser degli item (`- **KD-n** — descrizione` sotto `## Open`/`## Done`, con l'heading `###` come area) e delle milestone (`- **vX.Y.Z** [(due YYYY-MM-DD)] — scopo. Items: KD-a, KD-b.`). Un bullet che non rispetta la forma è un errore, non un item saltato in silenzio: un mirror che perde un item è peggio di un mirror che si rompe.
- **`backlog check`** — valida il file: id duplicati, item dichiarato sia open sia done, milestone che rivendica un id inesistente, item rivendicato da due milestone, milestone senza item. Problemi `error` (falliscono il gate) e `warn` (item senza area, milestone che rivendica un item già chiuso), ordinati worst-first, emessi come annotation `::error file=...,line=...` quando gira in Actions.
- **`backlog sync`** — mirror idempotente su GitHub: crea le milestone dichiarate, apre una issue per ogni item aperto (titolo `KD-n: …`, body con la descrizione e il puntatore al file, label `backlog` + `area/<slug>`), riallinea titolo/body/milestone/label di una issue modificata a mano e chiude la issue di un item spostato sotto `## Done`. Non cancella niente e non tocca le issue senza item corrispondente: le segnala come `orphan-issue`. `--dry-run` esegue solo GET. Le label aggiunte a mano su una issue vengono conservate.
- **`backlog export`** — il file parsato come JSON (item, milestone, assegnazioni), per chi vuole costruirci sopra.
- **Workflow `Backlog`** (`.github/workflows/backlog.yml`) — `check` su ogni pull request che tocca il backlog (con annotation sul diff) e sync su ogni push su `main`; su pull request e su `workflow_dispatch` gira in dry-run, con il piano nel job summary. `concurrency` serializza i run: due sync in parallelo creerebbero ognuno le issue che l'altro non ha ancora visto.
- **Test**: parser, validazione e riconciliazione completa contro un finto GitHub `httptest` (creazione, idempotenza al secondo giro, chiusura di un item finito, riparazione di una issue modificata a mano, dry-run che non emette una sola chiamata mutante, propagazione degli errori API senza mai stampare il token). Un test verifica che il `BACKLOG.md` committato parsi e sia consistente, come il gate su `docs/rules.md`.

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

[0.1.4]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.4
[0.1.3]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.3
[0.1.2]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.2
[0.1.1]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.1
[0.1.0]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.0
