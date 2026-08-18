// Package suppress applies a suppression file to an audit: the findings an
// operator has decided to accept, each with a date the decision expires on and a
// reason for it.
//
// Two properties are what make this safe to have in a pipeline, and they are the
// reason this is not just a list of rule ids:
//
//   - a suppression is never silent — the run reports how many findings it
//     removed, so a suppression file cannot be mistaken for a clean realm;
//   - a suppression is never permanent — past its date it stops suppressing and
//     the run says so. A suppression without an expiry is how a finding
//     disappears for three years.
//
// An entry that matches nothing is reported too: either the finding was fixed and
// the entry is dead, or the rule id is a typo and the operator believes they have
// suppressed something they have not.
package suppress

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
)

// The rule ids the suppression layer itself reports under. They are not realm
// rules and not part of the compiled catalogue: they are about the suppression
// file, so --only and --skip do not select them.
const (
	RuleExpired   = "suppression/expired"
	RuleUnmatched = "suppression/unmatched"
)

// dateLayout is the only accepted form of an expiry: a day, not a timestamp. A
// suppression is a decision somebody made, and decisions expire on dates.
const dateLayout = "2006-01-02"

// Entry is one accepted finding. Rule is required; Realm and Target narrow it,
// and an empty one (or "*") matches anything.
type Entry struct {
	Rule   string `json:"rule"`
	Realm  string `json:"realm,omitempty"`
	Target string `json:"target,omitempty"`
	Until  string `json:"until"`
	Reason string `json:"reason"`
}

// Set is a parsed suppression file.
type Set struct {
	Entries []Entry `json:"suppressions"`
}

// Load parses and validates a suppression file. Unknown fields are an error: a
// misspelled key would otherwise leave an entry that silently suppresses nothing,
// or — worse — suppresses more than intended.
func Load(r io.Reader) (*Set, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var set Set
	if err := dec.Decode(&set); err != nil {
		return nil, fmt.Errorf("reading the suppression file: %w", err)
	}
	if err := set.validate(); err != nil {
		return nil, err
	}
	return &set, nil
}

func (s *Set) validate() error {
	var problems []string
	for i, e := range s.Entries {
		where := fmt.Sprintf("suppression %d", i+1)
		if strings.TrimSpace(e.Rule) == "" {
			problems = append(problems, where+`: "rule" is required (the id the finding reports under)`)
		}
		if strings.TrimSpace(e.Until) == "" {
			problems = append(problems, where+`: "until" is required (YYYY-MM-DD): a suppression with no expiry never gets revisited`)
		} else if _, err := time.Parse(dateLayout, e.Until); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %q is not a date in the form YYYY-MM-DD", where, e.Until))
		}
		if strings.TrimSpace(e.Reason) == "" {
			problems = append(problems, where+`: "reason" is required: an accepted finding with no reason is one nobody can review`)
		}
	}
	if len(problems) > 0 {
		return errors.New("invalid suppression file:\n  " + strings.Join(problems, "\n  "))
	}
	return nil
}

// Outcome is what applying a set did to a run.
type Outcome struct {
	// Findings are the ones that survived, plus the findings the suppression layer
	// reports about the file itself.
	Findings []engine.Finding
	// Suppressed counts the removed findings.
	Suppressed int
}

// Apply removes the findings a live suppression covers and reports on the file:
// one finding per expired entry and one per entry that matched nothing. now is
// passed in rather than read from the clock so a test can sit on either side of
// an expiry.
func (s *Set) Apply(findings []engine.Finding, now time.Time) Outcome {
	today := now.UTC().Truncate(24 * time.Hour)
	matched := make([]int, len(s.Entries))

	kept := findings[:0:0]
	suppressed := 0
	for _, f := range findings {
		hit := -1
		for i, e := range s.Entries {
			if !e.matches(f) {
				continue
			}
			matched[i]++
			// An expired entry still counts as matched — that is what makes the
			// "expired" report point at an entry someone actually relies on — but it
			// no longer hides the finding.
			if hit < 0 && !e.expired(today) {
				hit = i
			}
		}
		if hit >= 0 {
			suppressed++
			continue
		}
		kept = append(kept, f)
	}

	out := Outcome{Findings: kept, Suppressed: suppressed}
	for i, e := range s.Entries {
		switch {
		case e.expired(today):
			out.Findings = append(out.Findings, engine.Finding{
				Rule:        RuleExpired,
				Realm:       e.scopeRealm(),
				Target:      e.scopeTarget(),
				Status:      engine.WARN,
				Message:     fmt.Sprintf("the suppression of %s expired on %s and no longer applies (reason given: %s)", e.Rule, e.Until, e.Reason),
				Remediation: "fix the finding, or renew the suppression with a new date and a reason that still holds",
			})
		case matched[i] == 0:
			out.Findings = append(out.Findings, engine.Finding{
				Rule:        RuleUnmatched,
				Realm:       e.scopeRealm(),
				Target:      e.scopeTarget(),
				Status:      engine.WARN,
				Message:     fmt.Sprintf("the suppression of %s matched no finding in this run", e.Rule),
				Remediation: "remove it if the finding is fixed; check the rule id, realm and target if you expected it to match",
			})
		}
	}
	return out
}

// Rules describes the findings the suppression layer emits, so they can be
// documented like any other id an operator may see in a report.
func Rules() []Rule {
	return []Rule{
		{
			ID:    RuleExpired,
			Title: "No suppression is past its expiry date",
			Rationale: "A suppression is a decision to accept a finding for a while. Past its date the decision has " +
				"not been reviewed, so the finding comes back and this reports which entry lapsed — the alternative is " +
				"a suppression file that quietly becomes permanent.",
		},
		{
			ID:    RuleUnmatched,
			Title: "Every suppression matches a finding",
			Rationale: "An entry that matches nothing is either dead (the finding was fixed and the entry should go) or " +
				"wrong (a typo in the rule id, realm or target), and a wrong entry means somebody believes a finding is " +
				"accepted when it is still reported.",
		},
	}
}

// Rule is the documentation of one suppression finding.
type Rule struct {
	ID        string
	Title     string
	Rationale string
}

// IDs lists the rule ids the suppression layer reports under, ordered.
func IDs() []string {
	ids := make([]string, 0, len(Rules()))
	for _, r := range Rules() {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	return ids
}

func (e Entry) matches(f engine.Finding) bool {
	return e.Rule == f.Rule && fieldMatches(e.Realm, f.Realm) && fieldMatches(e.Target, f.Target)
}

// fieldMatches treats an empty pattern and "*" as "any": a suppression that does
// not name a realm is about the rule everywhere.
func fieldMatches(pattern, value string) bool {
	p := strings.TrimSpace(pattern)
	return p == "" || p == "*" || p == value
}

func (e Entry) expired(today time.Time) bool {
	until, err := time.Parse(dateLayout, e.Until)
	if err != nil {
		return true // validated on load; an unparseable date must not suppress
	}
	return until.UTC().Before(today)
}

// scopeRealm and scopeTarget put the entry's own scope in the report: the realm it
// names (or "*" for every realm) and the rule it covers, so the finding reads as
// "this entry, about that rule" rather than as a finding about a realm object.
func (e Entry) scopeRealm() string {
	if r := strings.TrimSpace(e.Realm); r != "" {
		return r
	}
	return "*"
}

func (e Entry) scopeTarget() string {
	if t := strings.TrimSpace(e.Target); t != "" && t != "*" {
		return e.Rule + " · " + t
	}
	return e.Rule
}
