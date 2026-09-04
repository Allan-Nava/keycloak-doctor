// Package site renders the documentation site: a handful of static HTML pages,
// no JavaScript framework, no build toolchain, no dependency outside the standard
// library. The site is a projection of what is already in the repository — the
// Markdown documents and the rule catalogue compiled into the binary — the same
// way docs/rules.md is, so it cannot drift into describing a tool that no longer
// exists.
package site

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed assets
var assets embed.FS

// Action is a button in a page's hero.
type Action struct {
	Label   string
	Href    string
	Primary bool
}

// Page is one HTML page of the site.
type Page struct {
	Slug     string        // file name without the extension; "index" is the home page
	Nav      string        // label in the top navigation, empty to keep the page out of it
	Title    string        // browser title and heading of the page
	Subtitle string        // one line under the title, optional
	Content  template.HTML // rendered body
	TOC      []Heading     // headings for the sidebar, level 2 only
	Hero     bool          // render the mark, the title and the actions as a hero band
	Actions  []Action      // hero buttons, ignored unless Hero is set
	// Lang is the BCP 47 tag of the page's own content, "en" when empty. The
	// changelog of this project is written in Italian, and saying so is both an
	// accessibility fact (a screen reader pronounces it) and an indexing one.
	Lang string
	// NoIndex keeps a page out of the sitemap and asks crawlers to skip it — the
	// 404 page is the reason this exists.
	NoIndex bool
	// JSONLD is structured data for this page, already encoded. It goes into a
	// <script type="application/ld+json">.
	JSONLD template.JS
	// Modified is the time the page's source last changed; it becomes the sitemap's
	// lastmod. Zero leaves the entry without one, which is better than a made-up
	// date.
	Modified time.Time
}

// Site is the whole documentation site.
type Site struct {
	Name    string
	Tagline string
	RepoURL string
	// BaseURL is where the site is served from, with a trailing slash. It is what
	// makes the canonical links, the Open Graph tags and the sitemap absolute —
	// they cannot be relative, so without it those are all left out rather than
	// emitted pointing at nothing.
	BaseURL string
	// OGImage is the file name of the link-preview image, served next to the pages.
	OGImage   string
	Pages     []Page
	Generator string // the command that produced the site, shown in the footer
	// Files are extra static files to serve next to the pages, by name. The logo
	// lives in the repository rather than in this package's assets, because the
	// README uses the same file: one mark, one copy of it.
	Files map[string][]byte
}

type pageData struct {
	Site      Site
	Page      Page
	Nav       []navItem
	TOC       []Heading
	Canonical string // absolute URL of this page, empty when the site has no BaseURL
	OGImage   string // absolute URL of the preview image, empty when there is none
	Lang      string // BCP 47, for <html lang>
	OGLocale  string // language_TERRITORY, which is the form Open Graph asks for
}

type navItem struct {
	Label   string
	Href    string
	Current bool
}

// Build writes the site into dir, creating it if needed. It overwrites the pages
// it generates and leaves anything else in the directory alone.
func (s Site) Build(dir string) error {
	tmpl, err := template.ParseFS(assets, "assets/page.html.tmpl")
	if err != nil {
		return fmt.Errorf("parsing the page template: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	for _, p := range s.Pages {
		data := pageData{
			Site:      s,
			Page:      p,
			Nav:       s.nav(p),
			TOC:       sectionHeadings(p.TOC),
			Canonical: s.canonical(p),
			OGImage:   s.ogImage(),
			Lang:      p.lang(),
			OGLocale:  ogLocale(p.lang()),
		}
		f, err := os.Create(filepath.Join(dir, p.Slug+".html")) //nolint:gosec // the slug comes from the generator, not from input
		if err != nil {
			return err
		}
		err = tmpl.Execute(f, data)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("writing %s: %w", p.Slug, err)
		}
	}

	// The static assets ship next to the pages, with relative hrefs, so the site
	// works under the /<repo>/ prefix GitHub Pages serves a project site from.
	static, err := fs.Sub(assets, "assets")
	if err != nil {
		return err
	}
	for _, name := range []string{"style.css", "site.js"} {
		data, err := fs.ReadFile(static, name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil { //nolint:gosec // a public web asset
			return err
		}
	}

	// The crawler-facing files are generated from the pages, so they cannot list a
	// page the site does not serve. Without a BaseURL there is nothing absolute to
	// put in them, and half a sitemap is worse than none.
	if s.BaseURL != "" {
		if err := s.writeFile(dir, sitemapFile, s.writeSitemap); err != nil {
			return err
		}
		if err := s.writeFile(dir, robotsFile, s.writeRobots); err != nil {
			return err
		}
	}

	names := make([]string, 0, len(s.Files))
	for name := range s.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.ContainsRune(name, filepath.Separator) || name == ".." {
			return fmt.Errorf("extra file %q must be a plain file name", name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), s.Files[name], 0o644); err != nil { //nolint:gosec // a public web asset
			return err
		}
	}
	return nil
}

// writeFile creates one generated file with the given writer.
func (s Site) writeFile(dir, name string, write func(io.Writer) error) error {
	f, err := os.Create(filepath.Join(dir, name)) //nolint:gosec // a fixed name in the output directory
	if err != nil {
		return err
	}
	err = write(f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("writing %s: %w", name, err)
	}
	return nil
}

func (s Site) canonical(p Page) string {
	if s.BaseURL == "" {
		return ""
	}
	return p.URL(s.BaseURL)
}

func (s Site) ogImage() string {
	if s.BaseURL == "" || s.OGImage == "" {
		return ""
	}
	return strings.TrimSuffix(s.BaseURL, "/") + "/" + s.OGImage
}

// ogLocale renders a language tag the way Open Graph wants it: language_TERRITORY.
// Only the languages this site is written in need mapping; anything else is
// passed through, because inventing a territory for it would be worse than
// leaving it as the author wrote it.
func ogLocale(lang string) string {
	switch lang {
	case "en":
		return "en_US"
	case "it":
		return "it_IT"
	default:
		return lang
	}
}

func (p Page) lang() string {
	if p.Lang != "" {
		return p.Lang
	}
	return "en"
}

func (s Site) nav(current Page) []navItem {
	var items []navItem
	for _, p := range s.Pages {
		if p.Nav == "" {
			continue
		}
		items = append(items, navItem{Label: p.Nav, Href: p.Slug + ".html", Current: p.Slug == current.Slug})
	}
	return items
}

// sectionHeadings keeps the level-2 headings: a table of contents that lists
// every rule heading of the reference page would be longer than the page.
func sectionHeadings(all []Heading) []Heading {
	var out []Heading
	for _, h := range all {
		if h.Level == 2 {
			out = append(out, h)
		}
	}
	return out
}

// OGType is the Open Graph type: the home page is the site, every other page is
// a document about it.
func (p Page) OGType() string {
	if p.Slug == "index" {
		return "website"
	}
	return "article"
}

// NavTitle is the title of a page as it should read in a browser tab.
func (p Page) NavTitle(siteName string) string {
	if p.Slug == "index" || strings.EqualFold(p.Title, siteName) {
		return siteName
	}
	return p.Title + " · " + siteName
}
