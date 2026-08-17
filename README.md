# keycloak-doctor

**Audit a Keycloak realm for the mistakes that actually get exploited — from an export file or a live server, in one static Go binary.**

Keycloak gives you every switch you need to be secure, and no opinion about how they should be set. A realm accumulates a wildcard redirect URI here, a public client with the password grant there, brute force detection nobody re-enabled after a load test, a 1024-bit signing key inherited from a 2019 migration. Nothing warns you: each of those is a valid configuration.

`keycloak-doctor` reads the realm and reports the ones that matter, with the reason and the fix:

```console
$ keycloak-doctor audit realm-export.json --min-severity warn
keycloak-doctor 0.1.0 · 1 realm · 30 rule(s) · file:realm-export.json · 1ms

BAD    client/redirect-wildcard        demo · legacy-frontend   redirect URI "https://*" matches hosts the client does not control
       → list the exact callback URLs instead; Keycloak matches them literally
BAD    client/pkce                     demo · legacy-frontend   a public client runs the code flow without requiring PKCE
       → set the PKCE method to S256 in the client's Advanced settings
BAD    keys/rsa-size                   demo · legacy-rsa        key provider "legacy-rsa" signs with a 1024-bit RSA key
       → create a 2048-bit (or larger) provider, make it active, and keep the old one passive only until the tokens it signed expire
BAD    realm/brute-force               demo                     brute force detection is disabled: password guessing is unthrottled
       → enable Brute force detection in Realm settings › Security defenses
WARN   realm/browser-mfa               demo                     the bound browser flow "browser" has no OTP or WebAuthn step: a password is the only factor
       → add a Conditional OTP or WebAuthn subflow to the browser flow

17 BAD · 13 WARN — worst: BAD
```

It is **not** a scanner and not a policy engine: no agents, no server, no CRDs, no cluster. One binary, two inputs, three output formats.

## What it checks

30 rules in 7 categories. The full catalogue with the rationale for each is in [docs/rules.md](docs/rules.md), and it also lives in the binary:

```console
$ keycloak-doctor rules --only client
```

| Category | Rules | Examples |
|---|---|---|
| `realm` | 13 | brute force off, unrevocable 2h access tokens, offline tokens that never expire, no password policy, no second factor in the bound browser flow, events not recorded |
| `client` | 8 | wildcard and plaintext redirect URIs, `*` web origins, implicit flow, password grant on a public client, missing PKCE, full scope on a service account, hidden token-lifespan overrides |
| `mapper` | 2 | a mapper copying `api_key`/`password_hash` into a token claim, an audience mapper pointing at an API the client does not own |
| `idp` | 2 | broker trusted for unverified email while login-by-email is on, broker endpoints over plaintext HTTP |
| `keys` | 2 | RSA signing keys under 2048 bits, HMAC secrets under 32 bytes |
| `federation` | 2 | LDAP over `ldap://` without StartTLS, LDAP TLS with certificate verification off |
| `source` | 1 | how much credential material the audited source itself carries |

Severity means what it says: **BAD** is exploitable as configured, **WARN** is a weakening you should be able to justify, **OK** is a rule that ran and passed, and **ERROR** is a rule that *could not run* — a section the credentials were not allowed to read. A blind spot never renders as a clean bill.

## Install

```bash
go install github.com/Allan-Nava/keycloak-doctor/cmd/keycloak-doctor@latest
```

Or build from a checkout:

```bash
go build -o keycloak-doctor ./cmd/keycloak-doctor
```

## Use it

### Offline, against an export (no credentials involved)

```bash
kc.sh export --realm prod --file prod-realm.json     # or the admin console's Partial export
keycloak-doctor audit prod-realm.json
keycloak-doctor audit ./export-dir --realm prod      # a directory export works too
```

A realm export carries plaintext client secrets, LDAP bind credentials and broker secrets. `keycloak-doctor` drops every credential **value** at load time and keeps only the fact that one was there — nothing downstream (a finding, the JSON output, an error message) can print one. The `source/secret-material` rule tells you how much the file you just audited is worth to an attacker, which is usually the reminder people need before attaching it to a ticket.

### Live, against a running server

```bash
export KC_AUDIT_SECRET=...      # never passed as a flag value
keycloak-doctor audit --url https://sso.example.com --realm prod \
  --client-id keycloak-doctor --client-secret-env KC_AUDIT_SECRET
```

`--all-realms` audits every realm the credentials can see. A password grant works too (`--username admin --password-env KC_ADMIN_PASSWORD`, against `admin-cli` by default).

**Least privilege**: create a confidential client with a service account and give it the read-only `realm-management` roles `view-realm`, `view-clients` and `view-identity-providers` for the realm you are auditing. Nothing the tool does needs write access. If a section is still out of reach, the rules over it report `ERROR — not evaluated` instead of passing.

### In CI

```bash
keycloak-doctor audit prod-realm.json --output json --exit-on bad --out-file audit.json
```

Exit codes follow the same rule as the rest of the family: **0 even when there are WARN/BAD findings** — an audit that ran is a success, and the report is the deliverable. Non-zero only for systemic errors (unreadable source, credentials that do not work, unknown rule, bad flag). Pass `--exit-on warn|bad|error` when you want a gate, with `--exit-code N` to pick the code.

Useful flags: `--output text|markdown|json`, `--min-severity warn`, `--only client,keys`, `--skip realm/audit-events`, `--out-file PATH`, `--no-color`.

The `markdown` output is shaped for a report or a PR comment: summary, a *Needs attention* list, then the full table. The `json` output is the gating contract — `worst`, `summary` and one object per finding with its stable rule id.

## Design notes

- **Two sources, one model.** `internal/keycloak` decodes a realm export and the Admin REST API into the same partial model of `RealmRepresentation`, so a rule is written once and never knows where the realm came from.
- **Rules are pure functions.** `internal/rules` has no IO: every rule takes a realm and returns findings, which is why the whole catalogue is tested against fixtures and why the audit needs no network. No test in this repo touches a real Keycloak.
- **One finding per offender, one OK per rule.** A rule that finds nothing emits a single aggregate OK for the realm rather than one per client — the difference between a readable report and a wall of green on a realm with 200 clients.
- **Credentials never enter the model.** They are dropped by the loader, and secrets are named by environment variable rather than passed as flag values, so they stay out of shell history and process listings.
- **Zero dependencies.** Standard library only.

## License

Source-available under the [PolyForm Noncommercial License 1.0.0](LICENSE): free for personal projects, research, education, non-profits and public institutions. For commercial use — auditing the realms of a company, or embedding this in a product or service — see [COMMERCIAL.md](COMMERCIAL.md).

## Related

Part of a family of domain-specific operations tooling: [checkfleet](https://github.com/Allan-Nava/checkfleet) (infrastructure health checks), [segcheck](https://github.com/Allan-Nava/segcheck) (HLS/DASH segment truth), [nomad-lens](https://github.com/Allan-Nava/nomad-lens) · [nats-lens](https://github.com/Allan-Nava/nats-lens) · [ansible-vars-lens](https://github.com/Allan-Nava/ansible-vars-lens) (VS Code).
