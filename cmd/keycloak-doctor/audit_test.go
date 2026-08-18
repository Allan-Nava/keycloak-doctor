package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	insecureFixture = "../../testdata/insecure-realm.json"
	hardenedFixture = "../../testdata/hardened-realm.json"
)

// auditToFile runs the command with --out-file, which is also how the output is
// captured without touching the process' stdout.
func auditToFile(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "audit.out")
	err := runAudit(append(args, "--out-file", out))
	b, readErr := os.ReadFile(out) //nolint:gosec // a path this test just created
	if readErr != nil && err == nil {
		t.Fatalf("no output written: %v", readErr)
	}
	return string(b), err
}

func TestAuditFileShortForm(t *testing.T) {
	out, err := auditToFile(t, "audit-args-placeholder")
	if err == nil {
		t.Fatal("a non-existent source should fail")
	}
	if out != "" {
		t.Errorf("nothing should have been written: %q", out)
	}

	out, err = auditToFile(t, insecureFixture)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if !strings.Contains(out, "BAD") || !strings.Contains(out, "worst: BAD") {
		t.Errorf("the insecure fixture should report BAD:\n%s", out)
	}
	if strings.Contains(out, "example-value-not-a-real-secret") {
		t.Error("the output must never carry a credential value from the source")
	}
}

func TestAuditJSONAndExitGate(t *testing.T) {
	out, err := auditToFile(t, insecureFixture, "--output", "json", "--exit-on", "bad")
	var ee *exitError
	if !errors.As(err, &ee) {
		t.Fatalf("--exit-on bad should gate on this fixture, got %v", err)
	}
	if ee.code != 2 {
		t.Errorf("default gate exit code = %d, want 2", ee.code)
	}
	var parsed struct {
		Worst  string `json:"worst"`
		Realms []string
		Rules  int
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json output: %v\n%s", err, out)
	}
	if parsed.Worst != "BAD" || parsed.Rules == 0 || parsed.Realms[0] != "demo" {
		t.Errorf("unexpected result: %+v", parsed)
	}
}

// The gate must not fire on a realm that passes: findings are the deliverable,
// exit codes are for pipelines that asked for one.
func TestAuditHardenedRealmDoesNotGate(t *testing.T) {
	_, err := auditToFile(t, hardenedFixture, "--exit-on", "warn")
	if err != nil {
		t.Fatalf("a hardened realm should not gate: %v", err)
	}
}

func TestAuditCustomExitCode(t *testing.T) {
	_, err := auditToFile(t, insecureFixture, "--exit-on", "warn", "--exit-code", "7")
	var ee *exitError
	if !errors.As(err, &ee) || ee.code != 7 {
		t.Fatalf("--exit-code should be honoured, got %v", err)
	}
}

func TestAuditMinSeverityFiltersAndKeepsTheGateConsistent(t *testing.T) {
	out, err := auditToFile(t, insecureFixture, "--min-severity", "bad", "--output", "json")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	var parsed struct {
		Findings []struct {
			Status string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Findings) == 0 {
		t.Fatal("the fixture has BAD findings")
	}
	for _, f := range parsed.Findings {
		if f.Status != "BAD" {
			t.Errorf("--min-severity bad let a %s finding through", f.Status)
		}
	}
}

func TestAuditRuleSelection(t *testing.T) {
	out, err := auditToFile(t, insecureFixture, "--only", "client", "--output", "json")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	var parsed struct {
		Findings []struct {
			Rule string `json:"rule"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	for _, f := range parsed.Findings {
		if !strings.HasPrefix(f.Rule, "client/") {
			t.Errorf("--only client let %q through", f.Rule)
		}
	}
	if _, err := auditToFile(t, insecureFixture, "--only", "nonsense"); err == nil {
		t.Error("an unknown selector must be a systemic error")
	}
}

func TestAuditRealmSelection(t *testing.T) {
	if _, err := auditToFile(t, insecureFixture, "--realm", "demo"); err != nil {
		t.Errorf("selecting the realm in the file should work: %v", err)
	}
	if _, err := auditToFile(t, insecureFixture, "--realm", "prod"); err == nil {
		t.Error("a realm not in the source must be an error")
	}
}

func TestAuditRejectsAmbiguousOrMissingSource(t *testing.T) {
	cases := [][]string{
		{},
		{"--file", insecureFixture, "--url", "https://sso.example.com"},
		{insecureFixture, "--file", insecureFixture},
		{insecureFixture, hardenedFixture},
		{"--url", "https://sso.example.com"},                 // no --realm and no --all-realms
		{"--url", "https://sso.example.com", "--all-realms"}, // no credentials
	}
	for _, args := range cases {
		if _, err := auditToFile(t, args...); err == nil {
			t.Errorf("args %v should have been rejected", args)
		}
	}
}

// A named environment variable that is not set must fail loudly rather than fall
// back to another grant.
func TestAuditRequiresTheNamedSecretEnv(t *testing.T) {
	_, err := auditToFile(t, "--url", "https://sso.example.com", "--all-realms",
		"--client-id", "audit", "--client-secret-env", "KC_DOCTOR_TEST_UNSET")
	if err == nil || !strings.Contains(err.Error(), "KC_DOCTOR_TEST_UNSET") {
		t.Errorf("an unset secret variable should be named in the error, got %v", err)
	}
}

func TestAuditBadStatusFlags(t *testing.T) {
	if _, err := auditToFile(t, insecureFixture, "--min-severity", "critical"); err == nil {
		t.Error("an unknown status should be rejected")
	}
	if _, err := auditToFile(t, insecureFixture, "--exit-on", "yes"); err == nil {
		t.Error("an unknown gate status should be rejected")
	}
}

func TestRunRulesCatalogue(t *testing.T) {
	if err := runRules([]string{"--output", "json"}); err != nil {
		t.Errorf("rules --output json: %v", err)
	}
	if err := runRules([]string{"--only", "client/pkce"}); err != nil {
		t.Errorf("rules --only: %v", err)
	}
	if err := runRules([]string{"--only", "nope"}); err == nil {
		t.Error("an unknown selector must be an error")
	}
	if err := runRules([]string{"--output", "yaml"}); err == nil {
		t.Error("an unknown format must be an error")
	}
}

func TestWrap(t *testing.T) {
	got := wrap("one two three four five", 9, "  ")
	if !strings.Contains(got, "\n  ") {
		t.Errorf("text should wrap with the indent: %q", got)
	}
	if wrap("", 10, "") != "" {
		t.Error("empty text should stay empty")
	}
}

// write drops a file in a temp dir and returns its path.
func write(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAuditSARIF(t *testing.T) {
	out, err := auditToFile(t, insecureFixture, "--output", "sarif")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatalf("invalid SARIF: %v\n%s", err, out)
	}
	if log.Version != "2.1.0" || len(log.Runs) != 1 {
		t.Fatalf("version = %q, runs = %d", log.Version, len(log.Runs))
	}
	if len(log.Runs[0].Results) == 0 {
		t.Error("no results for the insecure fixture")
	}
	// The catalogue that ran is described, and so are the suppression findings an
	// operator can see in the same report.
	ids := map[string]bool{}
	for _, r := range log.Runs[0].Tool.Driver.Rules {
		ids[r.ID] = true
	}
	for _, want := range []string{"client/pkce", "suppression/expired", "suppression/unmatched"} {
		if !ids[want] {
			t.Errorf("%s is missing from the tool descriptor", want)
		}
	}
	if strings.Contains(out, "example-value-not-a-real-secret") {
		t.Error("the SARIF must never carry a credential value from the source")
	}
}

func TestAuditBaselineAndFailOnNew(t *testing.T) {
	// The baseline is the fixture audited with one rule skipped, so exactly that
	// rule's findings are "new" on the next run.
	baseline := filepath.Join(t.TempDir(), "base.json")
	if err := runAudit([]string{insecureFixture, "--output", "json", "--skip", "client/pkce", "--out-file", baseline}); err != nil {
		t.Fatalf("baseline run: %v", err)
	}

	out, err := auditToFile(t, insecureFixture, "--baseline", baseline, "--output", "json")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	var parsed struct {
		Findings []struct {
			Rule string `json:"rule"`
			New  bool   `json:"new"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	newRules := map[string]bool{}
	for _, f := range parsed.Findings {
		if f.New {
			newRules[f.Rule] = true
		}
	}
	if !newRules["client/pkce"] {
		t.Error("the finding the baseline did not have is not marked new")
	}
	if len(newRules) != 1 {
		t.Errorf("new findings = %v, want only the skipped rule: everything else was already known", newRules)
	}

	// The gate now fires on the regression...
	var ee *exitError
	_, err = auditToFile(t, insecureFixture, "--baseline", baseline, "--exit-on", "bad", "--fail-on-new")
	if !errors.As(err, &ee) {
		t.Errorf("--fail-on-new should gate on a finding the baseline did not have, got %v", err)
	}

	// ...and not on a baseline that already contains everything, even though the
	// realm is full of BAD findings. That is the whole point of the flag.
	full := filepath.Join(t.TempDir(), "full.json")
	if err := runAudit([]string{insecureFixture, "--output", "json", "--out-file", full}); err != nil {
		t.Fatalf("full baseline run: %v", err)
	}
	if _, err := auditToFile(t, insecureFixture, "--baseline", full, "--exit-on", "bad", "--fail-on-new"); err != nil {
		t.Errorf("--fail-on-new gated on findings that are all in the baseline: %v", err)
	}
	// Without --fail-on-new the same run still gates: the flag narrows the gate, it
	// does not disable it.
	if _, err := auditToFile(t, insecureFixture, "--baseline", full, "--exit-on", "bad"); !errors.As(err, &ee) {
		t.Errorf("--exit-on bad stopped gating once a baseline was given: %v", err)
	}
}

func TestAuditFailOnNewNeedsItsCompanions(t *testing.T) {
	if _, err := auditToFile(t, insecureFixture, "--fail-on-new"); err == nil ||
		!strings.Contains(err.Error(), "--baseline") {
		t.Errorf("--fail-on-new without a baseline should say so, got %v", err)
	}
	base := write(t, "base.json", `{"findings": []}`)
	if _, err := auditToFile(t, insecureFixture, "--fail-on-new", "--baseline", base); err == nil ||
		!strings.Contains(err.Error(), "--exit-on") {
		t.Errorf("--fail-on-new without a gate should say so, got %v", err)
	}
}

func TestAuditBaselineRejectsSomethingElse(t *testing.T) {
	notABaseline := write(t, "notes.json", `{"realms": ["demo"]}`)
	if _, err := auditToFile(t, insecureFixture, "--baseline", notABaseline); err == nil ||
		!strings.Contains(err.Error(), "findings") {
		t.Errorf("a file without findings should be refused as a baseline, got %v", err)
	}
	if _, err := auditToFile(t, insecureFixture, "--baseline", "no-such-file.json"); err == nil {
		t.Error("a missing baseline should fail the run, not be ignored")
	}
}

func TestAuditSuppressions(t *testing.T) {
	supp := write(t, "supp.json", `{"suppressions": [
	  {"rule": "client/pkce", "realm": "demo", "target": "legacy-frontend",
	   "until": "2099-01-01", "reason": "the SPA rewrite lands next quarter"},
	  {"rule": "keys/rsa-size", "until": "2000-01-01", "reason": "expired on purpose"},
	  {"rule": "client/nope", "until": "2099-01-01", "reason": "a rule id that does not exist"}
	]}`)

	out, err := auditToFile(t, insecureFixture, "--suppress", supp, "--output", "json")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	var parsed struct {
		Suppressed int `json:"suppressed"`
		Findings   []struct {
			Rule   string `json:"rule"`
			Target string `json:"target"`
			Status string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Suppressed != 1 {
		t.Errorf("suppressed = %d, want the one live suppression", parsed.Suppressed)
	}
	seen := map[string]bool{}
	for _, f := range parsed.Findings {
		seen[f.Rule] = true
		if f.Rule == "client/pkce" && f.Target == "legacy-frontend" {
			t.Error("the suppressed finding is still in the report")
		}
	}
	// The expired entry does not suppress, and both problems with the file are
	// reported rather than left for somebody to notice.
	if !seen["keys/rsa-size"] {
		t.Error("an expired suppression is still hiding its finding")
	}
	for _, want := range []string{"suppression/expired", "suppression/unmatched"} {
		if !seen[want] {
			t.Errorf("%s was not reported", want)
		}
	}

	// A suppressed finding cannot satisfy a gate: it is gone from the run.
	if _, err := auditToFile(t, insecureFixture, "--suppress", supp, "--only", "client/pkce",
		"--exit-on", "bad", "--min-severity", "bad"); err != nil {
		t.Errorf("the gate fired on a suppressed finding: %v", err)
	}
}

func TestAuditSuppressionsRejectABrokenFile(t *testing.T) {
	bad := write(t, "supp.json", `{"suppressions": [{"rule": "client/pkce"}]}`)
	_, err := auditToFile(t, insecureFixture, "--suppress", bad)
	if err == nil {
		t.Fatal("a suppression without an expiry or a reason should fail the run")
	}
	for _, want := range []string{"until", "reason"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not say what is missing (%s): %v", want, err)
		}
	}
	if _, err := auditToFile(t, insecureFixture, "--suppress", "no-such-file.json"); err == nil {
		t.Error("a missing suppression file should fail the run, not be ignored")
	}
}
