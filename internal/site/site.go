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
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed assets
var assets embed.FS

// Page is one HTML page of the site.
type Page struct {
	Slug     string        // file name without the extension; "index" is the home page
	Nav      string        // label in the top navigation, empty to keep the page out of it
	Title    string        // browser title and heading of the page
	Subtitle string        // one line under the title, optional
	Content  template.HTML // rendered body
	TOC      []Heading     // headings for the sidebar, level 2 only
}

// Site is the whole documentation site.
type Site struct {
	Name      string
	Tagline   string
	RepoURL   string
	Pages     []Page
	Generator string // the command that produced the site, shown in the footer
	// Files are extra static files to serve next to the pages, by name. The logo
	// lives in the repository rather than in this package's assets, because the
	// README uses the same file: one mark, one copy of it.
	Files map[string][]byte
}

type pageData struct {
	Site    Site
	Page    Page
	Nav     []navItem
	TOC     []Heading
	Version string
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
		data := pageData{Site: s, Page: p, Nav: s.nav(p), TOC: sectionHeadings(p.TOC)}
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

// NavTitle is the title of a page as it should read in a browser tab.
func (p Page) NavTitle(siteName string) string {
	if p.Slug == "index" || strings.EqualFold(p.Title, siteName) {
		return siteName
	}
	return p.Title + " · " + siteName
}
