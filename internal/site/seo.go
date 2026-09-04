package site

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// The two files a crawler looks for. They are generated, like every page: a
// sitemap somebody maintains by hand is a sitemap that lists a page the site no
// longer serves.
const (
	sitemapFile = "sitemap.xml"
	robotsFile  = "robots.txt"
)

// URL is the canonical address of a page. The home page is the directory itself
// ("…/keycloak-doctor/") rather than "…/index.html", so the canonical link, the
// sitemap and the og:url all name the address people actually share.
func (p Page) URL(baseURL string) string {
	base := strings.TrimSuffix(baseURL, "/") + "/"
	if p.Slug == "index" {
		return base
	}
	return base + p.Slug + ".html"
}

type urlEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	NS      string     `xml:"xmlns,attr"`
	URLs    []urlEntry `xml:"url"`
}

// writeSitemap lists the indexable pages. A page marked NoIndex is left out — a
// 404 page in a sitemap is an invitation to index it.
func (s Site) writeSitemap(w io.Writer) error {
	set := urlSet{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, p := range s.Pages {
		if p.NoIndex {
			continue
		}
		entry := urlEntry{Loc: p.URL(s.BaseURL)}
		if !p.Modified.IsZero() {
			entry.LastMod = p.Modified.UTC().Format("2006-01-02")
		}
		set.URLs = append(set.URLs, entry)
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(set); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// writeRobots allows everything and points at the sitemap. There is nothing on
// this site to hide from a crawler; the file exists so the sitemap is found
// without anyone submitting it anywhere.
func (s Site) writeRobots(w io.Writer) error {
	_, err := fmt.Fprintf(w, "User-agent: *\nAllow: /\n\nSitemap: %s%s\n",
		strings.TrimSuffix(s.BaseURL, "/")+"/", sitemapFile)
	return err
}
