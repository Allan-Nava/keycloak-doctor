package backlog

import (
	"os"
	"strings"
	"testing"
)

const sample = `# Backlog

Prose that must not be parsed as an item.

## Milestones

- **v0.2.0** (due 2026-09-30) — Pull-request gating: the audit as a required check.
  Items: KD-10, KD-11.
- **v0.3.0** — Desired state sources. Items: KD-8.

## Open

### Rules

- **KD-1** — ` + "`client/consent`" + `: report a client that skips consent. Needs the client scopes.
- **KD-8** — Kubernetes source: read realms from custom resources.

### Sources and integration

- **KD-10** — SARIF output, so findings land in code scanning.
- **KD-11** — GitHub Action wrapping the audit.

## Done

- **KD-0** — v0.1.0: engine, both sources, 30 rules.
`

func parseSample(t *testing.T) Doc {
	t.Helper()
	doc, err := Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc
}

func TestParseItems(t *testing.T) {
	doc := parseSample(t)

	if got, want := len(doc.Items), 5; got != want {
		t.Fatalf("items = %d, want %d", got, want)
	}
	if got, want := len(doc.Open()), 4; got != want {
		t.Errorf("open items = %d, want %d", got, want)
	}

	// Sorted by id number, not by position in the file.
	var ids []string
	for _, it := range doc.Items {
		ids = append(ids, it.ID)
	}
	if got, want := strings.Join(ids, ","), "KD-0,KD-1,KD-8,KD-10,KD-11"; got != want {
		t.Errorf("ids = %q, want %q", got, want)
	}

	kd1, ok := itemByID(doc, "KD-1")
	if !ok {
		t.Fatal("KD-1 not parsed")
	}
	if kd1.Group != "Rules" {
		t.Errorf("KD-1 group = %q, want %q", kd1.Group, "Rules")
	}
	if kd1.Done {
		t.Error("KD-1 is under Open but parsed as done")
	}
	if !strings.HasPrefix(kd1.Text, "`client/consent`: report") {
		t.Errorf("KD-1 text = %q", kd1.Text)
	}

	kd0, _ := itemByID(doc, "KD-0")
	if !kd0.Done {
		t.Error("KD-0 is under Done but parsed as open")
	}
}

func TestParseMilestones(t *testing.T) {
	doc := parseSample(t)

	if got, want := len(doc.Milestones), 2; got != want {
		t.Fatalf("milestones = %d, want %d", got, want)
	}
	m := doc.Milestones[0]
	if m.Title != "v0.2.0" {
		t.Errorf("title = %q", m.Title)
	}
	if m.Due != "2026-09-30" {
		t.Errorf("due = %q, want 2026-09-30", m.Due)
	}
	// The "Items:" tail is machine-readable and must not leak into the description
	// that becomes the milestone description on GitHub.
	if strings.Contains(m.Description, "Items:") || strings.Contains(m.Description, "KD-10") {
		t.Errorf("description still carries the item list: %q", m.Description)
	}
	if m.Description != "Pull-request gating: the audit as a required check" {
		t.Errorf("description = %q", m.Description)
	}
	if got, want := strings.Join(m.Items, ","), "KD-10,KD-11"; got != want {
		t.Errorf("items = %q, want %q", got, want)
	}

	for _, tc := range []struct{ id, milestone string }{
		{"KD-10", "v0.2.0"},
		{"KD-11", "v0.2.0"},
		{"KD-8", "v0.3.0"},
		{"KD-1", ""},
	} {
		it, _ := itemByID(doc, tc.id)
		if it.Milestone != tc.milestone {
			t.Errorf("%s milestone = %q, want %q", tc.id, it.Milestone, tc.milestone)
		}
	}
}

func TestParseMalformedBulletIsAnError(t *testing.T) {
	src := `## Open

### Rules

- **KD-1** — a well-formed item.
- KD-2: a bullet that forgot the shape.

## Milestones

- v0.2.0 — a milestone that forgot the shape.
`
	doc, err := Parse([]byte(src))
	if err == nil {
		t.Fatal("Parse accepted a malformed bullet")
	}
	for _, want := range []string{"line 6", "line 10"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not point at %s: %v", want, err)
		}
	}
	// The well-formed items are still returned, so the report is complete.
	if len(doc.Items) != 1 {
		t.Errorf("items = %d, want the one well-formed item", len(doc.Items))
	}
}

func TestItemTitleAndBody(t *testing.T) {
	long := Item{ID: "KD-1", Text: "`client/service-account-roles`: read the service account's role mappings from the Admin API and report a privileged one. Needs an extra endpoint."}
	title := long.Title()
	if !strings.HasPrefix(title, "KD-1: ") {
		t.Fatalf("title must start with the id: %q", title)
	}
	if strings.Contains(title, "`") {
		t.Errorf("title carries backticks: %q", title)
	}
	if strings.Contains(title, "Needs an extra endpoint") {
		t.Errorf("title kept a second sentence: %q", title)
	}
	if len(title) > len("KD-1: ")+titleMax+len("…") {
		t.Errorf("title not clipped: %q (%d bytes)", title, len(title))
	}
	// The id prefix is the matching key on sync, so it must survive round-tripping.
	if id, ok := backlogID(title); !ok || id != "KD-1" {
		t.Errorf("backlogID(%q) = %q, %v", title, id, ok)
	}

	short := Item{ID: "KD-2", Text: "Short one."}
	if got, want := short.Title(), "KD-2: Short one"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}

	body := Item{ID: "KD-3", Group: "Rules", Text: "Do the thing."}.Body("owner/name")
	for _, want := range []string{"Do the thing.", "Area: **Rules**", "BACKLOG.md", "owner/name", "is overwritten"} {
		if !strings.Contains(body, want) {
			t.Errorf("body is missing %q:\n%s", want, body)
		}
	}
}

func TestItemLabels(t *testing.T) {
	got := Item{ID: "KD-1", Group: "Sources and integration"}.Labels()
	want := []string{"backlog", "area/sources-and-integration"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("labels = %v, want %v", got, want)
	}
	ungrouped := Item{ID: "KD-2"}
	if got := ungrouped.Labels(); strings.Join(got, ",") != "backlog" {
		t.Errorf("labels without a group = %v, want [backlog]", got)
	}
}

// The committed BACKLOG.md is the input this tool exists for: if it stops
// parsing, or stops being consistent, the workflow would fail on a push instead
// of here — the same anti-divergence gate docs/rules.md has.
func TestCommittedBacklogIsValid(t *testing.T) {
	src, err := os.ReadFile("../../BACKLOG.md")
	if err != nil {
		t.Fatalf("reading BACKLOG.md: %v", err)
	}
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("BACKLOG.md does not parse: %v", err)
	}
	for _, p := range doc.Check() {
		t.Errorf("BACKLOG.md:%d: %s: %s", p.Line, p.Level, p.Message)
	}
	if len(doc.Items) == 0 || len(doc.Milestones) == 0 {
		t.Fatalf("BACKLOG.md parsed as %d items and %d milestones", len(doc.Items), len(doc.Milestones))
	}
}
