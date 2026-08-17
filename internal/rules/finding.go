package rules

import (
	"fmt"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

// at builds a finding about one object of the realm.
func at(rule string, r *keycloak.Realm, target string, st engine.Status, msg, remediation string) engine.Finding {
	return engine.Finding{
		Rule:        rule,
		Realm:       r.Name(),
		Target:      target,
		Status:      st,
		Message:     msg,
		Remediation: remediation,
	}
}

// bad, warn and okf are the shorthands the rules use. The realm-wide variants
// leave Target empty.
func bad(rule string, r *keycloak.Realm, target, msg, remediation string) engine.Finding {
	return at(rule, r, target, engine.BAD, msg, remediation)
}

func warn(rule string, r *keycloak.Realm, target, msg, remediation string) engine.Finding {
	return at(rule, r, target, engine.WARN, msg, remediation)
}

func pass(rule string, r *keycloak.Realm, msg string) engine.Finding {
	return at(rule, r, "", engine.OK, msg, "")
}

// unevaluated reports that a rule could not run because the source did not
// provide the data — a section the Admin API credentials may not read. It is an
// ERROR, not an OK: "I could not look" and "I looked and it is fine" must never
// render the same.
func unevaluated(rule string, r *keycloak.Realm, section string) engine.Finding {
	return at(rule, r, "", engine.ERROR,
		fmt.Sprintf("not evaluated: the source did not provide %s (%s)", section, r.Unavailable(section)),
		"grant the audit credentials the matching view- role, or audit a realm export instead")
}

// scanClients applies eval to every enabled client of the realm and returns the
// findings, or a single aggregate OK (formatted with the number of clients
// checked) when no client produced one. Disabled clients are skipped: they cannot
// be used to obtain a token, so reporting them is noise.
func scanClients(rule string, r *keycloak.Realm, okFormat string, eval func(*keycloak.Client) []engine.Finding) []engine.Finding {
	if reason := r.Unavailable("clients"); reason != "" {
		return []engine.Finding{unevaluated(rule, r, "clients")}
	}
	var out []engine.Finding
	checked := 0
	for i := range r.Clients {
		c := &r.Clients[i]
		if !c.IsEnabled() {
			continue
		}
		checked++
		out = append(out, eval(c)...)
	}
	if len(out) == 0 {
		return []engine.Finding{pass(rule, r, fmt.Sprintf(okFormat, checked))}
	}
	return out
}

// clientTarget names a client in a finding.
func clientTarget(c *keycloak.Client) string {
	if c.ClientID != "" {
		return c.ClientID
	}
	return c.Name
}
