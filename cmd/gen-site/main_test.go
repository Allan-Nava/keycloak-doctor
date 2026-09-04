package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Allan-Nava/keycloak-doctor/internal/rules"
	"github.com/Allan-Nava/keycloak-doctor/internal/site"
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

func TestSplitSkipsALeadingHTMLBlock(t *testing.T) {
	// The README opens with the logo; the layout renders the mark itself, so the
	// block must not reach the page body.
	title, subtitle, body := split("<img src=\"docs/assets/logo.svg\" width=\"76\" alt=\"\">\n\n# keycloak-doctor\n\n**Audit a Keycloak realm.**\n\n## Install\n")
	if title != "keycloak-doctor" || subtitle != "Audit a Keycloak realm." {
		t.Errorf("title = %q, subtitle = %q", title, subtitle)
	}
	if strings.Contains(body, "<img") {
		t.Errorf("the HTML block leaked into the body:\n%s", body)
	}
}

func TestBrandFilesAreServedWithTheSite(t *testing.T) {
	// The mark lives in the repository and is copied verbatim: the README and the
	// site must not drift onto two different files.
	for name, path := range brandFiles {
		if !strings.HasPrefix(path, "docs/assets/") {
			t.Errorf("%s -> %s: the brand files live under docs/assets/", name, path)
		}
		if strings.TrimSuffix(name, filepath.Ext(name)) == "" || filepath.Ext(name) != filepath.Ext(path) {
			t.Errorf("%s -> %s: the served name and the source must be the same kind of file", name, path)
		}
	}
	if _, ok := brandFiles["favicon.svg"]; !ok {
		t.Error("no favicon among the brand files")
	}
	// The preview image has to be a raster: no crawler and no chat client renders
	// SVG, so an SVG here would mean no preview at all.
	if got := brandFiles[ogImage]; filepath.Ext(got) != ".png" {
		t.Errorf("the link-preview image is %q, want a PNG", got)
	}
	if got := rewrite("assets/logo.svg"); got != "logo.svg" {
		t.Errorf("rewrite(\"assets/logo.svg\") = %q, want the file the site serves", got)
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

func TestRulesPageLaysCardsOutInAGrid(t *testing.T) {
	page, err := rulesPage(rules.All())
	if err != nil {
		t.Fatalf("rulesPage: %v", err)
	}
	body := string(page.Content)
	categories := strings.Count(body, `class="category"`)
	if got := strings.Count(body, `class="rules-grid"`); got != categories {
		t.Errorf("rules-grid containers = %d, want one per category (%d)", got, categories)
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

func TestHomeJSONLDIsValidAndHonest(t *testing.T) {
	var data map[string]any
	if err := json.Unmarshal([]byte(homeJSONLD(30)), &data); err != nil {
		t.Fatalf("the structured data is not valid JSON: %v", err)
	}
	if data["@type"] != "SoftwareApplication" || data["url"] != baseURL {
		t.Errorf("@type = %v, url = %v", data["@type"], data["url"])
	}
	// The licence is noncommercial, and structured data is exactly the place where
	// a "free" claim gets repeated by a crawler without its conditions.
	offer, ok := data["offers"].(map[string]any)
	if !ok {
		t.Fatalf("offers = %v", data["offers"])
	}
	desc, _ := offer["description"].(string)
	if !strings.Contains(desc, "Noncommercial") || !strings.Contains(desc, "Commercial use requires a licence") {
		t.Errorf("the offer does not carry the licence condition: %q", desc)
	}
	if req, _ := data["softwareRequirements"].(string); !strings.Contains(req, "30 rules") {
		t.Errorf("the rule count is not taken from the catalogue: %q", req)
	}
}

func TestNotFoundPageIsUsefulAndUnindexed(t *testing.T) {
	page := notFoundPage()
	if page.Slug != "404" {
		t.Errorf("slug = %q, want 404 (the file name GitHub Pages serves)", page.Slug)
	}
	if !page.NoIndex {
		t.Error("the 404 page must not be indexable")
	}
	// A 404 that only says "not found" wastes the visit.
	for _, want := range []string{"index.html", "rules.html", "ci.html", repoURL} {
		if !strings.Contains(string(page.Content), want) {
			t.Errorf("the 404 page does not offer %s", want)
		}
	}
}

func TestBuildPagesCarrySEOFields(t *testing.T) {
	// buildPages reads the repository, so this test runs from the module root — and
	// puts the working directory back, or every test after it would see a different
	// one.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	pages, err := buildPages(rules.All())
	if err != nil {
		t.Fatalf("buildPages: %v", err)
	}

	bySlug := map[string]site.Page{}
	for _, p := range pages {
		bySlug[p.Slug] = p
	}
	if got := bySlug["changelog"].Lang; got != "it" {
		t.Errorf("the changelog is written in Italian, lang = %q", got)
	}
	if got := bySlug["index"].Lang; got != "" {
		t.Errorf("an English page needs no override, lang = %q", got)
	}
	if bySlug["index"].JSONLD == "" {
		t.Error("the home page carries no structured data")
	}
	if _, ok := bySlug["404"]; !ok {
		t.Error("no 404 page was generated")
	}
	// Every page that has a source on disk gets a date for the sitemap.
	for _, slug := range []string{"index", "ci", "roadmap", "rules"} {
		if bySlug[slug].Modified.IsZero() {
			t.Errorf("%s has no modification time, so the sitemap cannot date it", slug)
		}
	}
}
