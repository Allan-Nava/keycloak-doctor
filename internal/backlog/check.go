package backlog

import (
	"fmt"
	"sort"
)

// Problem is one inconsistency found in BACKLOG.md. Level is "error" for
// something that makes the mirror ambiguous (a duplicate id, a milestone
// pointing at an id that does not exist) and "warn" for something a reader
// should fix but that still syncs deterministically.
type Problem struct {
	Level   string `json:"level"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Check reports what is inconsistent in a parsed backlog, worst first. It never
// looks at GitHub: a broken file has to be caught on a pull request, before any
// issue is touched.
func (d Doc) Check() []Problem {
	var ps []Problem
	errorf := func(line int, format string, a ...any) {
		ps = append(ps, Problem{Level: "error", Line: line, Message: fmt.Sprintf(format, a...)})
	}
	warnf := func(line int, format string, a ...any) {
		ps = append(ps, Problem{Level: "warn", Line: line, Message: fmt.Sprintf(format, a...)})
	}

	if len(d.Items) == 0 {
		errorf(0, "no items found: an item is a bullet %q under %q or %q", "- **KD-n** — description", "## Open", "## Done")
	}

	seen := map[string]Item{}
	for _, it := range d.Items {
		if prev, dup := seen[it.ID]; dup {
			switch {
			case prev.Done != it.Done:
				errorf(it.Line, "%s is listed both as open and as done (also on line %d): an item has one state", it.ID, prev.Line)
			default:
				errorf(it.Line, "%s is listed twice (also on line %d): ids are the identity of an item", it.ID, prev.Line)
			}
			continue
		}
		seen[it.ID] = it
		if !it.Done && it.Group == "" {
			warnf(it.Line, "%s has no %q group heading above it, so its issue gets no area label", it.ID, "###")
		}
	}

	titles := map[string]int{}
	claimed := map[string]string{}
	for _, m := range d.Milestones {
		if prev, dup := titles[m.Title]; dup {
			errorf(m.Line, "milestone %s is declared twice (also on line %d)", m.Title, prev)
			continue
		}
		titles[m.Title] = m.Line
		if len(m.Items) == 0 {
			errorf(m.Line, "milestone %s claims no item: end the line with %q", m.Title, "Items: KD-a, KD-b.")
		}
		if m.Description == "" {
			warnf(m.Line, "milestone %s has no description: it becomes the milestone description on GitHub", m.Title)
		}
		for _, id := range m.Items {
			it, ok := seen[id]
			switch {
			case !ok:
				errorf(m.Line, "milestone %s claims %s, which is not an item in this file", m.Title, id)
			case it.Done:
				warnf(m.Line, "milestone %s claims %s, which is already done: drop it from the milestone", m.Title, id)
			}
			if other, dup := claimed[id]; dup {
				errorf(m.Line, "%s is claimed by both %s and %s: an item belongs to one milestone", id, other, m.Title)
				continue
			}
			claimed[id] = m.Title
		}
	}

	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Level != ps[j].Level {
			return ps[i].Level == "error"
		}
		return ps[i].Line < ps[j].Line
	})
	return ps
}

// Errors counts the problems that must fail a run.
func Errors(ps []Problem) int {
	n := 0
	for _, p := range ps {
		if p.Level == "error" {
			n++
		}
	}
	return n
}
