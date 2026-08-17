package rules

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

const (
	insecureFixture = "../../testdata/insecure-realm.json"
	hardenedFixture = "../../testdata/hardened-realm.json"
)

func loadFixture(t *testing.T, path string) *keycloak.Realm {
	t.Helper()
	realms, err := keycloak.LoadFile(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if len(realms) != 1 {
		t.Fatalf("%s: got %d realms, want 1", path, len(realms))
	}
	return realms[0]
}

// findingsFor runs one rule against one realm.
func findingsFor(t *testing.T, id string, realm *keycloak.Realm) []engine.Finding {
	t.Helper()
	for _, r := range All() {
		if r.ID == id {
			return r.Eval(realm)
		}
	}
	t.Fatalf("no rule with id %q", id)
	return nil
}

// worstFor is the status a rule reports for a realm.
func worstFor(t *testing.T, id string, realm *keycloak.Realm) engine.Status {
	t.Helper()
	return engine.Worst(findingsFor(t, id, realm))
}

// The catalogue is the tool's documentation and the selector surface, so its
// invariants are worth a test of their own.
func TestCatalogueInvariants(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range All() {
		if seen[r.ID] {
			t.Errorf("duplicate rule id %q", r.ID)
		}
		seen[r.ID] = true
		if !strings.Contains(r.ID, "/") {
			t.Errorf("rule %q has no category prefix", r.ID)
		}
		if r.ID != strings.ToLower(r.ID) {
			t.Errorf("rule id %q should be lower case", r.ID)
		}
		if strings.TrimSpace(r.Title) == "" {
			t.Errorf("rule %q has no title", r.ID)
		}
		if len(strings.Fields(r.Rationale)) < 10 {
			t.Errorf("rule %q needs a rationale that explains the risk", r.ID)
		}
		if r.Eval == nil {
			t.Errorf("rule %q has no Eval", r.ID)
		}
	}
	if len(seen) < 20 {
		t.Errorf("only %d rules: the catalogue shrank unexpectedly", len(seen))
	}
}

// Every finding a rule emits must carry the rule's own id: the id is what an
// operator suppresses in CI, so a mismatch silently breaks their pin.
func TestFindingsCarryTheirRuleID(t *testing.T) {
	for _, path := range []string{insecureFixture, hardenedFixture} {
		realm := loadFixture(t, path)
		for _, r := range All() {
			for _, f := range r.Eval(realm) {
				if f.Rule != r.ID {
					t.Errorf("%s: rule %q emitted a finding tagged %q", path, r.ID, f.Rule)
				}
				if f.Realm != realm.Name() {
					t.Errorf("%s: rule %q emitted a finding for realm %q", path, r.ID, f.Realm)
				}
				if strings.TrimSpace(f.Message) == "" {
					t.Errorf("%s: rule %q emitted a finding with no message", path, r.ID)
				}
				if f.Status != engine.OK && strings.TrimSpace(f.Remediation) == "" {
					t.Errorf("%s: rule %q reports %s with no remediation", path, r.ID, f.Status)
				}
			}
		}
	}
}

// A rule must never leak a credential value into a finding, even though the
// loader is what guarantees it: this is the assertion that would catch a rule
// added later that reads Secret directly.
func TestNoFindingCarriesCredentialMaterial(t *testing.T) {
	realm := loadFixture(t, insecureFixture)
	findings := Audit([]*keycloak.Realm{realm}, All())
	if len(findings) == 0 {
		t.Fatal("the insecure fixture must produce findings")
	}
	for _, f := range findings {
		text := f.Message + " " + f.Remediation
		if strings.Contains(text, "example-value-not-a-real-secret") {
			t.Errorf("rule %q leaked the secret value from the source: %s", f.Rule, f.Message)
		}
	}
}

func TestHardenedRealmPassesEveryRule(t *testing.T) {
	realm := loadFixture(t, hardenedFixture)
	findings := Audit([]*keycloak.Realm{realm}, All())
	for _, f := range findings {
		if f.Status != engine.OK {
			t.Errorf("hardened realm should pass, got %s from %s: %s", f.Status, f.Rule, f.Message)
		}
	}
	if got := engine.Worst(findings); got != engine.OK {
		t.Errorf("worst = %s, want OK", got)
	}
}

func TestInsecureRealmIsCaught(t *testing.T) {
	realm := loadFixture(t, insecureFixture)
	findings := Audit([]*keycloak.Realm{realm}, All())
	if got := engine.Worst(findings); got != engine.BAD {
		t.Fatalf("worst = %s, want BAD", got)
	}
	// The rules that must fire on this fixture, with the status they must reach.
	want := map[string]engine.Status{
		"realm/brute-force":              engine.BAD,
		"realm/token-lifespan":           engine.BAD,
		"realm/self-registration":        engine.BAD,
		"realm/password-policy":          engine.WARN,
		"realm/ssl-required":             engine.WARN,
		"realm/offline-session-expiry":   engine.WARN,
		"realm/refresh-token-rotation":   engine.WARN,
		"realm/session-lifespan":         engine.WARN,
		"realm/audit-events":             engine.WARN,
		"realm/browser-mfa":              engine.WARN,
		"client/redirect-wildcard":       engine.BAD,
		"client/redirect-plain-http":     engine.BAD,
		"client/web-origins-wildcard":    engine.BAD,
		"client/implicit-flow":           engine.BAD,
		"client/direct-grant":            engine.BAD,
		"client/pkce":                    engine.BAD,
		"client/full-scope":              engine.BAD,
		"client/token-lifespan-override": engine.BAD,
		"mapper/sensitive-attribute":     engine.BAD,
		"idp/trust-email":                engine.WARN,
		"idp/plaintext-endpoints":        engine.BAD,
		"keys/rsa-size":                  engine.BAD,
		"federation/ldap-tls":            engine.BAD,
		"federation/ldap-truststore":     engine.BAD,
		"source/secret-material":         engine.WARN,
	}
	for id, wantStatus := range want {
		if got := worstFor(t, id, realm); got != wantStatus {
			t.Errorf("%s = %s, want %s", id, got, wantStatus)
		}
	}
}

// A disabled client cannot obtain a token, so its configuration is not a finding:
// the fixture's retired-app has a "*" redirect URI and must stay silent.
func TestDisabledClientsAreSkipped(t *testing.T) {
	realm := loadFixture(t, insecureFixture)
	for _, f := range findingsFor(t, "client/redirect-wildcard", realm) {
		if f.Target == "retired-app" {
			t.Errorf("disabled client reported: %s", f.Message)
		}
	}
}

// When a rule finds nothing to report it must say so once for the realm, not once
// per object — otherwise a realm with 200 clients buries its own findings.
func TestPassingRulesEmitOneAggregateFinding(t *testing.T) {
	realm := loadFixture(t, hardenedFixture)
	for _, r := range All() {
		got := r.Eval(realm)
		if len(got) > 1 {
			t.Errorf("rule %q emitted %d findings on a clean realm, want at most 1", r.ID, len(got))
		}
		if len(got) == 1 && got[0].Target != "" {
			t.Errorf("rule %q aggregate finding should not name a target, got %q", r.ID, got[0].Target)
		}
	}
}

// A section the credentials could not read must report ERROR, never OK: "I could
// not look" and "I looked and it is fine" must not render the same.
func TestUnreadableSectionReportsError(t *testing.T) {
	realm := loadFixture(t, hardenedFixture)
	realm.Missing = map[string]string{"clients": "HTTP 403 from /admin/realms/x/clients"}
	got := findingsFor(t, "client/pkce", realm)
	if len(got) != 1 || got[0].Status != engine.ERROR {
		t.Fatalf("got %+v, want a single ERROR finding", got)
	}
	if !strings.Contains(got[0].Message, "not evaluated") {
		t.Errorf("message should say the rule did not run: %q", got[0].Message)
	}
}

func TestSelect(t *testing.T) {
	byCategory, err := Select([]string{"client"}, nil)
	if err != nil {
		t.Fatalf("select category: %v", err)
	}
	for _, r := range byCategory {
		if r.Category() != "client" {
			t.Errorf("category selection leaked %q", r.ID)
		}
	}
	byID, err := Select([]string{"client/pkce"}, nil)
	if err != nil || len(byID) != 1 || byID[0].ID != "client/pkce" {
		t.Fatalf("select by id = %v (%v)", byID, err)
	}
	skipped, err := Select(nil, []string{"client"})
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	for _, r := range skipped {
		if r.Category() == "client" {
			t.Errorf("skip left %q in", r.ID)
		}
	}
	if _, err := Select([]string{"clients"}, nil); err == nil {
		t.Error("a typo in --only must be an error, not an empty audit")
	}
	if _, err := Select([]string{"client"}, []string{"client"}); err == nil {
		t.Error("a selection that leaves nothing to run must be an error")
	}
}

func TestAuditIsWorstFirstAcrossRealms(t *testing.T) {
	realms := []*keycloak.Realm{loadFixture(t, hardenedFixture), loadFixture(t, insecureFixture)}
	findings := Audit(realms, All())
	if findings[0].Status != engine.BAD {
		t.Errorf("first finding is %s, want the worst first", findings[0].Status)
	}
	seenRealms := map[string]bool{}
	for _, f := range findings {
		seenRealms[f.Realm] = true
	}
	if len(seenRealms) != 2 {
		t.Errorf("both realms should appear, got %v", seenRealms)
	}
}
