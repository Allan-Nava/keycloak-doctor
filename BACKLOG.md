# Backlog

Single source of truth for the work on keycloak-doctor. Items have a stable id (`KD-n`) so a commit, a branch or a finding can reference one. Do not scatter TODO comments in the code.

This file is machine-read. `go run ./cmd/backlog check` validates it and the [Backlog workflow](.github/workflows/backlog.yml) mirrors it into GitHub milestones and issues on every push to `main`, so keep the shape of the bullets:

- an item is `- **KD-n** — description`, under `## Open` (grouped by a `###` heading) or under `## Done`;
- a milestone is `- **vX.Y.Z** [(due YYYY-MM-DD)] — what the release is about. Items: KD-a, KD-b.`, under `## Milestones`.

Edit this file, not the issues: a synced issue's title, body, labels and milestone are overwritten on the next run, and moving an item to `## Done` is what closes its issue.

## Milestones

- **v0.2.0** — Pull-request gating: make the audit usable as a required check on the repository that holds the realm definition, not only as a report an operator reads in a terminal. Items: KD-10, KD-11, KD-12, KD-13.

## Open

### Rules

- **KD-1** — `client/service-account-roles`: read the service account's role mappings from the Admin API and report a service account holding `realm-admin`, `manage-users` or `manage-clients`. Needs `/clients/{id}/service-account-user/role-mappings`; report `ERROR — not evaluated` on a realm export, which does not carry them.
- **KD-2** — `realm/default-roles`: report a default role composite that grants more than `offline_access` and `uma_authorization` — every new user inherits it silently.
- **KD-3** — `auth/direct-grant-flow`: audit the bound direct grant flow the way `realm/browser-mfa` audits the browser flow (a direct grant flow with no OTP step is how MFA gets bypassed while looking enabled).
- **KD-4** — `client/consent`: consent not required on a client that requests scopes beyond the profile, when the realm brokers third-party identities.
- **KD-5** — `keys/active-rotation`: report a signing key provider that has been active for longer than a configurable window, and a passive provider still published in the JWKS long after the tokens it signed expired.
- **KD-6** — `realm/webauthn-policy`: report a WebAuthn policy with user verification off or attestation unchecked, when a WebAuthn authenticator is reachable from a bound flow.
- **KD-7** — `client/saml-signatures`: for SAML clients, report assertions or documents not signed and signature validation switched off.

### Sources and integration

- **KD-8** — Kubernetes/CRD source: read realms from `KeycloakRealmImport` custom resources so the audit runs on the desired state in git rather than on the live server.
- **KD-9** — Terraform source: audit `keycloak_realm` / `keycloak_openid_client` resources from a Terraform plan JSON, for a pre-apply gate.
- **KD-10** — SARIF output, so findings land in the GitHub code scanning tab of the repo that holds the realm definition (checkfleet already has a renderer worth mirroring).
- **KD-11** — GitHub Action (`action.yml`) wrapping `audit --output sarif --exit-on bad`, plus a documented workflow for auditing an export committed to a repo.
- **KD-12** — Baseline file: `--baseline audit.json --fail-on-new`, so a realm with known accepted findings still gates on regressions.
- **KD-13** — Suppression file with an expiry date per rule/target, and a rule that reports suppressions past their expiry.

### Quality of report

- **KD-14** — Per-rule severity override from a config file (`--config`), for the thresholds that are genuinely site policy: token lifespans, session lifespans, password length.
- **KD-15** — `--explain RULE`: print the rationale, the threshold applied and the exact admin console path, for the rule an operator is arguing with.
- **KD-16** — Diff mode: `audit --against previous.json` to show what changed in a realm's posture between two exports.

### Project plumbing

- **KD-18** — Mirror the backlog automation into the sibling tools (`checkfleet`, `segcheck`, `nomad-lens`, `nats-lens`, `ansible-vars-lens`), which share these conventions and the same `BACKLOG.md` shape.

## Done

- **KD-19** — v0.1.3: logo (shield, keyhole, pulsazione) in `docs/assets/`, brand guidelines in `docs/brand.md`, mark e favicon serviti dal sito e in testa al README.
- **KD-17** — v0.1.2: documentation site on GitHub Pages (`internal/site`, `cmd/gen-site`, `.github/workflows/pages.yml`), generated from the Markdown documents and from the compiled rule catalogue, with the reference filterable in the browser.
- **KD-0** — v0.1.0: engine, both sources, credential scrubbing, 30 rules, three output formats, exit-code semantics, generated rule docs, full local test suite.
