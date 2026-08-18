package suppress

import (
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
)

func load(t *testing.T, body string) *Set {
	t.Helper()
	set, err := Load(strings.NewReader(body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return set
}

func at(day string) time.Time {
	d, err := time.Parse(dateLayout, day)
	if err != nil {
		panic(err)
	}
	return d
}

func findings() []engine.Finding {
	return []engine.Finding{
		{Rule: "client/redirect-wildcard", Realm: "prod", Target: "spa", Status: engine.BAD, Message: "wildcard"},
		{Rule: "client/redirect-wildcard", Realm: "staging", Target: "spa", Status: engine.BAD, Message: "wildcard"},
		{Rule: "keys/rsa-size", Realm: "prod", Target: "legacy", Status: engine.BAD, Message: "1024 bits"},
		{Rule: "realm/enabled", Realm: "prod", Status: engine.OK, Message: "enabled"},
	}
}

func ruleIDs(fs []engine.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule+"@"+f.Realm)
	}
	return out
}

func TestApplySuppressesOnlyWhatItNames(t *testing.T) {
	set := load(t, `{"suppressions": [
	  {"rule": "client/redirect-wildcard", "realm": "prod", "target": "spa",
	   "until": "2030-01-01", "reason": "exact callbacks land with the SPA rewrite"}
	]}`)

	out := set.Apply(findings(), at("2026-08-17"))
	if out.Suppressed != 1 {
		t.Errorf("suppressed = %d, want 1", out.Suppressed)
	}
	// The staging finding of the same rule is a different finding and stays.
	if got := strings.Join(ruleIDs(out.Findings), " "); !strings.Contains(got, "client/redirect-wildcard@staging") {
		t.Errorf("the staging finding was suppressed too: %s", got)
	}
	for _, f := range out.Findings {
		if f.Rule == "client/redirect-wildcard" && f.Realm == "prod" {
			t.Error("the named finding was not suppressed")
		}
	}
}

func TestApplyWithoutRealmSuppressesEverywhere(t *testing.T) {
	set := load(t, `{"suppressions": [
	  {"rule": "client/redirect-wildcard", "until": "2030-01-01", "reason": "accepted in every realm for now"}
	]}`)
	out := set.Apply(findings(), at("2026-08-17"))
	if out.Suppressed != 2 {
		t.Errorf("suppressed = %d, want both realms (2)", out.Suppressed)
	}
}

// The property that makes a suppression file safe: it stops working, and the run
// says so.
func TestExpiredSuppressionStopsSuppressingAndIsReported(t *testing.T) {
	set := load(t, `{"suppressions": [
	  {"rule": "keys/rsa-size", "realm": "prod", "until": "2026-01-01", "reason": "provider rotation planned"}
	]}`)

	out := set.Apply(findings(), at("2026-08-17"))
	if out.Suppressed != 0 {
		t.Errorf("an expired suppression must not hide anything, suppressed = %d", out.Suppressed)
	}

	var reported *engine.Finding
	for i := range out.Findings {
		if out.Findings[i].Rule == RuleExpired {
			reported = &out.Findings[i]
		}
		if out.Findings[i].Rule == RuleUnmatched {
			t.Error("an expired entry that did match must not also be reported as unmatched")
		}
	}
	if reported == nil {
		t.Fatalf("no %s finding: an expiry nobody is told about is a silent suppression", RuleExpired)
	}
	if reported.Status != engine.WARN {
		t.Errorf("status = %s, want WARN", reported.Status)
	}
	for _, want := range []string{"keys/rsa-size", "2026-01-01", "provider rotation planned"} {
		if !strings.Contains(reported.Message, want) {
			t.Errorf("the message does not carry %q: %s", want, reported.Message)
		}
	}
	// The suppressed finding is back in the report.
	found := false
	for _, f := range out.Findings {
		if f.Rule == "keys/rsa-size" && f.Realm == "prod" {
			found = true
		}
	}
	if !found {
		t.Error("the finding did not come back after the suppression expired")
	}
}

func TestSuppressionExpiresTheDayAfterItsDate(t *testing.T) {
	set := load(t, `{"suppressions": [
	  {"rule": "keys/rsa-size", "until": "2026-08-17", "reason": "one more day"}
	]}`)
	if out := set.Apply(findings(), at("2026-08-17")); out.Suppressed != 1 {
		t.Errorf("on its last day the suppression still applies, suppressed = %d", out.Suppressed)
	}
	if out := set.Apply(findings(), at("2026-08-18")); out.Suppressed != 0 {
		t.Errorf("the day after, it does not, suppressed = %d", out.Suppressed)
	}
}

func TestUnmatchedSuppressionIsReported(t *testing.T) {
	set := load(t, `{"suppressions": [
	  {"rule": "client/typo-here", "until": "2030-01-01", "reason": "a rule id nobody checked"}
	]}`)
	out := set.Apply(findings(), at("2026-08-17"))

	var reported *engine.Finding
	for i := range out.Findings {
		if out.Findings[i].Rule == RuleUnmatched {
			reported = &out.Findings[i]
		}
	}
	if reported == nil {
		t.Fatalf("no %s finding: an entry that suppresses nothing looks like one that works", RuleUnmatched)
	}
	if !strings.Contains(reported.Message, "client/typo-here") {
		t.Errorf("the message does not name the entry: %s", reported.Message)
	}
}

func TestLoadRejectsAnIncompleteEntry(t *testing.T) {
	for name, body := range map[string]string{
		"no rule":     `{"suppressions": [{"until": "2030-01-01", "reason": "why"}]}`,
		"no expiry":   `{"suppressions": [{"rule": "a/b", "reason": "why"}]}`,
		"no reason":   `{"suppressions": [{"rule": "a/b", "until": "2030-01-01"}]}`,
		"bad date":    `{"suppressions": [{"rule": "a/b", "until": "next quarter", "reason": "why"}]}`,
		"typo in key": `{"suppressions": [{"rule": "a/b", "targt": "spa", "until": "2030-01-01", "reason": "why"}]}`,
		"not json":    `rule: a/b`,
	} {
		if _, err := Load(strings.NewReader(body)); err == nil {
			t.Errorf("%s: Load accepted it", name)
		}
	}
}

func TestLoadAcceptsAnEmptyFile(t *testing.T) {
	set := load(t, `{"suppressions": []}`)
	out := set.Apply(findings(), at("2026-08-17"))
	if out.Suppressed != 0 || len(out.Findings) != len(findings()) {
		t.Errorf("an empty file changed the run: %+v", out)
	}
}

func TestRulesAreDocumented(t *testing.T) {
	ids := IDs()
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want the two suppression findings", ids)
	}
	for _, r := range Rules() {
		if r.ID == "" || r.Title == "" || len(r.Rationale) < 40 {
			t.Errorf("%q is not documented well enough for a report an operator reads: %+v", r.ID, r)
		}
		if !strings.HasPrefix(r.ID, "suppression/") {
			t.Errorf("%q is not namespaced under suppression/", r.ID)
		}
	}
}
