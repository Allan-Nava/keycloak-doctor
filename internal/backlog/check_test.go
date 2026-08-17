package backlog

import (
	"strings"
	"testing"
)

func problems(t *testing.T, src string) []Problem {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc.Check()
}

func requireProblem(t *testing.T, ps []Problem, level, substring string) {
	t.Helper()
	for _, p := range ps {
		if p.Level == level && strings.Contains(p.Message, substring) {
			return
		}
	}
	t.Errorf("no %s containing %q; got %v", level, substring, ps)
}

func TestCheckCleanBacklog(t *testing.T) {
	if ps := problems(t, sample); len(ps) != 0 {
		t.Errorf("clean backlog reported problems: %v", ps)
	}
}

func TestCheckDuplicateID(t *testing.T) {
	ps := problems(t, `## Open

### Rules

- **KD-1** — first.
- **KD-1** — second, same id.
`)
	requireProblem(t, ps, "error", "KD-1 is listed twice")
	if got := Errors(ps); got != 1 {
		t.Errorf("errors = %d, want 1 (%v)", got, ps)
	}
}

func TestCheckItemBothOpenAndDone(t *testing.T) {
	ps := problems(t, `## Open

### Rules

- **KD-1** — still open.

## Done

- **KD-1** — also claimed as done.
`)
	requireProblem(t, ps, "error", "both as open and as done")
}

func TestCheckMilestoneProblems(t *testing.T) {
	ps := problems(t, `## Milestones

- **v0.2.0** — first scope. Items: KD-1, KD-99, KD-0.
- **v0.3.0** — second scope. Items: KD-1.
- **v0.4.0** — a scope with no ids.

## Open

### Rules

- **KD-1** — an open item.

## Done

- **KD-0** — shipped already.
`)
	requireProblem(t, ps, "error", "claims KD-99, which is not an item")
	requireProblem(t, ps, "error", "KD-1 is claimed by both v0.2.0 and v0.3.0")
	requireProblem(t, ps, "error", "milestone v0.4.0 claims no item")
	requireProblem(t, ps, "warn", "claims KD-0, which is already done")

	// Errors sort before warnings so the first line of a failed gate is the worst.
	if ps[0].Level != "error" || ps[len(ps)-1].Level != "warn" {
		t.Errorf("problems are not sorted worst-first: %v", ps)
	}
}

func TestCheckDuplicateMilestone(t *testing.T) {
	ps := problems(t, `## Milestones

- **v0.2.0** — one. Items: KD-1.
- **v0.2.0** — again. Items: KD-1.

## Open

### Rules

- **KD-1** — an open item.
`)
	requireProblem(t, ps, "error", "milestone v0.2.0 is declared twice")
}

func TestCheckItemWithoutGroupWarns(t *testing.T) {
	ps := problems(t, `## Open

- **KD-1** — an item with no group heading above it.
`)
	requireProblem(t, ps, "warn", "no \"###\" group heading")
	if Errors(ps) != 0 {
		t.Errorf("a missing group must not fail the gate: %v", ps)
	}
}

func TestCheckEmptyBacklog(t *testing.T) {
	ps := problems(t, "# Backlog\n\nNothing here yet.\n")
	requireProblem(t, ps, "error", "no items found")
}
