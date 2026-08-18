# In CI

**The audit as a required check: SARIF alerts on the repository that holds the realm, a baseline so an accepted realm stops failing the build, and suppressions that expire.**

## The exit code, first

`keycloak-doctor audit` exits **0 even when it reports BAD findings**. An audit that ran is a success and the report is the deliverable; a non-zero exit means something systemic broke — an unreadable source, credentials that do not work, an unknown rule, a bad flag.

Gating is opt-in and explicit:

```bash
keycloak-doctor audit prod-realm.json --exit-on bad     # exit 2 when anything is BAD or ERROR
keycloak-doctor audit prod-realm.json --exit-on warn --exit-code 1
```

`--exit-on` is evaluated on the findings that were **reported**, so it stays consistent with `--min-severity` and with what a human reading the same output would conclude.

## The action

```yaml
name: Realm audit
on:
  pull_request:
    paths: ["realms/**"]

permissions:
  contents: read
  security-events: write   # to upload the SARIF

jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - id: audit
        uses: Allan-Nava/keycloak-doctor@v0.2.0
        with:
          path: realms/prod-realm.json
          exit-on: bad
      - uses: github/codeql-action/upload-sarif@v3
        if: always()          # upload the alerts even when the audit gated
        with:
          sarif_file: ${{ steps.audit.outputs.report }}
```

The action downloads the release binary for the runner and verifies it against the release `checksums.txt` — it does not build from source and does not pull an image, so the step costs about a second and runs the same artifact a human would download. Linux and macOS runners.

| Input | Default | What it does |
|---|---|---|
| `path` | `.` | export file, or a directory of exports |
| `output` | `sarif` | `text`, `markdown`, `json` or `sarif` |
| `out-file` | `keycloak-doctor.sarif` | where the report goes; empty writes it to the step log |
| `exit-on` | `bad` | fail the step at this status; empty never fails it |
| `min-severity`, `only`, `skip`, `realm` | — | the matching flags |
| `baseline` | — | a previous JSON report |
| `fail-on-new` | `false` | narrow `exit-on` to what the baseline does not have |
| `suppress` | — | suppression file |
| `summary` | `true` | write the Markdown report to the job summary |
| `version` | `latest` | a tag such as `v0.2.0`, or `path` to use a `keycloak-doctor` already on `PATH` |

Outputs: `report` (the path), `worst` (`OK`/`WARN`/`BAD`/`ERROR`, or `UNKNOWN` when the audit could not be re-read — that is never reported as `OK`), `suppressed`, `exit-code`.

Prefer the image instead of the action? It is one step:

```yaml
- run: |
    docker run --rm -v "$PWD:/realm:ro" ghcr.io/allan-nava/keycloak-doctor \
      audit /realm/realms/prod-realm.json --output sarif --exit-on bad --out-file /realm/audit.sarif
```

## SARIF

`--output sarif` writes SARIF 2.1.0, which is what GitHub code scanning reads: the findings become alerts on the repository that holds the realm definition, next to the file, instead of a report somebody has to open a job log to find.

Three mappings are worth knowing, because they decide what you see:

- **`BAD` is `error`, `WARN` is `warning`.** The level is per finding, not per rule: the same rule is a BAD on a public client and a WARN on a confidential one.
- **`ERROR` — a rule that could not be evaluated — is a real result at level `note`**, not a skipped rule. A blind spot has to be visible in the same list as the findings; a section the credentials could not read must never render as a clean bill.
- **`OK` is a result of kind `pass` at level `none`.** Code scanning raises no alert for it, and a SARIF reader still sees that the rule ran and passed, so the file stays a faithful projection of the run.

Each result carries a stable `partialFingerprints` entry computed from the rule, the realm and the target — never from the message, which carries counts and lifespans that move without the posture moving. That is what keeps one alert open across runs instead of closing it and opening a new one. When the source is a file in the repository the result is anchored to it; when the source is a live server there is no file, and the result carries only its logical location (`realm/target`) rather than an invented one.

## A baseline: gate on what changed

A realm that has accumulated findings cannot be fixed in one pull request, and a check that always fails is a check people learn to ignore. Take the current state as the baseline, commit it, and gate only on regressions:

```bash
# once, on the branch that is already merged
keycloak-doctor audit prod-realm.json --output json --out-file audit-baseline.json

# in the pull request
keycloak-doctor audit prod-realm.json \
  --baseline audit-baseline.json --exit-on bad --fail-on-new
```

- A finding is matched to the baseline by **rule, realm and target** — not by message.
- A finding whose status got **worse** than the baseline counts as new: a WARN that became a BAD is a regression.
- A finding that improved is not new, and a finding that disappeared is simply gone.
- Without `--fail-on-new`, `--baseline` only annotates: the text output marks the line `NEW ·`, Markdown counts them, JSON and SARIF carry `new`. The gate still considers everything.

`--fail-on-new` needs both `--baseline` (without one, every finding is new) and `--exit-on` (it narrows a gate, it does not create one). Both are usage errors rather than defaults, because guessing here means guessing whether somebody's pipeline fails.

## Suppressions: accepted, dated, reviewable

A suppression is a decision to accept a finding for a while. The file makes that decision explicit, and the two things that make it safe are that it is **never silent** and **never permanent**:

```json
{
  "suppressions": [
    {
      "rule": "client/redirect-wildcard",
      "realm": "prod",
      "target": "legacy-frontend",
      "until": "2026-12-31",
      "reason": "exact callback URLs land with the SPA rewrite, tracked in KD-1234"
    },
    {
      "rule": "keys/hmac-secret-size",
      "until": "2026-10-01",
      "reason": "the shared secret rotates with the next release train"
    }
  ]
}
```

```bash
keycloak-doctor audit prod-realm.json --suppress suppressions.json --exit-on bad
```

- `rule` is required. `realm` and `target` narrow the entry; an empty one (or `"*"`) matches everything, so the second entry above accepts that finding in every realm.
- `until` is required, as a date (`YYYY-MM-DD`). A suppression with no expiry is how a finding disappears for three years.
- `reason` is required. An accepted finding with no reason is one nobody can review.
- An unknown key is an error, not a no-op: a misspelled `targt` would otherwise leave an entry that silently suppresses more than intended.

What the run then tells you:

| The entry | What the run does with it |
|---|---|
| a live suppression | the finding is removed, and the header says `· 2 suppressed` — the count is in the JSON and the SARIF too |
| an expired suppression | it stops suppressing, the finding comes back, and `suppression/expired` reports which entry lapsed and what its reason was |
| an entry that matched nothing | `suppression/unmatched` — either the finding is fixed and the entry is dead, or the rule id, realm or target is a typo and you believe something is accepted when it is still reported |

`suppression/expired` and `suppression/unmatched` are **not** realm rules: they are about your suppression file, so they are not in [the catalogue](rules.md) and `--only`/`--skip` do not select them.

Order of operations in one run: audit → suppress → compare against the baseline → `--min-severity` → render → gate. A suppressed finding never reaches the baseline comparison (it would show up as fixed, then as new again the day the suppression expires) and it can never satisfy a gate, because it is gone from the run.
