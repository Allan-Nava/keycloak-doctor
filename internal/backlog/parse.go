// Package backlog parses BACKLOG.md — the single source of truth for the work on
// keycloak-doctor — and mirrors it into GitHub milestones and issues.
//
// The parser is deliberately strict about the shape of a bullet. The file is read
// by a human first, so a bullet the automation does not understand has to fail
// the run loudly: silently dropping an item would leave the tracker looking
// complete while an item is missing from it, which is the one failure mode a
// mirror must not have.
package backlog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Item is one backlog entry: a stable id, the prose that describes it, and where
// it sits (open under a group heading, or done).
type Item struct {
	ID        string `json:"id"`
	Num       int    `json:"num"`
	Group     string `json:"group,omitempty"`
	Done      bool   `json:"done"`
	Text      string `json:"text"`
	Milestone string `json:"milestone,omitempty"`
	Line      int    `json:"line"`
}

// Milestone is one release scope: a version, an optional due date, the prose that
// says what the release is about, and the ids it claims.
type Milestone struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Due         string   `json:"due,omitempty"` // YYYY-MM-DD, empty when open-ended
	Items       []string `json:"items"`
	Line        int      `json:"line"`
}

// Doc is a parsed BACKLOG.md.
type Doc struct {
	Milestones []Milestone `json:"milestones"`
	Items      []Item      `json:"items"`
}

var (
	itemRe      = regexp.MustCompile(`^-\s+\*\*(KD-(\d+))\*\*\s*[—–-]\s*(.+)$`)
	milestoneRe = regexp.MustCompile(`^-\s+\*\*(v[0-9]+\.[0-9]+\.[0-9]+)\*\*(?:\s+\(due\s+([0-9]{4}-[0-9]{2}-[0-9]{2})\))?\s*[—–-]\s*(.+)$`)
	itemsRe     = regexp.MustCompile(`(?i)\bitems:\s*(.*)$`)
	idRe        = regexp.MustCompile(`KD-[0-9]+`)
)

// Parse reads BACKLOG.md. It returns an error for a bullet that does not match
// the documented shape; everything that is well-formed but inconsistent (an
// unknown id in a milestone, a duplicate) is reported by [Doc.Check] instead, so
// one malformed line does not hide the rest of the report.
func Parse(src []byte) (Doc, error) {
	var d Doc
	var malformed []string

	sc := bufio.NewScanner(bytes.NewReader(src))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	section, group := "", ""
	item, milestone := -1, -1 // indexes of the entry a continuation line belongs to
	line := 0

	for sc.Scan() {
		line++
		text := strings.TrimRight(sc.Text(), " \t")

		switch {
		case strings.HasPrefix(text, "## "):
			section, group, item, milestone = strings.ToLower(strings.TrimSpace(text[3:])), "", -1, -1
			continue
		case strings.HasPrefix(text, "### "):
			if section == "open" || section == "done" {
				group = strings.TrimSpace(text[4:])
			}
			item, milestone = -1, -1
			continue
		case strings.HasPrefix(text, "# "):
			section, group, item, milestone = "", "", -1, -1
			continue
		}

		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			item, milestone = -1, -1
			continue
		}

		switch section {
		case "open", "done":
			if m := itemRe.FindStringSubmatch(trimmed); m != nil {
				num, _ := strconv.Atoi(m[2]) // the regexp already proved it is digits
				d.Items = append(d.Items, Item{
					ID:    m[1],
					Num:   num,
					Group: group,
					Done:  section == "done",
					Text:  strings.TrimSpace(m[3]),
					Line:  line,
				})
				item, milestone = len(d.Items)-1, -1
				continue
			}
			if strings.HasPrefix(trimmed, "- ") {
				malformed = append(malformed, fmt.Sprintf("line %d: bullet under %q is not an item, want %q", line, "## "+section, "- **KD-n** — description"))
				item = -1
				continue
			}
			if item >= 0 {
				d.Items[item].Text += " " + trimmed
			}
		case "milestones":
			if m := milestoneRe.FindStringSubmatch(trimmed); m != nil {
				d.Milestones = append(d.Milestones, Milestone{
					Title:       m[1],
					Due:         m[2],
					Description: strings.TrimSpace(m[3]),
					Line:        line,
				})
				milestone, item = len(d.Milestones)-1, -1
				continue
			}
			if strings.HasPrefix(trimmed, "- ") {
				malformed = append(malformed, fmt.Sprintf("line %d: bullet under %q is not a milestone, want %q", line, "## Milestones", "- **vX.Y.Z** [(due YYYY-MM-DD)] — description. Items: KD-a, KD-b."))
				milestone = -1
				continue
			}
			if milestone >= 0 {
				d.Milestones[milestone].Description += " " + trimmed
			}
		}
	}
	if err := sc.Err(); err != nil {
		return d, fmt.Errorf("reading BACKLOG.md: %w", err)
	}

	d.splitMilestoneItems()
	d.assignMilestones()
	sort.SliceStable(d.Items, func(i, j int) bool { return d.Items[i].Num < d.Items[j].Num })

	if len(malformed) > 0 {
		return d, errors.New("BACKLOG.md is malformed:\n  " + strings.Join(malformed, "\n  "))
	}
	return d, nil
}

// splitMilestoneItems peels the trailing "Items: KD-a, KD-b." off each milestone
// description: the ids are the machine-readable half of a line whose first half
// is prose meant for a reader of the file.
func (d *Doc) splitMilestoneItems() {
	for i := range d.Milestones {
		m := &d.Milestones[i]
		loc := itemsRe.FindStringSubmatchIndex(m.Description)
		if loc == nil {
			continue
		}
		m.Items = idRe.FindAllString(m.Description[loc[2]:loc[3]], -1)
		m.Description = strings.TrimRight(strings.TrimSpace(m.Description[:loc[0]]), " .,;:")
	}
}

// assignMilestones records, on each item, the first milestone that claims it. A
// second claim is left for [Doc.Check] to report.
func (d *Doc) assignMilestones() {
	claimed := map[string]string{}
	for _, m := range d.Milestones {
		for _, id := range m.Items {
			if _, dup := claimed[id]; !dup {
				claimed[id] = m.Title
			}
		}
	}
	for i := range d.Items {
		d.Items[i].Milestone = claimed[d.Items[i].ID]
	}
}

// Open returns the items that are not done yet, in id order.
func (d Doc) Open() []Item {
	var out []Item
	for _, it := range d.Items {
		if !it.Done {
			out = append(out, it)
		}
	}
	return out
}

const titleMax = 72

// Title renders the issue title of an item: the id, then the first sentence of
// its description, clipped. The id prefix is how a sync run finds the issue that
// already tracks an item, so it is part of the contract, not decoration.
func (it Item) Title() string {
	t := strings.TrimSpace(strings.ReplaceAll(firstSentence(it.Text), "`", ""))
	if len(t) > titleMax {
		cut := t[:titleMax]
		if i := strings.LastIndex(cut, " "); i > titleMax/2 {
			cut = cut[:i]
		}
		t = strings.TrimRight(cut, " ,;:") + "…"
	}
	return it.ID + ": " + t
}

// Body renders the issue body: the description verbatim, plus a pointer back to
// the file that owns it, so nobody edits the copy instead of the original.
func (it Item) Body(repo string) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "%s\n\n", it.Text)
	if it.Group != "" {
		fmt.Fprintf(b, "Area: **%s**\n\n", it.Group)
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(b, "Tracked as **%s** in [BACKLOG.md](https://github.com/%s/blob/main/BACKLOG.md), the single source of truth for the work on this project. "+
		"This issue is generated by `go run ./cmd/backlog sync`: edit `BACKLOG.md` and the next sync updates the issue — an edit made here is overwritten.\n", it.ID, repo)
	return b.String()
}

// Labels returns the labels a synced issue carries: one to mark it as generated,
// one for the area it sits under.
func (it Item) Labels() []string {
	labels := []string{"backlog"}
	if s := slug(it.Group); s != "" {
		labels = append(labels, "area/"+s)
	}
	return labels
}

// firstSentence cuts at the first ". " so a title stays a title. An abbreviation
// mid-sentence would cut early, which costs a shorter title and nothing else.
func firstSentence(s string) string {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '.' && s[i+1] == ' ' {
			return s[:i]
		}
	}
	return strings.TrimSuffix(s, ".")
}

func slug(s string) string {
	b := &strings.Builder{}
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
