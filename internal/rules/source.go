package rules

import (
	"fmt"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

func sourceRules() []Rule {
	return []Rule{
		{
			ID:        "source/secret-material",
			Title:     "The audited source is handled as credential material",
			Rationale: "A realm export is not a config file: it carries plaintext client secrets, LDAP bind credentials and broker client secrets. Exports end up in ticket attachments, shared drives and git history because they look like YAML-adjacent configuration. keycloak-doctor drops the values at load time — this rule tells you how much the file you just audited is worth to an attacker.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "source/secret-material"
				clients, brokers, comps, keys := countSecrets(r)
				total := clients + brokers + comps
				if total == 0 {
					return []engine.Finding{pass(id, r, "the source carried no credential values")}
				}
				parts := []string{}
				if clients > 0 {
					parts = append(parts, fmt.Sprintf("%d client secret(s)", clients))
				}
				if brokers > 0 {
					parts = append(parts, fmt.Sprintf("%d broker secret(s)", brokers))
				}
				if comps > 0 {
					parts = append(parts, fmt.Sprintf("%d component credential(s): %s", comps, strings.Join(keys, ", ")))
				}
				summary := strings.Join(parts, ", ")
				if strings.HasPrefix(r.Origin, "file:") {
					return []engine.Finding{warn(id, r, "",
						fmt.Sprintf("the export carries %s in plaintext", summary),
						"treat the file as a credential: keep it out of git and ticket attachments, and delete it after the audit")}
				}
				return []engine.Finding{pass(id, r,
					fmt.Sprintf("the audit credentials can read %s (values dropped at load time)", summary))}
			},
		},
	}
}

// countSecrets reports how much credential material the source carried, by kind,
// plus the component config keys involved.
func countSecrets(r *keycloak.Realm) (clients, brokers, components int, keys []string) {
	for i := range r.Clients {
		if r.Clients[i].SecretSet {
			clients++
		}
	}
	for i := range r.IdentityProviders {
		if r.IdentityProviders[i].SecretSet {
			brokers++
		}
	}
	seen := map[string]bool{}
	for _, c := range r.Components() {
		if len(c.SecretKeys) == 0 {
			continue
		}
		components++
		for _, k := range c.SecretKeys {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return clients, brokers, components, keys
}
