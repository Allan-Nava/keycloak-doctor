// gen-site writes the documentation site GitHub Pages serves.
//
// Every page is a projection of something already in the repository: the Markdown
// documents at its root, and the rule catalogue compiled into this binary. That is
// the point — a hand-maintained copy of the catalogue on a web page is a copy
// that will eventually describe rules the tool no longer has.
//
//	go run ./cmd/gen-site              # writes ./site
//	go run ./cmd/gen-site --out /tmp/s # anywhere else
//
// The output is static: relative links only, one stylesheet, one small script.
// It works under the /<repo>/ prefix a GitHub Pages project site is served from,
// and it works opened from disk.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Allan-Nava/keycloak-doctor/internal/rules"
	"github.com/Allan-Nava/keycloak-doctor/internal/site"
)

const (
	repoURL = "https://github.com/Allan-Nava/keycloak-doctor"
	blobURL = repoURL + "/blob/main/"
	// baseURL is where GitHub Pages serves this project site from. It is what makes
	// the canonical links, the link previews and the sitemap absolute.
	baseURL   = "https://allan-nava.github.io/keycloak-doctor/"
	ogImage   = "og-card.png"
	siteName  = "keycloak-doctor"
	tagline   = "Audit a Keycloak realm for the mistakes that actually get exploited."
	generator = "go run ./cmd/gen-site"
)

// docPages are the Markdown documents that become pages, in navigation order.
var docPages = []struct {
	slug, nav, file, subtitle string
}{
	{"index", "Overview", "README.md", ""},
	{"ci", "In CI", "docs/ci.md", ""},
	{"roadmap", "Roadmap", "BACKLOG.md", "What is planned, with the id every commit and issue references."},
	{"security", "Security", "SECURITY.md", "What the tool touches, what it never prints, and how to report a vulnerability."},
	{"commercial", "Commercial use", "COMMERCIAL.md", "The licence is noncommercial; this is what a company needs instead."},
	{"brand", "Brand", "docs/brand.md", ""},
	{"changelog", "Changelog", "CHANGELOG.md", "Il changelog di questo progetto è in italiano."},
}

// docLang overrides the language of a page whose content is not in English. A
// screen reader pronounces it, and a crawler indexes it, in the language the page
// claims — so claiming the wrong one is a real defect, not a detail.
var docLang = map[string]string{"changelog": "it"}

// linkTargets maps a relative Markdown link onto the page that serves it. Every
// other relative target is a file in the repository, so it goes to GitHub rather
// than to a 404 on the site.
var linkTargets = map[string]string{
	"README.md":     "index.html",
	"BACKLOG.md":    "roadmap.html",
	"SECURITY.md":   "security.html",
	"COMMERCIAL.md": "commercial.html",
	"CHANGELOG.md":  "changelog.html",
	"docs/rules.md": "rules.html",
	"docs/ci.md":    "ci.html",
	"rules.md":      "rules.html", // docs/ci.md links its sibling relatively
	"docs/brand.md": "brand.html",
	// brand.md links the mark relative to docs/, which resolves on GitHub; the
	// site serves the same files next to its pages.
	"assets/logo.svg":    "logo.svg",
	"assets/favicon.svg": "favicon.svg",
}

// brandFiles are served verbatim next to the pages. They live in the repository
// because the README uses the same files: one mark, one copy of it.
var brandFiles = map[string]string{
	"logo.svg":    "docs/assets/logo.svg",
	"favicon.svg": "docs/assets/favicon.svg",
	// The link-preview image is a PNG on purpose: crawlers and chat clients do not
	// render SVG. docs/assets/og-card.svg is its source, and docs/brand.md has the
	// command that re-renders it.
	ogImage: "docs/assets/og-card.png",
}

func main() {
	out := flag.String("out", "site", "directory to write the site into")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "gen-site — write the documentation site GitHub Pages serves\n\nusage: gen-site [--out DIR]")
		flag.PrintDefaults()
	}
	flag.Parse()

	s := site.Site{
		Name:      siteName,
		Tagline:   tagline,
		RepoURL:   repoURL,
		BaseURL:   baseURL,
		OGImage:   ogImage,
		Generator: generator,
	}

	catalogue := rules.All()
	pages, err := buildPages(catalogue)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-site:", err)
		os.Exit(1)
	}
	s.Pages = pages

	files, err := readBrandFiles()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-site:", err)
		os.Exit(1)
	}
	s.Files = files

	if err := s.Build(*out); err != nil {
		fmt.Fprintln(os.Stderr, "gen-site:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d pages, %d rules)\n", *out, len(s.Pages), len(catalogue))
}

func readBrandFiles() (map[string][]byte, error) {
	files := map[string][]byte{}
	for name, path := range brandFiles {
		data, err := os.ReadFile(path) //nolint:gosec // the file list is fixed above
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w (run this from the repository root)", path, err)
		}
		files[name] = data
	}
	return files, nil
}

func buildPages(catalogue []rules.Rule) ([]site.Page, error) {
	renderer := &site.Renderer{Link: rewrite}

	var pages []site.Page
	for _, d := range docPages {
		md, err := os.ReadFile(d.file) //nolint:gosec // the file list is fixed above
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w (run this from the repository root)", d.file, err)
		}
		title, subtitle, body := split(string(md))
		if d.subtitle != "" {
			subtitle = d.subtitle
		}
		content, headings := renderer.Render(body)
		page := site.Page{
			Slug:     d.slug,
			Nav:      d.nav,
			Title:    title,
			Subtitle: subtitle,
			Content:  content,
			TOC:      headings,
			Lang:     docLang[d.slug],
			Modified: sourceTime(d.file),
		}
		if d.slug == "index" {
			page.JSONLD = homeJSONLD(len(catalogue))
			page.Hero = true
			page.Actions = []site.Action{
				{Label: fmt.Sprintf("Browse the %d rules", len(catalogue)), Href: "rules.html", Primary: true},
				{Label: "Install", Href: "#install"},
				{Label: "Audit an export", Href: "#offline-against-an-export-no-credentials-involved"},
			}
		}
		pages = append(pages, page)

		// The rule reference sits right after the overview in the navigation: it is
		// what a reader comes here for.
		if d.slug == "index" {
			reference, err := rulesPage(catalogue)
			if err != nil {
				return nil, err
			}
			pages = append(pages, reference)
		}
	}
	return append(pages, notFoundPage()), nil
}

// notFoundPage is what GitHub Pages serves for an unknown path. It carries
// NoIndex, which keeps it out of the sitemap: a 404 in a sitemap is an invitation
// to index it.
func notFoundPage() site.Page {
	return site.Page{
		Slug:     "404",
		Title:    "Page not found",
		Subtitle: "That page is not here. It may have moved, or the link may have been to a file in the repository rather than to a page of this site.",
		NoIndex:  true,
		Content: template.HTML(`<ul>` + //nolint:gosec // a fixed string, no input
			`<li><a href="index.html">Start from the overview</a> — what the tool checks and how to install it.</li>` +
			`<li><a href="rules.html">The rule reference</a> — every rule, with the rationale, filterable.</li>` +
			`<li><a href="ci.html">In CI</a> — the action, SARIF, baselines and suppressions.</li>` +
			`<li><a href="` + repoURL + `">The repository</a> — the source, the releases and the issues.</li>` +
			`</ul>`),
	}
}

// homeJSONLD describes the tool for a search engine: what it is, what it costs,
// and where its source is. Nothing here is a claim the page does not already make
// in prose.
func homeJSONLD(rules int) template.JS {
	data := map[string]any{
		"@context":             "https://schema.org",
		"@type":                "SoftwareApplication",
		"name":                 siteName,
		"description":          tagline,
		"url":                  baseURL,
		"applicationCategory":  "DeveloperApplication",
		"operatingSystem":      "Linux, macOS, Windows",
		"softwareRequirements": fmt.Sprintf("A Keycloak realm export or Admin REST API access; %d rules ship in the binary", rules),
		"license":              repoURL + "/blob/main/LICENSE",
		"codeRepository":       repoURL,
		"programmingLanguage":  "Go",
		"image":                baseURL + ogImage,
		"author": map[string]any{
			"@type": "Person",
			"name":  "Allan Nava",
			"url":   "https://github.com/Allan-Nava",
		},
		// Free for the uses the licence allows; a company needs a commercial one, and
		// pretending otherwise in structured data would be a lie a crawler repeats.
		"offers": map[string]any{
			"@type":         "Offer",
			"price":         "0",
			"priceCurrency": "USD",
			"description":   "Free for personal projects, research, education, non-profits and public institutions (PolyForm Noncommercial 1.0.0). Commercial use requires a licence.",
			"url":           baseURL + "commercial.html",
		},
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		// The map is a literal: an error here would be a programming mistake, and a
		// page without structured data is better than a page with broken data.
		fmt.Fprintln(os.Stderr, "gen-site: skipping the structured data:", err)
		return ""
	}
	return template.JS(encoded) //nolint:gosec // encoded by encoding/json above
}

// newestOf is the most recent modification time in a glob. The rule reference has
// no Markdown source — it is generated from the catalogue — so its date comes from
// the code that defines the rules.
func newestOf(pattern string) time.Time {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return time.Time{}
	}
	var newest time.Time
	for _, m := range matches {
		if t := sourceTime(m); t.After(newest) {
			newest = t
		}
	}
	return newest
}

// sourceTime is the modification time of a page's source, for the sitemap's
// lastmod. In a CI checkout every file carries the clone time, so lastmod becomes
// the day of the deploy — true, if less precise than a commit date.
func sourceTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// split peels the leading "# Title" and the bold one-liner under it off a
// document, so they become the page's heading and subtitle instead of being
// repeated inside the body.
func split(md string) (title, subtitle, body string) {
	lines := strings.Split(md, "\n")
	i := 0
	for ; i < len(lines) && strings.TrimSpace(lines[i]) == ""; i++ {
	}
	// A document may open with an HTML block — the README leads with the logo. The
	// layout shows the mark in its own header, so the block is skipped here.
	for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "<"); i++ {
	}
	for ; i < len(lines) && strings.TrimSpace(lines[i]) == ""; i++ {
	}
	if i < len(lines) && strings.HasPrefix(lines[i], "# ") {
		title = strings.TrimSpace(strings.TrimPrefix(lines[i], "# "))
		i++
	}
	for ; i < len(lines) && strings.TrimSpace(lines[i]) == ""; i++ {
	}
	if i < len(lines) {
		if t := strings.TrimSpace(lines[i]); strings.HasPrefix(t, "**") && strings.HasSuffix(t, "**") {
			subtitle = strings.Trim(t, "*")
			i++
		}
	}
	// The title and the subtitle are rendered by the layout, so the body starts at
	// the first real section: repeating them inside it would show them twice.
	return title, subtitle, strings.Join(lines[i:], "\n")
}

func rewrite(target string) string {
	switch {
	case strings.HasPrefix(target, "#"), strings.Contains(target, "://"), strings.HasPrefix(target, "mailto:"):
		return target
	}
	clean := strings.TrimPrefix(target, "./")
	if page, ok := linkTargets[clean]; ok {
		return page
	}
	// A relative link to anything else — LICENSE, a workflow, a source file — is a
	// link into the repository.
	return blobURL + clean
}

type ruleView struct {
	ID        string
	Slug      string
	Title     string
	Rationale string
	Search    string
}

type categoryView struct {
	Name  string
	Slug  string
	Rules []ruleView
}

var rulesBody = template.Must(template.New("rules").Parse(`
<p>keycloak-doctor ships <strong>{{ .Total }} rules</strong> in {{ len .Categories }} categories. Every rule is evaluated against a realm read either from an export file or from the Admin REST API, and every id below is stable: it is what <code>--only</code>, <code>--skip</code> and any suppression in your pipeline should pin.</p>

<ul class="severity">
  <li class="bad"><code>BAD</code> exploitable as configured</li>
  <li class="warn"><code>WARN</code> a weakening you should be able to justify</li>
  <li class="ok"><code>OK</code> the rule ran and passed</li>
  <li class="error"><code>ERROR</code> the rule could not run — a blind spot, never a clean bill</li>
</ul>

<div class="rule-search">
  <input id="rule-filter" type="search" placeholder="Filter by id, title or rationale…" aria-label="Filter rules" autocomplete="off" hidden>
  <span class="count" id="rule-count">{{ .Total }} rules</span>
</div>
{{ range .Categories }}
<section class="category">
  <h2 id="{{ .Slug }}"><code>{{ .Name }}</code> <span class="category-count">{{ len .Rules }} rule{{ if ne (len .Rules) 1 }}s{{ end }}</span></h2>
  <div class="rules-grid">
  {{ range .Rules }}
  <article class="rule" id="{{ .Slug }}" data-search="{{ .Search }}">
    <h3><a href="#{{ .Slug }}">{{ .ID }}</a></h3>
    <p class="title">{{ .Title }}</p>
    <p class="rationale">{{ .Rationale }}</p>
  </article>
  {{ end }}
  </div>
</section>
{{ end }}
`))

func rulesPage(catalogue []rules.Rule) (site.Page, error) {
	byCategory := map[string][]ruleView{}
	for _, r := range catalogue {
		byCategory[r.Category()] = append(byCategory[r.Category()], ruleView{
			ID:        r.ID,
			Slug:      site.Slug(r.ID),
			Title:     r.Title,
			Rationale: r.Rationale,
			Search:    strings.ToLower(r.ID + " " + r.Title + " " + r.Rationale),
		})
	}
	names := make([]string, 0, len(byCategory))
	for name := range byCategory {
		names = append(names, name)
	}
	sort.Strings(names)

	categories := make([]categoryView, 0, len(names))
	toc := make([]site.Heading, 0, len(names))
	for _, name := range names {
		categories = append(categories, categoryView{Name: name, Slug: site.Slug(name), Rules: byCategory[name]})
		toc = append(toc, site.Heading{
			Level: 2,
			Text:  template.HTML("<code>" + template.HTMLEscapeString(name) + "</code>"), //nolint:gosec // escaped above
			ID:    site.Slug(name),
		})
	}

	b := &strings.Builder{}
	data := struct {
		Total      int
		Categories []categoryView
	}{Total: len(catalogue), Categories: categories}
	if err := rulesBody.Execute(b, data); err != nil {
		return site.Page{}, fmt.Errorf("rendering the rule reference: %w", err)
	}

	return site.Page{
		Slug:     "rules",
		Nav:      "Rules",
		Title:    "Rule reference",
		Modified: newestOf("internal/rules/*.go"),
		Subtitle: "The catalogue compiled into the binary, with the rationale for each rule.",
		Content:  template.HTML(b.String()), //nolint:gosec // rendered by html/template above
		TOC:      toc,
	}, nil
}
