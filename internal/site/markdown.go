package site

import (
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
)

// The renderer covers the Markdown this repository actually writes — headings,
// paragraphs, fenced code, tables, nested lists, thematic breaks, link reference
// definitions, and inline code/bold/italic/links. That is a deliberate ceiling,
// not an unfinished job: the input is our own documentation, and a hand-written
// subset keeps the site a zero-dependency `go run` instead of a toolchain.
var (
	headingRe   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	bulletRe    = regexp.MustCompile(`^(\s*)[-*]\s+(.*)$`)
	orderedRe   = regexp.MustCompile(`^(\s*)[0-9]+[.)]\s+(.*)$`)
	tableSepRe  = regexp.MustCompile(`^\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?$`)
	refDefRe    = regexp.MustCompile(`^\[([^\]]+)\]:\s*(\S+)\s*$`)
	linkRe      = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`)
	refLinkRe   = regexp.MustCompile(`\[([^\]]+)\](?:\[([^\]]*)\])?`)
	autolinkRe  = regexp.MustCompile(`&lt;((?:https?://|mailto:)[^\s&]+)&gt;`)
	boldRe      = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe    = regexp.MustCompile(`(^|[\s(])\*([^*\s][^*]*)\*`)
	slugStripRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// Heading is one heading of a rendered page, in document order, so a page can
// carry its own table of contents.
type Heading struct {
	Level int
	Text  template.HTML
	ID    string
}

// Renderer converts Markdown to the HTML the site serves.
type Renderer struct {
	// Link rewrites a link target. A relative Markdown link points at a file in
	// the repository, which the site does not serve, so every page needs a say in
	// where such a link should land.
	Link func(target string) string

	headings []Heading
	refs     map[string]string
}

// Render returns the HTML of a Markdown document and the headings it contains.
func (r *Renderer) Render(md string) (template.HTML, []Heading) {
	r.headings = nil
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	lines = r.collectRefs(lines)

	b := &strings.Builder{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
		case strings.HasPrefix(trimmed, "```"):
			i = r.codeBlock(b, lines, i)
		case headingRe.MatchString(trimmed):
			r.heading(b, trimmed)
		case trimmed == "---" || trimmed == "***":
			b.WriteString("<hr>\n")
		case strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && tableSepRe.MatchString(strings.TrimSpace(lines[i+1])):
			i = r.table(b, lines, i)
		case bulletRe.MatchString(line), orderedRe.MatchString(line):
			i = r.list(b, lines, i)
		default:
			i = r.paragraph(b, lines, i)
		}
	}
	return template.HTML(b.String()), r.headings //nolint:gosec // the renderer is the escaper
}

// collectRefs peels off the link reference definitions — the block of
// "[0.1.1]: https://…" lines a changelog ends with — so `[0.1.1]` in a heading
// renders as a link instead of as literal brackets.
func (r *Renderer) collectRefs(lines []string) []string {
	r.refs = map[string]string{}
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if m := refDefRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			r.refs[strings.ToLower(m[1])] = m[2]
			continue
		}
		kept = append(kept, line)
	}
	return kept
}

func (r *Renderer) heading(b *strings.Builder, line string) {
	m := headingRe.FindStringSubmatch(line)
	level, text := len(m[1]), strings.TrimSpace(m[2])
	id := Slug(text)
	inline := r.inline(text)
	r.headings = append(r.headings, Heading{Level: level, Text: template.HTML(inline), ID: id}) //nolint:gosec // the renderer is the escaper
	fmt.Fprintf(b, "<h%d id=%q>%s<a class=\"anchor\" href=\"#%s\" aria-label=\"Permalink\">#</a></h%d>\n", level, id, inline, id, level)
}

func (r *Renderer) codeBlock(b *strings.Builder, lines []string, start int) int {
	lang := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), "```"))
	var body []string
	i := start + 1
	for ; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
		body = append(body, lines[i])
	}
	class := ""
	if lang != "" {
		class = fmt.Sprintf(" class=%q", "language-"+Slug(lang))
	}
	fmt.Fprintf(b, "<pre><code%s>%s</code></pre>\n", class, html.EscapeString(strings.Join(body, "\n")))
	return i
}

func (r *Renderer) table(b *strings.Builder, lines []string, start int) int {
	// A table is the one block that can be wider than the page, so it scrolls in
	// its own box rather than making the whole document scroll sideways.
	b.WriteString("<div class=\"table-wrap\">\n<table>\n<thead>\n<tr>")
	for _, c := range tableCells(lines[start]) {
		fmt.Fprintf(b, "<th>%s</th>", r.inline(c))
	}
	b.WriteString("</tr>\n</thead>\n<tbody>\n")

	i := start + 2
	for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
		b.WriteString("<tr>")
		for _, c := range tableCells(lines[i]) {
			fmt.Fprintf(b, "<td>%s</td>", r.inline(c))
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n</div>\n")
	return i - 1
}

func tableCells(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimSuffix(strings.TrimPrefix(t, "|"), "|")
	cells := strings.Split(t, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func (r *Renderer) list(b *strings.Builder, lines []string, start int) int {
	end := start
	for end+1 < len(lines) {
		next := lines[end+1]
		if strings.TrimSpace(next) == "" {
			break
		}
		if !bulletRe.MatchString(next) && !orderedRe.MatchString(next) && !strings.HasPrefix(next, "  ") {
			break
		}
		end++
	}
	r.renderList(b, lines[start:end+1], indentOf(lines[start]))
	return end
}

// renderList renders one level of a list and recurses into the deeper ones. A
// line that is neither a marker nor deeper than this level is a wrapped item and
// is folded back into the item's text.
func (r *Renderer) renderList(b *strings.Builder, block []string, indent int) {
	tag := "ul"
	if orderedRe.MatchString(block[0]) {
		tag = "ol"
	}
	fmt.Fprintf(b, "<%s>\n", tag)

	var item []string
	flush := func() {
		if len(item) == 0 {
			return
		}
		var nested, wrapped []string
		for _, l := range item[1:] {
			if bulletRe.MatchString(l) || orderedRe.MatchString(l) {
				nested = append(nested, l)
				continue
			}
			wrapped = append(wrapped, strings.TrimSpace(l))
		}
		text := item[0]
		if len(wrapped) > 0 {
			text += " " + strings.Join(wrapped, " ")
		}
		b.WriteString("<li>" + r.inline(text))
		if len(nested) > 0 {
			b.WriteString("\n")
			r.renderList(b, nested, indentOf(nested[0]))
		}
		b.WriteString("</li>\n")
		item = nil
	}

	for _, l := range block {
		if m := markerText(l); m != "" && indentOf(l) <= indent {
			flush()
			item = append(item, m)
			continue
		}
		item = append(item, l)
	}
	flush()
	fmt.Fprintf(b, "</%s>\n", tag)
}

// markerText returns the text of a list item, or "" when the line is not one.
func markerText(line string) string {
	if m := bulletRe.FindStringSubmatch(line); m != nil {
		return m[2]
	}
	if m := orderedRe.FindStringSubmatch(line); m != nil {
		return m[2]
	}
	return ""
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func (r *Renderer) paragraph(b *strings.Builder, lines []string, start int) int {
	var body []string
	i := start
	for ; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "```") || headingRe.MatchString(trimmed) ||
			strings.HasPrefix(trimmed, "|") || bulletRe.MatchString(lines[i]) || orderedRe.MatchString(lines[i]) {
			break
		}
		body = append(body, trimmed)
	}
	fmt.Fprintf(b, "<p>%s</p>\n", r.inline(strings.Join(body, " ")))
	return i - 1
}

// inline renders the span-level syntax. Code spans are lifted out first, so a
// `**` inside backticks stays literal, and put back last, so a bold or a link
// that *wraps* a code span still renders — "**Milestone `v0.2.0`**" is one span
// in the sources of this repository, not two. Everything outside a code span is
// HTML-escaped before any tag is introduced.
func (r *Renderer) inline(s string) string {
	var codes []string
	if strings.Count(s, "`")%2 == 0 { // an odd count is an unclosed span: leave the backticks as text
		b := &strings.Builder{}
		for i, part := range strings.Split(s, "`") {
			if i%2 == 1 {
				fmt.Fprintf(b, "\x00%d\x00", len(codes))
				codes = append(codes, "<code>"+html.EscapeString(part)+"</code>")
				continue
			}
			b.WriteString(part)
		}
		s = b.String()
	}

	out := r.spans(html.EscapeString(s))
	for i, code := range codes {
		out = strings.ReplaceAll(out, fmt.Sprintf("\x00%d\x00", i), code)
	}
	return out
}

func (r *Renderer) spans(s string) string {
	// An autolink is written <https://…>; by the time spans runs, the brackets are
	// already escaped entities, which is what the pattern matches.
	s = autolinkRe.ReplaceAllStringFunc(s, func(m string) string {
		target := autolinkRe.FindStringSubmatch(m)[1]
		return fmt.Sprintf("<a href=%q>%s</a>", r.href(target), target)
	})
	s = linkRe.ReplaceAllStringFunc(s, func(m string) string {
		p := linkRe.FindStringSubmatch(m)
		return fmt.Sprintf("<a href=%q>%s</a>", r.href(p[2]), p[1])
	})
	s = refLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		p := refLinkRe.FindStringSubmatch(m)
		key := p[2]
		if key == "" {
			key = p[1]
		}
		target, ok := r.refs[strings.ToLower(key)]
		if !ok {
			return m // not a reference: leave the brackets as the author wrote them
		}
		return fmt.Sprintf("<a href=%q>%s</a>", r.href(target), p[1])
	})
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "$1<em>$2</em>")
	return s
}

func (r *Renderer) href(target string) string {
	if r.Link == nil {
		return target
	}
	return r.Link(target)
}

// Slug is the id form of a heading: lowercase, alphanumerics and hyphens. It is
// what an in-page anchor and the table of contents agree on.
func Slug(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "`", ""))
	return strings.Trim(slugStripRe.ReplaceAllString(s, "-"), "-")
}
