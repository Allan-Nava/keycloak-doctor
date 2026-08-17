package site

import (
	"strings"
	"testing"
)

func render(t *testing.T, md string) string {
	t.Helper()
	r := &Renderer{Link: func(target string) string {
		if strings.HasSuffix(target, ".md") {
			return "rewritten.html"
		}
		return target
	}}
	html, _ := r.Render(md)
	return string(html)
}

func contains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestHeadingsCarryAnchors(t *testing.T) {
	r := &Renderer{}
	html, headings := r.Render("## Use it\n\n### In CI\n")
	contains(t, string(html), `<h2 id="use-it">`, `href="#use-it"`, `<h3 id="in-ci">`)

	if len(headings) != 2 {
		t.Fatalf("headings = %d, want 2", len(headings))
	}
	if headings[0].Level != 2 || headings[0].ID != "use-it" {
		t.Errorf("first heading = %+v", headings[0])
	}
	if headings[1].Level != 3 {
		t.Errorf("second heading level = %d, want 3", headings[1].Level)
	}
}

func TestCodeBlockIsEscapedAndTagged(t *testing.T) {
	got := render(t, "```bash\nkeycloak-doctor audit realm.json --output json | jq '.worst'\n```\n")
	contains(t, got, `<pre><code class="language-bash">`, "jq &#39;.worst&#39;")
	if strings.Contains(got, "<code><code>") {
		t.Errorf("nested code tags:\n%s", got)
	}
}

func TestTableScrollsInItsOwnBox(t *testing.T) {
	got := render(t, "| Category | Rules |\n|---|---|\n| `realm` | 13 |\n| `client` | 8 |\n")
	contains(t, got, `<div class="table-wrap">`, "<th>Category</th>", "<td><code>realm</code></td>", "<td>8</td>")
	if n := strings.Count(got, "<tr>"); n != 3 {
		t.Errorf("rows = %d, want 3 (header + 2)", n)
	}
}

func TestNestedAndOrderedLists(t *testing.T) {
	got := render(t, "- top level\n  - nested one\n  - nested two\n- second top\n")
	contains(t, got, "<ul>", "<li>top level", "<li>nested one</li>", "<li>second top</li>")
	if n := strings.Count(got, "<ul>"); n != 2 {
		t.Errorf("ul count = %d, want 2 (outer + nested):\n%s", n, got)
	}

	ordered := render(t, "1. first\n2. second\n")
	contains(t, ordered, "<ol>", "<li>first</li>", "<li>second</li>")

	// A wrapped continuation line belongs to the item above it, not to a new one.
	wrapped := render(t, "- an item that continues\n  on the next line\n")
	contains(t, wrapped, "<li>an item that continues on the next line</li>")
}

func TestInlineSpans(t *testing.T) {
	got := render(t, "A **bold** and *italic* and `code` line with a [link](docs/rules.md).\n")
	contains(t, got, "<strong>bold</strong>", "<em>italic</em>", "<code>code</code>", `<a href="rewritten.html">link</a>`)

	// Bold wrapping a code span is how this repository writes changelog entries.
	contains(t, render(t, "- **Milestone `v0.2.0`** — scope.\n"), "<strong>Milestone <code>v0.2.0</code></strong>")

	// Markdown inside a code span stays literal.
	literal := render(t, "The shape is `- **KD-n** — description`.\n")
	contains(t, literal, "<code>- **KD-n** — description</code>")
	if strings.Contains(literal, "<strong>KD-n</strong>") {
		t.Errorf("markdown inside a code span was interpreted:\n%s", literal)
	}

	// An unclosed span is text, not a swallowed rest-of-line.
	contains(t, render(t, "a stray ` backtick\n"), "a stray ` backtick")
}

func TestReferenceLinks(t *testing.T) {
	got := render(t, "## [0.1.1] - 2026-08-17\n\nSee [the release][0.1.1] and [nothing] here.\n\n[0.1.1]: https://example.test/releases/v0.1.1\n")
	contains(t, got, `<a href="https://example.test/releases/v0.1.1">0.1.1</a>`, `<a href="https://example.test/releases/v0.1.1">the release</a>`)
	// The definition line itself must not render as a paragraph.
	if strings.Contains(got, "<p>[0.1.1]:") {
		t.Errorf("reference definition leaked into the page:\n%s", got)
	}
	// Brackets that are not a reference stay as the author typed them.
	contains(t, got, "[nothing]")
}

// The documents are ours, but the renderer is still the only escaper between a
// Markdown file and the served page: nothing may pass through as live HTML.
func TestHTMLIsEscapedEverywhere(t *testing.T) {
	for _, md := range []string{
		"A paragraph with <script>alert(1)</script>.\n",
		"## A heading with <img src=x onerror=alert(1)>\n",
		"- an item with <script>alert(1)</script>\n",
		"| head |\n|---|\n| <script>alert(1)</script> |\n",
		"```\n<script>alert(1)</script>\n```\n",
		"An [xss](javascript:alert&#40;1&#41;) link.\n",
	} {
		// The tag has to arrive as text: the angle brackets are what makes it live
		// HTML, and they are the thing that must be gone.
		if got := render(t, md); strings.Contains(got, "<script") || strings.Contains(got, "<img") {
			t.Errorf("unescaped HTML from %q:\n%s", md, got)
		}
	}
}

func TestThematicBreakAndParagraphs(t *testing.T) {
	got := render(t, "first paragraph\nsame paragraph\n\n---\n\nsecond paragraph\n")
	contains(t, got, "<p>first paragraph same paragraph</p>", "<hr>", "<p>second paragraph</p>")
}

func TestSlug(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Use it", "use-it"},
		{"`client/pkce`", "client-pkce"},
		{"What it checks", "what-it-checks"},
		{"[0.1.1] - 2026-08-17", "0-1-1-2026-08-17"},
		{"---", ""},
	} {
		if got := Slug(tc.in); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
