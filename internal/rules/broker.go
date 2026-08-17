package rules

import (
	"fmt"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

func brokerRules() []Rule {
	return []Rule{
		{
			ID:        "idp/trust-email",
			Title:     "No identity provider is trusted to assert email addresses unchecked",
			Rationale: "Trust email skips Keycloak's own verification for accounts coming from that broker. When the realm also lets users log in by email, an IdP that does not verify its addresses (or lets a user set one) can be walked into an existing local account through first-broker-login account linking.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "idp/trust-email"
				return scanIDPs(id, r, "no identity provider is trusted to assert email addresses unchecked (%d checked)", func(idp *keycloak.IdentityProvider) []engine.Finding {
					if !idp.TrustEmail {
						return nil
					}
					if r.LoginWithEmailAllowed {
						return []engine.Finding{warn(id, r, idp.Alias,
							fmt.Sprintf("broker %q is trusted for email while the realm allows login by email: linking to an existing account needs no verification", idp.Alias),
							"turn Trust email off unless the provider verifies addresses and users cannot change them there")}
					}
					return []engine.Finding{warn(id, r, idp.Alias,
						fmt.Sprintf("broker %q is trusted for email: addresses from it are never verified by Keycloak", idp.Alias),
						"turn Trust email off unless the provider verifies addresses itself")}
				})
			},
		},
		{
			ID:        "idp/plaintext-endpoints",
			Title:     "Broker endpoints are HTTPS",
			Rationale: "The token and authorization endpoints of a broker carry codes, client secrets and identity assertions. Over plaintext HTTP, whoever sits on the path can read them and, worse, answer in the provider's place.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "idp/plaintext-endpoints"
				endpointKeys := []string{"authorizationUrl", "tokenUrl", "jwksUrl", "userInfoUrl", "logoutUrl", "singleSignOnServiceUrl"}
				return scanIDPs(id, r, "every broker endpoint is HTTPS (%d providers checked)", func(idp *keycloak.IdentityProvider) []engine.Finding {
					var out []engine.Finding
					for _, key := range endpointKeys {
						if v := idp.Config.Get(key); hasPlainHTTPEndpoint(v) {
							out = append(out, bad(id, r, idp.Alias,
								fmt.Sprintf("broker %q reaches %s over plaintext HTTP (%s)", idp.Alias, key, v),
								"point the endpoint at its HTTPS URL"))
						}
					}
					return out
				})
			},
		},
	}
}

// scanIDPs applies eval to every enabled identity provider, with the same
// aggregate-OK and unevaluated handling as scanClients.
func scanIDPs(rule string, r *keycloak.Realm, okFormat string, eval func(*keycloak.IdentityProvider) []engine.Finding) []engine.Finding {
	if reason := r.Unavailable("identityProviders"); reason != "" {
		return []engine.Finding{unevaluated(rule, r, "identityProviders")}
	}
	var out []engine.Finding
	checked := 0
	for i := range r.IdentityProviders {
		idp := &r.IdentityProviders[i]
		if !idp.IsEnabled() {
			continue
		}
		checked++
		out = append(out, eval(idp)...)
	}
	if len(out) == 0 {
		if checked == 0 {
			return []engine.Finding{pass(rule, r, "the realm has no enabled identity provider")}
		}
		return []engine.Finding{pass(rule, r, fmt.Sprintf(okFormat, checked))}
	}
	return out
}
