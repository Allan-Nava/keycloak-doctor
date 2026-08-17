package rules

import (
	"fmt"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

func clientRules() []Rule {
	return []Rule{
		{
			ID:        "client/redirect-wildcard",
			Title:     "Redirect URIs do not widen past a host you control",
			Rationale: "The redirect URI is the only thing that keeps an authorization code going to the right place. A wildcard in the authority — \"*\", \"https://*.example.com\", \"http://*\" — lets an attacker who can answer at a matching host complete the flow with the victim's code. A wildcard in the path is milder but still turns any open redirect on that host into a token leak.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "client/redirect-wildcard"
				return scanClients(id, r, "no enabled client widens its redirect URIs with a host wildcard (%d checked)", func(c *keycloak.Client) []engine.Finding {
					if !usesRedirects(c) {
						return nil
					}
					var out []engine.Finding
					for _, uri := range c.RedirectURIs {
						switch classifyRedirect(uri) {
						case hostWildcard:
							out = append(out, bad(id, r, clientTarget(c),
								fmt.Sprintf("redirect URI %q matches hosts the client does not control", uri),
								"list the exact callback URLs instead; Keycloak matches them literally"))
						case pathWildcard:
							out = append(out, warn(id, r, clientTarget(c),
								fmt.Sprintf("redirect URI %q wildcards the path", uri),
								"pin the callback path; any open redirect under it becomes a code leak"))
						case noWildcard:
						}
					}
					return out
				})
			},
		},
		{
			ID:        "client/redirect-plain-http",
			Title:     "Redirect URIs are HTTPS outside the loopback interface",
			Rationale: "An authorization code delivered over plaintext HTTP is readable by every hop in between. Loopback URLs are the specified exception for native and development clients; a real hostname over http:// is not.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "client/redirect-plain-http"
				return scanClients(id, r, "every enabled client redirects over HTTPS or to the loopback interface (%d checked)", func(c *keycloak.Client) []engine.Finding {
					if !usesRedirects(c) {
						return nil
					}
					var out []engine.Finding
					for _, uri := range c.RedirectURIs {
						if host := plainHTTPHost(uri); host != "" {
							out = append(out, bad(id, r, clientTarget(c),
								fmt.Sprintf("redirect URI %q delivers the authorization code over plaintext HTTP to %s", uri, host),
								"serve the callback over HTTPS and update the client"))
						}
					}
					return out
				})
			},
		},
		{
			ID:        "client/web-origins-wildcard",
			Title:     "CORS web origins are listed, not wildcarded",
			Rationale: "\"*\" in Web origins makes Keycloak answer token and userinfo requests for any page on the internet. Combined with a session cookie in the browser, that is what turns a stray XSS anywhere into a token for this client.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "client/web-origins-wildcard"
				return scanClients(id, r, "no enabled client allows every CORS origin (%d checked)", func(c *keycloak.Client) []engine.Finding {
					for _, o := range c.WebOrigins {
						if strings.TrimSpace(o) == "*" {
							return []engine.Finding{bad(id, r, clientTarget(c),
								"web origins contain \"*\": any site may call the token endpoint for this client",
								"list the exact origins, or use \"+\" to reuse the redirect URIs")}
						}
					}
					return nil
				})
			},
		},
		{
			ID:        "client/implicit-flow",
			Title:     "The implicit flow is off",
			Rationale: "The implicit flow returns the access token in the URL fragment, where it lands in browser history, referrers and any script on the page. OAuth 2.1 drops it; Keycloak still offers the checkbox.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "client/implicit-flow"
				return scanClients(id, r, "no enabled client uses the implicit flow (%d checked)", func(c *keycloak.Client) []engine.Finding {
					if !c.ImplicitFlowEnabled {
						return nil
					}
					return []engine.Finding{bad(id, r, clientTarget(c),
						"the implicit flow is enabled: tokens are returned in the URL fragment",
						"switch the client to the authorization code flow with PKCE and turn the implicit flow off")}
				})
			},
		},
		{
			ID:        "client/direct-grant",
			Title:     "The resource owner password grant is off",
			Rationale: "Direct access grants make the client collect the user's password itself: no MFA prompt, no consent, no broker, and a password handled by code that should never see it. On a public client it is also unauthenticated, so anyone can spray passwords against it.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "client/direct-grant"
				return scanClients(id, r, "no enabled client uses the password grant (%d checked)", func(c *keycloak.Client) []engine.Finding {
					if !c.DirectAccessGrantsEnabled {
						return nil
					}
					if c.PublicClient {
						return []engine.Finding{bad(id, r, clientTarget(c),
							"a public client accepts the password grant: password guessing needs no client credential",
							"disable Direct access grants; use the authorization code flow with PKCE")}
					}
					return []engine.Finding{warn(id, r, clientTarget(c),
						"the client accepts the password grant, which bypasses the browser flow and its second factor",
						"disable Direct access grants unless a legacy integration truly requires it")}
				})
			},
		},
		{
			ID:        "client/pkce",
			Title:     "Public clients require PKCE with S256",
			Rationale: "Without PKCE a public client's authorization code can be redeemed by anyone who intercepts it — there is no client secret to stop them. \"plain\" is not a substitute: the verifier travels in the clear.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "client/pkce"
				return scanClients(id, r, "every public client with the code flow requires PKCE S256 (%d clients checked)", func(c *keycloak.Client) []engine.Finding {
					if !c.PublicClient || !c.StandardFlowEnabled || c.BearerOnly || !c.IsOIDC() {
						return nil
					}
					method := strings.TrimSpace(c.Attributes.Get("pkce.code.challenge.method"))
					switch {
					case method == "":
						return []engine.Finding{bad(id, r, clientTarget(c),
							"a public client runs the code flow without requiring PKCE",
							"set the PKCE method to S256 in the client's Advanced settings")}
					case !strings.EqualFold(method, "S256"):
						return []engine.Finding{bad(id, r, clientTarget(c),
							fmt.Sprintf("a public client requires PKCE method %q instead of S256", method),
							"set the PKCE method to S256")}
					}
					return nil
				})
			},
		},
		{
			ID:        "client/full-scope",
			Title:     "Clients do not inherit every role in the realm",
			Rationale: "Full scope allowed (Keycloak's default for a new client) puts every realm and client role the user has into this client's tokens — so a compromise of one frontend yields a token good everywhere. On a service account it is worse: the account holds all realm roles by construction.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "client/full-scope"
				return scanClients(id, r, "no enabled client has full scope over the realm's roles (%d checked)", func(c *keycloak.Client) []engine.Finding {
					if !c.FullScopeAllowed {
						return nil
					}
					if c.ServiceAccountsEnabled {
						return []engine.Finding{bad(id, r, clientTarget(c),
							"a service account client has full scope: its token carries every role in the realm",
							"turn Full scope allowed off and assign the client only the roles it needs")}
					}
					return []engine.Finding{warn(id, r, clientTarget(c),
						"full scope is allowed: the client's tokens carry every role the user has, in every other client",
						"turn Full scope allowed off and add the scopes the client actually reads")}
				})
			},
		},
		{
			ID:        "client/token-lifespan-override",
			Title:     "No client quietly overrides the realm token lifespan",
			Rationale: "A per-client access token lifespan overrides the realm setting, so a realm audited as \"5 minutes\" can still be issuing day-long tokens for one client — and the override lives in the client's Advanced tab where nobody looks.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "client/token-lifespan-override"
				return scanClients(id, r, "no enabled client overrides the realm's token lifespan upwards (%d checked)", func(c *keycloak.Client) []engine.Finding {
					n, ok := c.Attributes.Int("access.token.lifespan")
					if !ok || n <= 0 {
						return nil
					}
					if n > badAccessTokenLifespan {
						return []engine.Finding{bad(id, r, clientTarget(c),
							fmt.Sprintf("the client overrides the access token lifespan to %s", humanSeconds(n)),
							"remove the override, or bring it in line with the realm's lifespan")}
					}
					if n > warnAccessTokenLifespan {
						return []engine.Finding{warn(id, r, clientTarget(c),
							fmt.Sprintf("the client overrides the access token lifespan to %s", humanSeconds(n)),
							"confirm the override is deliberate and documented")}
					}
					return nil
				})
			},
		},
	}
}

// usesRedirects reports whether a client ever redirects a browser back, which is
// what makes its redirect URIs security-relevant. A bearer-only client, or one
// with neither browser flow enabled, does not.
func usesRedirects(c *keycloak.Client) bool {
	return !c.BearerOnly && (c.StandardFlowEnabled || c.ImplicitFlowEnabled)
}
