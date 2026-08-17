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
