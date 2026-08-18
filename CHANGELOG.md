# Changelog

Tutte le modifiche rilevanti a questo progetto sono documentate qui.
Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/) e il progetto usa il [Semantic Versioning](https://semver.org/lang/it/).

## [0.2.0] - 2026-08-17

Milestone **v0.2.0 «Pull-request gating»** completata: l'audit diventa un required check sul repo che contiene la definizione del realm, non solo un report da leggere in terminale. Chiude KD-10, KD-11, KD-12, KD-13.

### Aggiunto

- **Output SARIF 2.1.0** (`--output sarif`, KD-10): i finding diventano alert di GitHub code scanning accanto al file del realm. Tre scelte di mappatura, tutte deliberate: `BAD` → `error` e `WARN` → `warning` (il livello è **per finding**, non per regola — la stessa regola è BAD su un client pubblico e WARN su uno confidenziale); un finding **`ERROR` è un result vero a livello `note`**, non una regola saltata, perché un punto cieco deve stare nella stessa lista dei finding; una `OK` è un result `kind: pass` a livello `none`, così il file resta una proiezione fedele del run senza generare alert. Il `partialFingerprints` è calcolato su rule+realm+target e **mai sul messaggio**: è ciò che tiene aperto lo stesso alert tra run invece di chiuderlo e riaprirlo quando cambia un conteggio. Se la sorgente è un file del repo il result è ancorato ad esso; se è un server live non c'è nessun file e resta la sola posizione logica `realm/target`, invece di inventarne una.
- **GitHub Action** (`action.yml`, KD-11): scarica il binario di release per il runner e **lo verifica con il `checksums.txt`** della release — non compila dal sorgente e non tira l'immagine, così lo step costa circa un secondo ed esegue lo stesso artefatto che scaricherebbe una persona. 13 input (`path`, `output`, `out-file`, `exit-on`, `min-severity`, `only`, `skip`, `realm`, `baseline`, `fail-on-new`, `suppress`, `summary`, `version`), output `report`/`worst`/`suppressed`/`exit-code`, report Markdown nel job summary. `version: path` usa un `keycloak-doctor` già sul PATH: è l'escape hatch per chi se lo installa da sé, ed è come la CI testa l'action **sul working tree** invece che sull'ultima release (job `action` in `ci.yml`, che verifica anche che l'action gati sulla fixture rotta e passi su quella hardened).
- **Baseline** (`--baseline audit.json`, `--fail-on-new`, KD-12): un realm che ha accumulato finding non si sistema in una pull request, e un check che falisce sempre è un check che si impara a ignorare. `--baseline` rilegge il JSON di un run precedente e marca ciò che non c'era (`NEW ·` nel testo, conteggio in Markdown, `"new": true` in JSON e in SARIF); `--fail-on-new` restringe `--exit-on` a quei finding. Il match è su **rule+realm+target**, non sul messaggio (che porta conteggi e lifespan che si muovono senza che si muova la postura), e uno stato **peggiorato** conta come nuovo: un WARN diventato BAD è una regressione, non un problema noto.
- **File di suppression** (`--suppress`, KD-13): i finding che si è deciso di accettare, ognuno con `until` (data) e `reason` **obbligatori**. Due proprietà lo rendono usabile in pipeline: una suppression non è mai **silenziosa** (il run stampa `· 2 suppressed`, ed è nel JSON e nel SARIF) e non è mai **permanente** (dopo la data smette di sopprimere e il finding torna). Il file riporta anche se stesso: `suppression/expired` dice quale entry è scaduta e con che motivazione era stata scritta, `suppression/unmatched` quale non ha corrisposto a niente — o il finding è risolto e l'entry è morta, o l'id/realm/target è un typo e qualcuno crede di aver soppresso qualcosa che invece viene ancora riportato. Una chiave sconosciuta nel file è un errore, non un no-op: un `targt` scritto male lascerebbe un'entry che sopprime più del previsto. Le due id **non sono regole del catalogo** (non sono funzioni del realm): `--only`/`--skip` non le selezionano.
- **`docs/ci.md`** (pagina *In CI* del sito): exit code, l'action con la tabella degli input, la mappatura SARIF, come il baseline confronta un finding e il formato del file di suppression. Sezione «In CI» del README riscritta di conseguenza.

### Corretto

- **`MarkNew` non registrava le `OK` del baseline**: la mappa teneva «il peggiore per chiave» con un confronto di severità, e `severity[OK]` è 0 come lo zero value — così ogni finding OK risultava nuovo. «Non presente» e «presente e passata» sono affermazioni diverse. Trovato eseguendo il tool sul suo stesso baseline, non rileggendo il codice.
- **Il conteggio dei soppressi non arrivava al JSON**: era nel testo e nel Markdown ma non in `jsonResult`, quindi una pipeline che gata su `worst` non poteva vedere che dei finding erano stati rimossi dal run. Preso da un test.
- **Il sync staccava le issue da una milestone rilasciata**: `BACKLOG.md` elenca ciò che è *pianificato*, quindi quando una milestone esce dal file (perché è stata rilasciata) le sue issue non avevano più una milestone dichiarata e il sync la rimuoveva — cancellando il registro di cosa era stato rilasciato in quella release. Ora il sync gestisce l'appartenenza **solo delle milestone che il file dichiara**: una milestone che il file non menziona più è una release, e le sue issue chiuse ne sono la prova. Preso in faccia applicando questa stessa release (le 4 issue sono state riattaccate a mano), con due test nuovi: uno sulla milestone rilasciata, uno sullo spostamento di un item tra due milestone dichiarate, che deve continuare a funzionare.
- **Gli output dell'action dicevano `worst=OK` quando l'audit non era rileggibile**: ora dicono `UNKNOWN`. Un job che non è riuscito a guardare non è un job che non ha trovato niente — è la stessa regola di `ERROR` nei finding, applicata agli output di uno step.

### Nota

Ordine delle operazioni dentro un run, ora che sono tre livelli: audit → suppress → confronto col baseline → `--min-severity` → render → gate. Un finding soppresso non arriva al confronto col baseline (comparirebbe come risolto e poi di nuovo come nuovo il giorno della scadenza) e non può soddisfare un gate, perché non è più nel run.

## [0.1.7] - 2026-08-17

### Aggiunto

- **Milestone `v0.4.0`** — «Desired-state sources»: auditare il realm che git dice che dovresti avere, *prima* dell'apply, invece di quello che il server è già diventato — custom resource `KeycloakRealmImport` (KD-8) e plan JSON di Terraform (KD-9), decodificati nella stessa forma parziale del modello che export e Admin API producono già.
- **Milestone `v0.5.0`** — «Report ergonomics»: le parti del report su cui un operatore discute — soglie che sono davvero policy di sito spostate in un file di config (KD-14), `--explain RULE` con razionale e percorso esatto nella admin console (KD-15), e un diff tra due export per mostrare cosa ha fatto un cambio alla postura di un realm (KD-16).
- Ogni item aperto del backlog ha ora una milestone, tranne **KD-18** (mirror dell'automazione del backlog sui repo affini), che è plumbing e non appartiene a una release del tool.

## [0.1.6] - 2026-08-17

### Aggiunto

- **Milestone `v0.3.0`** — «Rule coverage»: le sette regole che un realm apparentemente hardened nasconde ancora — un service account con ruoli di amministrazione (KD-1), un default role composite che nessuno rilegge (KD-2), un direct grant flow che bypassa l'MFA imposto sul browser flow (KD-3), consent non richiesto su un realm che fa brokering di identità terze (KD-4), chiavi di firma che non ruotano mai (KD-5), una WebAuthn policy che non verifica niente (KD-6), asserzioni SAML non firmate (KD-7). Sono le regole il prodotto: questa milestone è la prossima dopo il gating (`v0.2.0`).

### Corretto

- **La CI compila golangci-lint da sorgente** invece di scaricare il binario pre-compilato: un linter compilato con go1.25 va in panic quando deve caricare la stdlib di go1.26 (`file requires newer Go version`). Con `setup-go: stable` quel guasto arriva da solo, su un run che non ha cambiato niente — è successo in locale appena Homebrew ha portato il Go a 1.26.6. Nota anche in `CLAUDE.md`: dopo un major del Go locale, `go install …/golangci-lint@v2.12.2` va rifatto.
- **Il dry-run del sync sotto-riportava il lavoro**: dichiarando una milestone nuova in `BACKLOG.md`, `backlog sync --dry-run` stampava solo `create-milestone` e non le issue che ci sarebbero finite dentro — perché in dry-run la milestone non ha ancora un numero con cui fare la PATCH. Ora quelle issue sono riportate come `milestone v0.3.0 (to be created)`: una preview che sotto-riporta è peggio di nessuna preview, e il principio è lo stesso dei finding `ERROR` (un punto cieco non può renderizzare come esito pulito). Test dedicato sul finto GitHub.

## [0.1.5] - 2026-08-17

Distribuzione: `docker run` e `brew install`. Il binario `keycloak-doctor` è identico alla 0.1.0.

### Aggiunto

- **Immagine Docker** su `ghcr.io/allan-nava/keycloak-doctor` (multi-arch amd64/arm64, tag `x.y.z`, `x.y` e `latest`):

  ```bash
  docker run --rm -v "$PWD:/realm:ro" ghcr.io/allan-nava/keycloak-doctor audit /realm/prod-realm.json
  ```

  ~6 MB, `FROM scratch`: il binario statico e il bundle di CA (senza cui `--url https://…` non parla con nessun server), e nient'altro — un'immagine a cui si dà un realm di produzione non ha motivo di portarsi dietro anche una shell e un package manager. Gira come uid `65532`, non serve filesystem scrivibile, e l'export si monta in sola lettura su `/realm`.
- **Smoke test dell'immagine prima del push** nel workflow `Docker`: `version`, `rules`, la fixture hardened che passa con `--exit-on warn` e quella insicura che *deve* gatare con `--exit-on bad`, più due controlli sull'immagine stessa (utente non-root, nessuna shell eseguibile). Su pull request si costruisce e si testa senza pubblicare.
- **Formula Homebrew** (`Formula/keycloak-doctor.rb`) — il tap **è** questo repo:

  ```bash
  brew tap Allan-Nava/keycloak-doctor https://github.com/Allan-Nava/keycloak-doctor
  brew install keycloak-doctor
  ```

  L'URL nel `brew tap` non è opzionale, perché il repo non si chiama `homebrew-*`. La formula compila dal sorgente al tag (`url … tag:/revision:`, quindi nessuno sha256 da ricalcolare a ogni release) e inietta la versione con gli stessi `-ldflags` dei binari di release, così `keycloak-doctor version` non dice `dev`. Il suo `test do` audita un realm minimo e verifica che `realm/ssl-required` compaia e che l'exit code resti 0.
- **Il job `homebrew` di `release.yml`** sposta `tag`/`revision` della formula sull'ultima release pubblicata e committa su `main`, serializzato e con un guardrail `sort -V` perché il push di più tag insieme avvia più run e la formula deve solo andare avanti. **Nota**: dopo un release la CI aggiunge un commit `chore:` su `main`, quindi serve un `git pull` prima del commit successivo.
- **`--exit-on` vale anche dentro il container**: la semantica degli exit code è quella documentata, l'immagine non la cambia.

### Nota sulla licenza

`homebrew-core` accetta solo licenze approvate OSI, e PolyForm Noncommercial non lo è: la formula dichiara `license :cannot_represent` e vive in questo tap. Lo stesso vale per l'immagine, che porta `org.opencontainers.image.licenses=LicenseRef-PolyForm-Noncommercial-1.0.0`.

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

[0.2.0]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.2.0
[0.1.7]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.7
[0.1.6]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.6
[0.1.5]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.5
[0.1.4]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.4
[0.1.3]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.3
[0.1.2]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.2
[0.1.1]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.1
[0.1.0]: https://github.com/Allan-Nava/keycloak-doctor/releases/tag/v0.1.0
