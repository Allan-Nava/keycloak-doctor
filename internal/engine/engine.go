// Package engine defines the audit contract: the finding statuses, the finding
// itself and the aggregation helpers every renderer and command shares.
//
// Unlike a prober, an audit has no IO of its own: the realm model is loaded once
// (internal/keycloak) and every rule (internal/rules) is a pure function over
// it. That is why this package imports nothing but the standard library and has
// no runner with timeouts — the network lives in the loader, not here.
package engine

import (
	"sort"
	"time"
)

// Status of a single finding. Severity order: OK < WARN < BAD < ERROR.
type Status string

const (
	OK    Status = "OK"
	WARN  Status = "WARN"
	BAD   Status = "BAD"
	ERROR Status = "ERROR" // the rule could not be evaluated (data missing from the source)
)

var severity = map[Status]int{OK: 0, WARN: 1, BAD: 2, ERROR: 3}

// AtLeast reports whether s is at or above threshold in the severity order
// OK < WARN < BAD < ERROR. An empty threshold is satisfied by anything, since
// severity[""] is the zero value — callers that mean "no threshold at all" must
// test for "" themselves rather than relying on this.
func AtLeast(s, threshold Status) bool {
	return severity[s] >= severity[threshold]
}

// Known reports whether s is one of the four statuses.
func Known(s Status) bool {
	_, ok := severity[s]
	return ok && s != ""
}

// Finding is one observation of one rule about one object of one realm.
//
// Target names the object the finding is about — a client id, an identity
// provider alias, a component name — and is empty for realm-wide findings.
// Remediation is the operator-facing "what to do"; it is written by the rule and
// never contains values read from the realm that could be credential material.
type Finding struct {
	Rule        string `json:"rule"`
	Realm       string `json:"realm"`
	Target      string `json:"target,omitempty"`
	Status      Status `json:"status"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// Result aggregates one audit run.
type Result struct {
	Findings []Finding     `json:"findings"`
	Realms   []string      `json:"realms"`
	Rules    int           `json:"rules"`
	Source   string        `json:"source"`
	Started  time.Time     `json:"started"`
	Duration time.Duration `json:"duration_ns"`
}

// SortFindings orders findings worst-first, then by rule, realm and target, with
// a stable sort so equal keys keep their input order.
//
// This ordering is a de-facto API: anything parsing the text output relies on the
// first line being the thing to look at.
func SortFindings(findings []Finding) []Finding {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if severity[a.Status] != severity[b.Status] {
			return severity[a.Status] > severity[b.Status]
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Realm != b.Realm {
			return a.Realm < b.Realm
		}
		return a.Target < b.Target
	})
	return findings
}

// Dedup removes exact-duplicate findings, keeping the first occurrence and
// preserving order. Duplicates arise when the same realm is passed twice (a
// directory export listing a realm in two files, say).
func Dedup(findings []Finding) []Finding {
	seen := make(map[Finding]bool, len(findings))
	out := findings[:0:0]
	for _, f := range findings {
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// MinSeverity keeps only the findings at or above threshold. An empty threshold
// keeps everything.
func MinSeverity(findings []Finding, threshold Status) []Finding {
	if threshold == "" {
		return findings
	}
	out := findings[:0:0]
	for _, f := range findings {
		if AtLeast(f.Status, threshold) {
			out = append(out, f)
		}
	}
	return out
}

// Summarize counts findings per status.
func Summarize(findings []Finding) map[Status]int {
	m := map[Status]int{}
	for _, f := range findings {
		m[f.Status]++
	}
	return m
}

// Worst returns the most severe status present (OK for an empty list).
func Worst(findings []Finding) Status {
	worst := OK
	for _, f := range findings {
		if severity[f.Status] > severity[worst] {
			worst = f.Status
		}
	}
	return worst
}
