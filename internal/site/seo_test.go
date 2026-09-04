package site

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seoSite() Site {
	s := testSite()
	s.BaseURL = "https://allan-nava.github.io/keycloak-doctor/"
	s.OGImage = "og-card.png"
	s.Files["og-card.png"] = []byte("PNG")
	s.Pages[0].Modified = time.Date(2026, 8, 17, 9, 30, 0, 0, time.UTC)
	s.Pages[1].Lang = "it"
	s.Pages[2].NoIndex = true // the "hidden" page of the fixture stands in for the 404
	return s
}

func buildSEO(t *testing.T) (string, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	if err := seoSite().Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	files := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(data)
	}
	return dir, files
}

func TestSitemapListsTheIndexablePages(t *testing.T) {
	_, files := buildSEO(t)

	raw, ok := files["sitemap.xml"]
	if !ok {
		t.Fatal("no sitemap.xml")
	}
	var set struct {
		URLs []struct {
			Loc     string `xml:"loc"`
			LastMod string `xml:"lastmod"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal([]byte(raw), &set); err != nil {
		t.Fatalf("the sitemap is not valid XML: %v\n%s", err, raw)
	}

	var locs []string
	for _, u := range set.URLs {
		locs = append(locs, u.Loc)
	}
	got := strings.Join(locs, " ")
	// The home page is the directory, not index.html: that is the address people
	// share, and the canonical link has to agree with the sitemap.
	want := "https://allan-nava.github.io/keycloak-doctor/ https://allan-nava.github.io/keycloak-doctor/rules.html"
	if got != want {
		t.Errorf("sitemap = %q, want %q (and nothing marked NoIndex)", got, want)
	}
	if set.URLs[0].LastMod != "2026-08-17" {
		t.Errorf("lastmod = %q, want the source's date", set.URLs[0].LastMod)
	}
	// A page with no known source date gets no invented one.
	if set.URLs[1].LastMod != "" {
		t.Errorf("lastmod = %q, want empty for a page with no source time", set.URLs[1].LastMod)
	}
}

func TestRobotsPointsAtTheSitemap(t *testing.T) {
	_, files := buildSEO(t)
	robots, ok := files["robots.txt"]
	if !ok {
		t.Fatal("no robots.txt")
	}
	for _, want := range []string{"User-agent: *", "Allow: /", "Sitemap: https://allan-nava.github.io/keycloak-doctor/sitemap.xml"} {
		if !strings.Contains(robots, want) {
			t.Errorf("robots.txt is missing %q:\n%s", want, robots)
		}
	}
}

// Without a BaseURL there is nothing absolute to put in them, and half a sitemap
// is worse than none.
func TestNoBaseURLMeansNoCrawlerFiles(t *testing.T) {
	s := seoSite()
	s.BaseURL = ""
	dir := t.TempDir()
	if err := s.Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, name := range []string{"sitemap.xml", "robots.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s was written without a BaseURL", name)
		}
	}
	page, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"rel=\"canonical\"", "og:url", "og:image"} {
		if strings.Contains(string(page), unwanted) {
			t.Errorf("%s was emitted with nothing absolute to point at", unwanted)
		}
	}
}

func TestCanonicalAndPreviewPerPage(t *testing.T) {
	_, files := buildSEO(t)

	for page, canonical := range map[string]string{
		"index.html": "https://allan-nava.github.io/keycloak-doctor/",
		"rules.html": "https://allan-nava.github.io/keycloak-doctor/rules.html",
	} {
		if !strings.Contains(files[page], `<link rel="canonical" href="`+canonical+`">`) {
			t.Errorf("%s has the wrong canonical, want %s", page, canonical)
		}
		if !strings.Contains(files[page], `<meta property="og:url" content="`+canonical+`">`) {
			t.Errorf("%s: og:url and the canonical must agree", page)
		}
	}

	index := files["index.html"]
	for _, want := range []string{
		`<meta property="og:type" content="website">`,
		`<meta property="og:image" content="https://allan-nava.github.io/keycloak-doctor/og-card.png">`,
		`<meta property="og:image:width" content="1200">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="theme-color" content="#0b3a66">`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("index.html is missing %s", want)
		}
	}
	// Every other page is a document about the tool, not the site itself.
	if !strings.Contains(files["rules.html"], `<meta property="og:type" content="article">`) {
		t.Error("a documentation page should be an article, not the website")
	}
	// The preview image is served next to the pages, or the tag points at nothing.
	if files["og-card.png"] != "PNG" {
		t.Error("the preview image was not written next to the pages")
	}
}

func TestPageLanguage(t *testing.T) {
	_, files := buildSEO(t)
	if !strings.Contains(files["rules.html"], `<html lang="it">`) ||
		!strings.Contains(files["rules.html"], `content="it_IT"`) {
		t.Error("a page whose content is not English must say so, in both forms")
	}
	if !strings.Contains(files["index.html"], `<html lang="en">`) {
		t.Error("the default language is en")
	}
}

func TestNoIndexPageIsMarkedAndExcluded(t *testing.T) {
	_, files := buildSEO(t)
	if !strings.Contains(files["hidden.html"], `<meta name="robots" content="noindex, follow">`) {
		t.Error("a NoIndex page does not ask crawlers to skip it")
	}
	if strings.Contains(files["sitemap.xml"], "hidden.html") {
		t.Error("a NoIndex page reached the sitemap")
	}
	if strings.Contains(files["index.html"], `content="noindex`) {
		t.Error("an indexable page was marked noindex")
	}
}

func TestPageURL(t *testing.T) {
	for _, tc := range []struct{ slug, base, want string }{
		{"index", "https://example.test/kd/", "https://example.test/kd/"},
		{"rules", "https://example.test/kd/", "https://example.test/kd/rules.html"},
		// A base without its trailing slash must not swallow the page name.
		{"rules", "https://example.test/kd", "https://example.test/kd/rules.html"},
	} {
		if got := (Page{Slug: tc.slug}).URL(tc.base); got != tc.want {
			t.Errorf("URL(%q) for %q = %q, want %q", tc.base, tc.slug, got, tc.want)
		}
	}
}
