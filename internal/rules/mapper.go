package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

// sensitiveAttribute matches user attribute names that hold credential material.
// A protocol mapper reading one of these copies it into every token the client
// gets — and tokens are logged, cached and forwarded far more freely than the
// user store is.
var sensitiveAttribute = regexp.MustCompile(`(?i)(pass(word|wd)|secret|token|api[-_.]?key|private[-_.]?key|credential|otp[-_.]?seed)`)

func mapperRules() []Rule {
	return []Rule{
		{
			ID:        "mapper/sensitive-attribute",
			Title:     "No protocol mapper copies credential material into a token",
			Rationale: "Mappers are the quiet way secrets leave Keycloak: a user attribute called password_hash or api_key mapped into a claim ends up in every access token for that client, and from there in proxy logs, browser storage and any downstream service that decodes the token.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "mapper/sensitive-attribute"
				return scanClients(id, r, "no protocol mapper copies a credential-looking attribute into a token (%d clients checked)", func(c *keycloak.Client) []engine.Finding {
					var out []engine.Finding
					for _, m := range c.ProtocolMappers {
						attr := firstNonEmpty(m.Config.Get("user.attribute"), m.Config.Get("user.session.note"))
						if attr == "" || !sensitiveAttribute.MatchString(attr) {
							continue
						}
						claim := firstNonEmpty(m.Config.Get("claim.name"), m.Config.Get("attribute.name"), m.Name)
						out = append(out, bad(id, r, clientTarget(c)+" · "+m.Name,
							fmt.Sprintf("mapper %q copies the %q attribute into the %q claim", m.Name, attr, claim),
							"remove the mapper; nothing that authenticates a user belongs in a token claim"))
					}
					return out
				})
			},
		},
		{
			ID:        "mapper/hardcoded-audience",
			Title:     "No mapper hands a client an audience it does not own",
			Rationale: "An audience mapper is how a token minted for one client becomes acceptable to another. Pointed at a privileged API, it lets a low-trust frontend obtain tokens that API will honour — an authorization bypass that reads as ordinary configuration.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "mapper/hardcoded-audience"
				return scanClients(id, r, "no client adds a foreign audience through a mapper (%d checked)", func(c *keycloak.Client) []engine.Finding {
					var out []engine.Finding
					for _, m := range c.ProtocolMappers {
						if !strings.Contains(strings.ToLower(m.ProtocolMapper), "audience") {
							continue
						}
						aud := firstNonEmpty(m.Config.Get("included.client.audience"), m.Config.Get("included.custom.audience"))
						if aud == "" || aud == c.ClientID {
							continue
						}
						out = append(out, warn(id, r, clientTarget(c)+" · "+m.Name,
							fmt.Sprintf("mapper %q adds the audience %q to this client's tokens", m.Name, aud),
							"confirm that the audience is meant to accept tokens obtained through this client"))
					}
					return out
				})
			},
		},
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
