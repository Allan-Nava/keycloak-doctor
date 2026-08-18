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
	"flag"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/rules"
	"github.com/Allan-Nava/keycloak-doctor/internal/site"
)

const (
	repoURL   = "https://github.com/Allan-Nava/keycloak-doctor"
	blobURL   = repoURL + "/blob/main/"
	siteName  = "keycloak-doctor"
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
		Tagline:   "Audit a Keycloak realm for the mistakes that actually get exploited.",
		RepoURL:   repoURL,
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
		}
		if d.slug == "index" {
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
	return pages, nil
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
		Subtitle: "The catalogue compiled into the binary, with the rationale for each rule.",
		Content:  template.HTML(b.String()), //nolint:gosec // rendered by html/template above
		TOC:      toc,
	}, nil
}
