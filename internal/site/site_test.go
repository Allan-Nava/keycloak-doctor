package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSite() Site {
	return Site{
		Name:      "keycloak-doctor",
		Tagline:   "Audit a Keycloak realm.",
		RepoURL:   "https://github.com/Allan-Nava/keycloak-doctor",
		Generator: "go run ./cmd/gen-site",
		Files:     map[string][]byte{"logo.svg": []byte("<svg/>"), "favicon.svg": []byte("<svg/>")},
		Pages: []Page{
			{Slug: "index", Nav: "Overview", Title: "keycloak-doctor", Subtitle: "One binary.", Content: "<p>home</p>"},
			{Slug: "rules", Nav: "Rules", Title: "Rule reference", Content: "<p>rules</p>", TOC: []Heading{
				{Level: 2, Text: "<code>client</code>", ID: "client"},
				{Level: 3, Text: "client/pkce", ID: "client-pkce"},
			}},
			{Slug: "hidden", Title: "Not in the navigation", Content: "<p>hidden</p>"},
		},
	}
}

func build(t *testing.T) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	if err := testSite().Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	pages := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		pages[e.Name()] = string(data)
	}
	return dir, pages
}

func TestBuildWritesEveryPageAndAsset(t *testing.T) {
	_, files := build(t)
	for _, name := range []string{"index.html", "rules.html", "hidden.html", "style.css", "site.js", "logo.svg", "favicon.svg"} {
		if _, ok := files[name]; !ok {
			t.Errorf("%s was not written", name)
		}
	}
	if len(files) != 7 {
		t.Errorf("files = %v, want exactly the three pages, the two assets and the two brand files", files)
	}
	if files["logo.svg"] != "<svg/>" {
		t.Errorf("logo.svg was rewritten: %q", files["logo.svg"])
	}
}

// The mark identifies the tool in a browser tab and in the header of every page.
func TestBrandIsWiredIntoEveryPage(t *testing.T) {
	_, files := build(t)
	for name, page := range files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		if !strings.Contains(page, `<link rel="icon" href="favicon.svg"`) {
			t.Errorf("%s has no favicon", name)
		}
		if !strings.Contains(page, `<img src="logo.svg"`) {
			t.Errorf("%s has no mark in the header", name)
		}
	}
}

// An extra file has to stay next to the pages: a name with a path in it would let
// the generator write outside the output directory.
func TestBuildRejectsAPathAsAnExtraFileName(t *testing.T) {
	s := testSite()
	s.Files = map[string][]byte{filepath.Join("..", "escaped.svg"): []byte("<svg/>")}
	if err := s.Build(t.TempDir()); err == nil {
		t.Fatal("Build accepted an extra file name with a path in it")
	}
}

func TestNavMarksTheCurrentPageAndSkipsUnlisted(t *testing.T) {
	_, files := build(t)

	if !strings.Contains(files["rules.html"], `<a href="rules.html" class="current" aria-current="page">Rules</a>`) {
		t.Errorf("the current page is not marked in the navigation:\n%s", files["rules.html"])
	}
	if strings.Contains(files["index.html"], `href="hidden.html"`) {
		t.Error("a page without a Nav label appeared in the navigation")
	}
	// Relative hrefs only: a project site is served under /<repo>/, not at the root.
	for name, page := range files {
		if strings.Contains(page, `href="/`) || strings.Contains(page, `src="/`) {
			t.Errorf("%s has an absolute local reference, which breaks under the project prefix", name)
		}
	}
}

func TestTOCKeepsSectionsOnly(t *testing.T) {
	_, files := build(t)
	rules := files["rules.html"]

	if !strings.Contains(rules, `<a href="#client">`) {
		t.Errorf("the level-2 heading is missing from the table of contents:\n%s", rules)
	}
	if strings.Contains(rules, `<a href="#client-pkce">`) {
		t.Error("a level-3 heading reached the table of contents, which would make it longer than the page")
	}
	// A page with no headings gets no sidebar at all.
	if !strings.Contains(files["index.html"], `class="layout no-toc"`) {
		t.Errorf("a page without headings still reserved the sidebar column:\n%s", files["index.html"])
	}
}

// The home page opens on a hero: the mark, the name, the tagline and the actions
// a first-time reader needs — and the title must not then repeat below it.
func TestHeroReplacesThePageHeading(t *testing.T) {
	s := testSite()
	s.Pages[0].Hero = true
	s.Pages[0].Actions = []Action{
		{Label: "Browse the 30 rules", Href: "rules.html", Primary: true},
		{Label: "Install", Href: "#install"},
	}
	dir := t.TempDir()
	if err := s.Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)

	for _, want := range []string{
		`<section class="hero">`,
		`class="hero-mark" src="logo.svg"`,
		`<p class="hero-tagline">One binary.</p>`,
		`<a class="btn btn-primary" href="rules.html">Browse the 30 rules</a>`,
		`<a class="btn" href="#install">Install</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q in the hero page", want)
		}
	}
	if n := strings.Count(page, "<h1>"); n != 1 {
		t.Errorf("<h1> appears %d times, want once (the hero owns it)", n)
	}
	if strings.Contains(page, `<p class="subtitle">`) {
		t.Error("the subtitle is rendered twice: once in the hero and once in the page")
	}

	// A page without a hero keeps the plain heading.
	rules, err := os.ReadFile(filepath.Join(dir, "rules.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rules), "<h1>Rule reference</h1>") {
		t.Error("a page without a hero lost its heading")
	}
}

func TestPageTitles(t *testing.T) {
	_, files := build(t)
	contains := func(name, want string) {
		if !strings.Contains(files[name], want) {
			t.Errorf("%s: missing %q", name, want)
		}
	}
	contains("index.html", "<title>keycloak-doctor</title>")
	contains("rules.html", "<title>Rule reference · keycloak-doctor</title>")
	contains("index.html", `<p class="subtitle">One binary.</p>`)
	// The description falls back to the site tagline when a page has no subtitle.
	contains("rules.html", `content="Audit a Keycloak realm."`)
}

func TestBuildCreatesTheOutputDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "site")
	if err := testSite().Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		t.Errorf("index.html: %v", err)
	}
}
