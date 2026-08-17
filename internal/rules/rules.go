// Package rules holds the audit rules: what "insecure Keycloak realm" means,
// spelled out one rule at a time.
//
// Every rule is a pure function over a loaded realm (internal/keycloak), so a
// rule is testable against a struct literal and the whole audit needs no network.
// A rule reports one finding per offending object; when a rule finds nothing to
// report it emits a single aggregate OK for the realm rather than one OK per
// object, which is what keeps the output readable on a realm with 200 clients.
//
// Rule ids are stable and namespaced by category ("client/redirect-wildcard").
// They are the selector for --only/--skip and the key anyone suppressing a
// finding in CI will pin, so they must not be renamed lightly.
package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

// Rule is one audit rule.
type Rule struct {
	// ID is the stable "category/name" identifier.
	ID string
	// Title is a one-line statement of the property the rule wants to hold.
	Title string
	// Rationale explains why it matters — the domain knowledge that makes this
	// tool worth running, shown by `keycloak-doctor rules`.
	Rationale string
	// Eval returns the findings for one realm.
	Eval func(*keycloak.Realm) []engine.Finding
}

// Category is the part of the id before the slash.
func (r Rule) Category() string {
	if i := strings.Index(r.ID, "/"); i > 0 {
		return r.ID[:i]
	}
	return r.ID
}

// All returns every rule, ordered by id.
func All() []Rule {
	var all []Rule
	all = append(all, realmRules()...)
	all = append(all, clientRules()...)
	all = append(all, mapperRules()...)
	all = append(all, brokerRules()...)
	all = append(all, keyRules()...)
	all = append(all, federationRules()...)
	all = append(all, sourceRules()...)
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all
}

// Categories lists the rule categories, ordered.
func Categories() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range All() {
		if c := r.Category(); !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// Select filters the full rule set by the --only and --skip selectors. A selector
// is a rule id or a whole category; an unknown one is an error, because silently
// auditing a smaller set than the operator asked for is the failure mode that
// makes a security tool lie.
func Select(only, skip []string) ([]Rule, error) {
	all := All()
	valid := map[string]bool{}
	for _, r := range all {
		valid[r.ID] = true
		valid[r.Category()] = true
	}
	for _, s := range append(append([]string{}, only...), skip...) {
		if !valid[s] {
			return nil, fmt.Errorf("unknown rule or category %q (try `keycloak-doctor rules`)", s)
		}
	}
	matches := func(r Rule, sels []string) bool {
		for _, s := range sels {
			if s == r.ID || s == r.Category() {
				return true
			}
		}
		return false
	}
	out := make([]Rule, 0, len(all))
	for _, r := range all {
		if len(only) > 0 && !matches(r, only) {
			continue
		}
		if matches(r, skip) {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the selection leaves no rule to run")
	}
	return out, nil
}

// Audit evaluates every rule against every realm, worst-first.
func Audit(realms []*keycloak.Realm, rs []Rule) []engine.Finding {
	var findings []engine.Finding
	for _, realm := range realms {
		for _, rule := range rs {
			findings = append(findings, rule.Eval(realm)...)
		}
	}
	return engine.SortFindings(engine.Dedup(findings))
}
