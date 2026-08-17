package main

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/keycloak-doctor/internal/rules"
)

func TestSplitLiftsTitleAndSubtitle(t *testing.T) {
	title, subtitle, body := split("# keycloak-doctor\n\n**Audit a Keycloak realm.**\n\n## What it checks\n\nprose\n")
	if title != "keycloak-doctor" {
		t.Errorf("title = %q", title)
	}
	if subtitle != "Audit a Keycloak realm." {
		t.Errorf("subtitle = %q", subtitle)
	}
	// The layout renders both, so neither may remain in the body.
	if strings.Contains(body, "# keycloak-doctor") || strings.Contains(body, "**Audit") {
		t.Errorf("body repeats the title or the subtitle:\n%s", body)
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "## What it checks") {
		t.Errorf("body does not start at the first section:\n%s", body)
	}

	// A document without a bold one-liner keeps its first paragraph.
	_, subtitle, body = split("# Backlog\n\nSingle source of truth.\n")
	if subtitle != "" {
		t.Errorf("subtitle = %q, want empty", subtitle)
	}
	if !strings.Contains(body, "Single source of truth.") {
		t.Errorf("body lost its first paragraph:\n%s", body)
	}
}

func TestRewriteLinks(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"docs/rules.md", "rules.html"},
		{"./COMMERCIAL.md", "commercial.html"},
		{"BACKLOG.md", "roadmap.html"},
		{"LICENSE", blobURL + "LICENSE"},
		{".github/workflows/backlog.yml", blobURL + ".github/workflows/backlog.yml"},
		{"#install", "#install"},
		{"https://github.com/Allan-Nava/segcheck", "https://github.com/Allan-Nava/segcheck"},
		{"mailto:dev-ops@hiway.media", "mailto:dev-ops@hiway.media"},
	} {
		if got := rewrite(tc.in); got != tc.want {
			t.Errorf("rewrite(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRulesPageCoversTheWholeCatalogue(t *testing.T) {
	catalogue := rules.All()
	page, err := rulesPage(catalogue)
	if err != nil {
		t.Fatalf("rulesPage: %v", err)
	}
	body := string(page.Content)

	// Every rule in the binary has to be on the page: the reference is the
	// catalogue, not a subset somebody remembered to update.
	for _, r := range catalogue {
		if !strings.Contains(body, ">"+r.ID+"</a>") {
			t.Errorf("%s is missing from the reference", r.ID)
		}
		if !strings.Contains(body, r.Title) {
			t.Errorf("%s: title is missing", r.ID)
		}
	}
	if n := strings.Count(body, `class="rule"`); n != len(catalogue) {
		t.Errorf("rule cards = %d, want %d", n, len(catalogue))
	}

	// One section per category, and the sidebar lists them all.
	categories := map[string]bool{}
	for _, r := range catalogue {
		categories[r.Category()] = true
	}
	if n := strings.Count(body, `class="category"`); n != len(categories) {
		t.Errorf("category sections = %d, want %d", n, len(categories))
	}
	if len(page.TOC) != len(categories) {
		t.Errorf("table of contents = %d entries, want %d", len(page.TOC), len(categories))
	}

	// The filter needs a lowercase haystack per card, and the input it hangs off.
	if !strings.Contains(body, `id="rule-filter"`) || !strings.Contains(body, "data-search=") {
		t.Errorf("the rule filter is not wired up:\n%s", body[:min(len(body), 600)])
	}
}

func TestRulesPageRationaleIsEscaped(t *testing.T) {
	page, err := rulesPage(rules.All())
	if err != nil {
		t.Fatalf("rulesPage: %v", err)
	}
	for _, unsafe := range []string{"<script", "onerror="} {
		if strings.Contains(string(page.Content), unsafe) {
			t.Errorf("the reference page carries %q", unsafe)
		}
	}
}
