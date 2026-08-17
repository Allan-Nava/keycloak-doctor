package rules

import (
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

// reachableAuthenticators returns the authenticator ids reachable from the named
// top-level flow, and whether a flow by that name exists at all.
//
// The two sources shape flows differently: a realm export gives each flow its own
// execution list and nests subflows through flowAlias, while the Admin API
// executions endpoint returns an already-flat list. Following flowAlias when it is
// there and taking the list as-is when it is not handles both. DISABLED executions
// are ignored — a disabled OTP step is not a second factor.
func reachableAuthenticators(r *keycloak.Realm, alias string) (authenticators []string, found bool) {
	byAlias := make(map[string]*keycloak.AuthFlow, len(r.AuthenticationFlows))
	for i := range r.AuthenticationFlows {
		byAlias[r.AuthenticationFlows[i].Alias] = &r.AuthenticationFlows[i]
	}
	start, ok := byAlias[alias]
	if !ok {
		return nil, false
	}
	visited := map[string]bool{alias: true}
	queue := []*keycloak.AuthFlow{start}
	for len(queue) > 0 {
		flow := queue[0]
		queue = queue[1:]
		for _, e := range flow.Executions {
			if e.IsDisabled() {
				continue
			}
			if e.FlowAlias != "" && !visited[e.FlowAlias] {
				visited[e.FlowAlias] = true
				if sub, ok := byAlias[e.FlowAlias]; ok {
					queue = append(queue, sub)
				}
			}
			if p := e.Provider(); p != "" {
				authenticators = append(authenticators, p)
			}
		}
	}
	return authenticators, true
}

// secondFactorMarkers are the substrings that identify a Keycloak authenticator
// asking for something other than a password: the OTP forms (including the
// conditional subflow), WebAuthn, and recovery codes.
// Keycloak's recovery-code authenticator is auth-recovery-authn-code-form, so the
// marker is the singular stem that matches it and its config provider alike.
var secondFactorMarkers = []string{"otp", "webauthn", "recovery-authn-code"}

// isSecondFactor reports whether an authenticator id is a second factor.
func isSecondFactor(authenticator string) bool {
	a := strings.ToLower(authenticator)
	for _, m := range secondFactorMarkers {
		if strings.Contains(a, m) {
			return true
		}
	}
	return false
}
