# Security policy

## Reporting a vulnerability

**Use [GitHub's private vulnerability reporting](https://github.com/Allan-Nava/keycloak-doctor/security/advisories/new)** — the *Report a vulnerability* button in the repository's Security tab. The report stays private until a fix is available.

Please do **not** open a public issue for a vulnerability.

What helps, in rough order of usefulness:

- the version (`keycloak-doctor version`) and how it was installed
- a minimal realm fixture and command that reproduces it — **with secrets removed**
- what an attacker gets out of it

**Redact before you send.** A Keycloak realm export is credential material: it carries client secrets, LDAP bind credentials and broker secrets in plaintext. Never attach a real export to a report. Reduce it to the few fields that trigger the bug and replace every secret, hostname and URL with a placeholder — a vulnerability report must not be the thing that leaks your identity provider.

## What to expect

keycloak-doctor is maintained by one person, so this is a best-effort commitment rather than an SLA:

| Stage | Target |
|---|---|
| Acknowledgement | within 5 working days |
| Assessment and severity | within 10 working days |
| Fix for a confirmed high-severity issue | in the next release, as a patch on the current minor |

Reports are credited in the release notes and in the advisory unless you ask otherwise. There is no bug bounty.

## Supported versions

Only the **latest release** receives security fixes. Backports to older tags are not provided — keycloak-doctor is a single static binary, so upgrading is replacing one file.

## Scope

In scope — a defect in keycloak-doctor itself:

- **credential leakage**: a client secret, an LDAP bind credential, a broker secret or an admin token appearing in a finding, in any output format, in an `--out-file`, or in an error message. The loader drops every credential value by design (`Scrub` in `internal/keycloak`), and *never printing one* is a design rule rather than a preference — a leak here is a real vulnerability even at low severity.
- **a wrong OK**: a rule that reports OK on a realm that does not satisfy it, or an unreadable section rendering as a pass instead of `ERROR — not evaluated`. An audit tool that reassures you falsely is worse than no audit tool.
- **path traversal or arbitrary file access** through the source path or `--out-file`.
- **crash, hang, or unbounded memory** from a malformed realm export or a hostile Admin API response — both are untrusted input.
- **credentials reaching the process argument list or the environment of a child process**, contrary to the `--client-secret-env` / `--password-env` design.

Out of scope:

- Findings you disagree with, missing rules, or thresholds you consider too strict or too lax — those are ordinary issues (see [BACKLOG.md](BACKLOG.md)), not vulnerabilities.
- Vulnerabilities in Keycloak itself: report those to the [Keycloak project](https://github.com/keycloak/keycloak/security).
- Anything requiring an attacker who already controls the machine running the audit.

## Handling the source you audit

Two notes that are policy rather than bugs:

1. A realm export is a credential. Keep it out of git, ticket attachments and shared drives, and delete it after the audit. The `source/secret-material` rule reports how much is in the file you just read.
2. Give the Admin API credentials read-only rights (`view-realm`, `view-clients`, `view-identity-providers`). keycloak-doctor never writes to a realm, and a section it may not read is reported as not evaluated rather than skipped silently.
