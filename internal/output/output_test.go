package output

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
)

func sampleResult() engine.Result {
	return engine.Result{
		Findings: engine.SortFindings([]engine.Finding{
			{Rule: "realm/brute-force", Realm: "demo", Status: engine.BAD,
				Message: "brute force detection is disabled", Remediation: "enable it"},
			{Rule: "client/pkce", Realm: "demo", Target: "web", Status: engine.WARN,
				Message: "no PKCE", Remediation: "set S256"},
			{Rule: "realm/ssl-required", Realm: "demo", Status: engine.OK,
				Message: "HTTPS is required for all requests"},
		}),
		Realms:   []string{"demo"},
		Rules:    3,
		Source:   "file:testdata/insecure-realm.json",
		Started:  time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
		Duration: 12 * time.Millisecond,
	}
}

func render(t *testing.T, format Format, res engine.Result) string {
	t.Helper()
	var b strings.Builder
	if err := Render(&b, format, res, Options{Version: "0.1.0"}); err != nil {
		t.Fatalf("render %s: %v", format, err)
	}
	return b.String()
}

func TestTextIsWorstFirstAndCarriesRemediation(t *testing.T) {
	got := render(t, Text, sampleResult())
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if !strings.Contains(lines[0], "keycloak-doctor 0.1.0") || !strings.Contains(lines[0], "1 realm") {
		t.Errorf("header = %q", lines[0])
	}
	// The first finding line is the thing to look at: that ordering is API for
	// anything parsing the text output.
	if !strings.Contains(lines[2], "BAD") || !strings.Contains(lines[2], "realm/brute-force") {
		t.Errorf("first finding line = %q", lines[2])
	}
	if !strings.Contains(got, "→ enable it") {
		t.Error("remediation should be printed under the finding")
	}
	if strings.Contains(got, "→ ") && strings.Contains(got, "HTTPS is required for all requests\n      →") {
		t.Error("an OK finding needs no remediation line")
	}
	if !strings.Contains(got, "demo · web") {
		t.Error("the target should be shown next to its realm")
	}
	if !strings.Contains(got, "worst: BAD") {
		t.Error("the summary should carry the worst status")
	}
	if strings.Contains(got, "\033[") {
		t.Error("colour must be off unless the caller asks for it")
	}
}

func TestTextColorOnlyWhenAsked(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, Text, sampleResult(), Options{Color: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), ansiRed) {
		t.Error("a BAD finding should be painted when Color is set")
	}
}

func TestTextWithNoFindings(t *testing.T) {
	res := sampleResult()
	res.Findings = nil
	got := render(t, Text, res)
	if !strings.Contains(got, "no findings") {
		t.Errorf("empty run should say so: %q", got)
	}
}

func TestMarkdownHasSummaryAndAttentionSection(t *testing.T) {
	got := render(t, Markdown, sampleResult())
	for _, want := range []string{
		"# Keycloak audit — demo",
		"**Worst status**: `BAD`",
		"## Needs attention",
		"## All findings",
		"| BAD | `realm/brute-force` | demo |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown is missing %q:\n%s", want, got)
		}
	}
	// OK findings belong in the table, not in the attention list.
	attention := got[strings.Index(got, "## Needs attention"):strings.Index(got, "## All findings")]
	if strings.Contains(attention, "realm/ssl-required") {
		t.Error("an OK finding must not appear under Needs attention")
	}
}

func TestMarkdownEscapesPipes(t *testing.T) {
	res := sampleResult()
	res.Findings = []engine.Finding{{Rule: "r/x", Realm: "demo", Status: engine.BAD, Message: "a|b", Remediation: "y"}}
	if !strings.Contains(render(t, Markdown, res), `a\|b`) {
		t.Error("a pipe in a message must be escaped or it breaks the table")
	}
}

func TestJSONShapeIsTheGatingContract(t *testing.T) {
	out := render(t, JSON, sampleResult())
	var parsed struct {
		Version  string         `json:"version"`
		Source   string         `json:"source"`
		Realms   []string       `json:"realms"`
		Rules    int            `json:"rules"`
		Worst    string         `json:"worst"`
		Summary  map[string]int `json:"summary"`
		Findings []struct {
			Rule        string `json:"rule"`
			Realm       string `json:"realm"`
			Target      string `json:"target"`
			Status      string `json:"status"`
			Message     string `json:"message"`
			Remediation string `json:"remediation"`
		} `json:"findings"`
		Duration int64 `json:"duration_ns"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if parsed.Worst != "BAD" {
		t.Errorf("worst = %q", parsed.Worst)
	}
	if parsed.Summary["BAD"] != 1 || parsed.Summary["WARN"] != 1 || parsed.Summary["OK"] != 1 {
		t.Errorf("summary = %+v", parsed.Summary)
	}
	if len(parsed.Findings) != 3 || parsed.Findings[0].Rule != "realm/brute-force" {
		t.Errorf("findings = %+v", parsed.Findings)
	}
	if parsed.Rules != 3 || parsed.Version != "0.1.0" || parsed.Realms[0] != "demo" {
		t.Errorf("header fields = %+v", parsed)
	}
	if parsed.Duration != int64(12*time.Millisecond) {
		t.Errorf("duration_ns = %d", parsed.Duration)
	}
}

func TestJSONEmptyRunHasArraysNotNull(t *testing.T) {
	res := engine.Result{}
	out := render(t, JSON, res)
	if strings.Contains(out, "null") {
		t.Errorf("an empty run must render empty arrays, not null:\n%s", out)
	}
}

func TestUnknownFormatIsAnError(t *testing.T) {
	var b strings.Builder
	if err := Render(&b, Format("yaml"), sampleResult(), Options{}); err == nil {
		t.Error("an unknown format should be rejected")
	}
}
