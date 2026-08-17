package rules

import (
	"fmt"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

// userStorageType is the component provider type of user federation providers.
const userStorageType = "org.keycloak.storage.UserStorageProvider"

func federationRules() []Rule {
	return []Rule{
		{
			ID:        "federation/ldap-tls",
			Title:     "LDAP federation talks over TLS",
			Rationale: "Keycloak authenticates users against LDAP by binding with their password. Over ldap:// without StartTLS, every one of those passwords — plus the service account's bind credential — crosses the network in the clear.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "federation/ldap-tls"
				return scanFederation(id, r, "every LDAP federation provider uses TLS (%d checked)", func(c *keycloak.Component) []engine.Finding {
					url := c.Cfg("connectionUrl")
					if url == "" {
						return nil
					}
					lower := strings.ToLower(url)
					if strings.HasPrefix(lower, "ldaps://") {
						return nil
					}
					if c.CfgBool("startTls") {
						return nil
					}
					return []engine.Finding{bad(id, r, c.Label(),
						fmt.Sprintf("LDAP provider %q connects to %s without TLS: bind passwords cross the network in the clear", c.Label(), url),
						"switch the connection URL to ldaps://, or enable StartTLS on the provider")}
				})
			},
		},
		{
			ID:        "federation/ldap-truststore",
			Title:     "LDAP TLS certificates are verified",
			Rationale: "Use Truststore SPI set to Never means Keycloak accepts any certificate on the LDAP connection, which turns TLS into encoding: anyone who can answer at that address collects the bind passwords.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "federation/ldap-truststore"
				return scanFederation(id, r, "every LDAP provider verifies its server certificate (%d checked)", func(c *keycloak.Component) []engine.Finding {
					if !strings.EqualFold(c.Cfg("useTruststoreSpi"), "never") {
						return nil
					}
					return []engine.Finding{bad(id, r, c.Label(),
						fmt.Sprintf("LDAP provider %q does not verify the server certificate (useTruststoreSpi=never)", c.Label()),
						"set Use Truststore SPI to 'Always' and add the LDAP CA to the Keycloak truststore")}
				})
			},
		},
	}
}

// scanFederation applies eval to every user federation component.
func scanFederation(rule string, r *keycloak.Realm, okFormat string, eval func(*keycloak.Component) []engine.Finding) []engine.Finding {
	if reason := r.Unavailable("components"); reason != "" {
		return []engine.Finding{unevaluated(rule, r, "components")}
	}
	var out []engine.Finding
	checked := 0
	for _, c := range r.Components() {
		if c.ProviderType != userStorageType || !strings.Contains(strings.ToLower(c.ProviderID), "ldap") {
			continue
		}
		checked++
		comp := c
		out = append(out, eval(&comp)...)
	}
	if len(out) == 0 {
		if checked == 0 {
			return nil // no LDAP federation in this realm: nothing to claim
		}
		return []engine.Finding{pass(rule, r, fmt.Sprintf(okFormat, checked))}
	}
	return out
}
