package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
)

func sarifOf(t *testing.T, res engine.Result, opts Options) map[string]any {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := Render(buf, SARIF, res, opts); err != nil {
		t.Fatalf("Render: %v", err)
	}
	var log map[string]any
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatalf("the SARIF is not valid JSON: %v\n%s", err, buf.String())
	}
	return log
}

func sarifResults(t *testing.T, log map[string]any) []map[string]any {
	t.Helper()
	runs, ok := log["runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("runs = %v, want exactly one", log["runs"])
	}
	raw, _ := runs[0].(map[string]any)["results"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func firstRun(t *testing.T, log map[string]any) map[string]any {
	t.Helper()
	return log["runs"].([]any)[0].(map[string]any)
}

func demoResult() engine.Result {
	return engine.Result{
		Findings: []engine.Finding{
			{Rule: "client/pkce", Realm: "demo", Target: "spa", Status: engine.BAD,
				Message:     "a public client runs the code flow without requiring PKCE",
				Remediation: "set the PKCE method to S256"},
			{Rule: "realm/browser-mfa", Realm: "demo", Status: engine.WARN, Message: "one factor only"},
			{Rule: "client/service-account-roles", Realm: "demo", Status: engine.ERROR, Message: "not evaluated: no access to clients"},
			{Rule: "realm/enabled", Realm: "demo", Status: engine.OK, Message: "the realm is enabled"},
		},
		Realms:   []string{"demo"},
		Rules:    30,
		Source:   "file:testdata/insecure-realm.json",
		Started:  time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
		Duration: 3 * time.Millisecond,
	}
}

func TestSARIFHeaderAndTool(t *testing.T) {
	log := sarifOf(t, demoResult(), Options{Version: "0.2.0", Rules: []RuleInfo{
		{ID: "client/pkce", Title: "Public clients require PKCE with S256", Rationale: "Without PKCE a code can be redeemed by anyone."},
	}})

	if log["version"] != "2.1.0" {
		t.Errorf("version = %v, want 2.1.0", log["version"])
	}
	if !strings.Contains(log["$schema"].(string), "sarif-2.1.0") {
		t.Errorf("$schema = %v", log["$schema"])
	}

	driver := firstRun(t, log)["tool"].(map[string]any)["driver"].(map[string]any)
	if driver["name"] != "keycloak-doctor" || driver["version"] != "0.2.0" {
		t.Errorf("driver = %v", driver)
	}
	rules := driver["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("the driver lists %d rules, want the catalogue it was given", len(rules))
	}
	rule := rules[0].(map[string]any)
	if rule["id"] != "client/pkce" || rule["name"] != "ClientPkce" {
		t.Errorf("rule id/name = %v / %v", rule["id"], rule["name"])
	}
	// The rationale is the reason this tool is worth running: it has to reach the
	// alert, not just the terminal.
	if !strings.Contains(rule["fullDescription"].(map[string]any)["text"].(string), "redeemed by anyone") {
		t.Errorf("the rationale is missing from the rule descriptor: %v", rule)
	}
}

func TestSARIFLevelsPerStatus(t *testing.T) {
	log := sarifOf(t, demoResult(), Options{})
	want := map[string]struct{ level, kind string }{
		"client/pkce":                  {"error", ""},
		"realm/browser-mfa":            {"warning", ""},
		"client/service-account-roles": {"note", ""}, // a blind spot has to be visible
		"realm/enabled":                {"none", "pass"},
	}
	seen := map[string]bool{}
	for _, r := range sarifResults(t, log) {
		id := r["ruleId"].(string)
		exp, ok := want[id]
		if !ok {
			t.Errorf("unexpected result for %s", id)
			continue
		}
		seen[id] = true
		if r["level"] != exp.level {
			t.Errorf("%s level = %v, want %v", id, r["level"], exp.level)
		}
		kind, _ := r["kind"].(string)
		if kind != exp.kind {
			t.Errorf("%s kind = %q, want %q", id, kind, exp.kind)
		}
	}
	if len(seen) != len(want) {
		t.Errorf("results cover %d statuses, want all %d: an audit that hides one status is not a projection of the run", len(seen), len(want))
	}
}

func TestSARIFCarriesRemediationAndScope(t *testing.T) {
	log := sarifOf(t, demoResult(), Options{})
	for _, r := range sarifResults(t, log) {
		if r["ruleId"] != "client/pkce" {
			continue
		}
		msg := r["message"].(map[string]any)["text"].(string)
		if !strings.Contains(msg, "spa: ") || !strings.Contains(msg, "→ set the PKCE method") {
			t.Errorf("the alert body must name the target and say what to do: %q", msg)
		}
		props := r["properties"].(map[string]any)
		if props["realm"] != "demo" || props["target"] != "spa" || props["status"] != "BAD" {
			t.Errorf("properties = %v", props)
		}
		loc := r["locations"].([]any)[0].(map[string]any)
		phys := loc["physicalLocation"].(map[string]any)["artifactLocation"].(map[string]any)
		if phys["uri"] != "testdata/insecure-realm.json" {
			t.Errorf("the alert is not anchored to the audited file: %v", phys)
		}
		logical := loc["logicalLocations"].([]any)[0].(map[string]any)
		if logical["fullyQualifiedName"] != "demo/spa" {
			t.Errorf("logical location = %v", logical)
		}
		return
	}
	t.Fatal("no result for client/pkce")
}

// Code scanning tracks an alert across runs by its fingerprint: it must not move
// when the message does, and two findings must not share one.
func TestSARIFFingerprintsAreStableAndDistinct(t *testing.T) {
	res := demoResult()
	first := sarifResults(t, sarifOf(t, res, Options{}))

	res.Findings[0].Message = "reworded, same finding"
	res.Duration = 9 * time.Millisecond
	second := sarifResults(t, sarifOf(t, res, Options{}))

	fp := func(r map[string]any) string {
		return r["partialFingerprints"].(map[string]any)["keycloakDoctor/v1"].(string)
	}
	if fp(first[0]) != fp(second[0]) {
		t.Errorf("the fingerprint moved when only the message changed: %s vs %s", fp(first[0]), fp(second[0]))
	}
	seen := map[string]string{}
	for _, r := range first {
		f := fp(r)
		if other, dup := seen[f]; dup {
			t.Errorf("%s and %s share a fingerprint", other, r["ruleId"])
		}
		seen[f] = r["ruleId"].(string)
	}
}

// A live server is not a file in the repository: inventing a location would put an
// alert on whatever file happened to be named.
func TestSARIFHasNoPhysicalLocationForALiveServer(t *testing.T) {
	res := demoResult()
	res.Source = "api:https://sso.example.com"
	log := sarifOf(t, res, Options{})

	if _, ok := firstRun(t, log)["artifacts"]; ok {
		t.Error("a live-server run declared an artifact")
	}
	for _, r := range sarifResults(t, log) {
		loc := r["locations"].([]any)[0].(map[string]any)
		if _, ok := loc["physicalLocation"]; ok {
			t.Errorf("%s got a physical location from a live server: %v", r["ruleId"], loc)
		}
		if _, ok := loc["logicalLocations"]; !ok {
			t.Errorf("%s lost its logical location", r["ruleId"])
		}
	}
}

func TestSARIFReportsSuppressedAndNew(t *testing.T) {
	res := demoResult()
	res.Suppressed = 2
	res.Findings[0].New = true
	log := sarifOf(t, res, Options{})

	props := firstRun(t, log)["properties"].(map[string]any)
	// ERROR outranks BAD in this project's ordering: a rule that could not run is
	// the worst thing an audit can report.
	if props["suppressed"].(float64) != 2 || props["worst"] != "ERROR" {
		t.Errorf("run properties = %v", props)
	}
	for _, r := range sarifResults(t, log) {
		if r["ruleId"] == "client/pkce" {
			if r["properties"].(map[string]any)["new"] != true {
				t.Errorf("the new finding is not flagged in SARIF: %v", r["properties"])
			}
			return
		}
	}
}

// Without a catalogue the file must still describe its rules, or a reader gets
// results pointing at ids that are documented nowhere.
func TestSARIFFallsBackToTheIDsInTheFindings(t *testing.T) {
	log := sarifOf(t, demoResult(), Options{})
	rules := firstRun(t, log)["tool"].(map[string]any)["driver"].(map[string]any)["rules"].([]any)
	if len(rules) != 4 {
		t.Errorf("driver rules = %d, want one per distinct rule in the findings (4)", len(rules))
	}
	for i, r := range sarifResults(t, log) {
		idx := int(r["ruleIndex"].(float64))
		if idx < 0 || idx >= len(rules) {
			t.Fatalf("result %d has ruleIndex %d, out of range", i, idx)
		}
		if rules[idx].(map[string]any)["id"] != r["ruleId"] {
			t.Errorf("result %d points at the wrong rule: %v vs %v", i, rules[idx], r["ruleId"])
		}
	}
}

func TestSARIFIsAKnownFormat(t *testing.T) {
	found := false
	for _, f := range Formats() {
		if f == "sarif" {
			found = true
		}
	}
	if !found {
		t.Errorf("Formats() = %v, want sarif listed for the usage text and validation", Formats())
	}
}
