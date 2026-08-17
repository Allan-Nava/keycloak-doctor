package rules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

// keyProviderType is the component provider type of realm key providers.
const keyProviderType = "org.keycloak.keys.KeyProvider"

// minRSAKeySize is the floor for a signing key: below 2048 bits the key is inside
// factoring range of published results, and every token this realm ever signed
// stays forgeable once it falls.
const minRSAKeySize = 2048

// minHMACSecretSize is the floor, in bytes, for an HMAC signing secret: below 32
// the secret is weaker than the SHA-256 it feeds.
const minHMACSecretSize = 32

func keyRules() []Rule {
	return []Rule{
		{
			ID:        "keys/rsa-size",
			Title:     "RSA signing keys are at least 2048 bits",
			Rationale: "The realm's RSA key signs every token it issues. A 1024-bit key is not a configuration preference: forge one signature and you can mint any identity, retroactively, for as long as the key stays published in the JWKS.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "keys/rsa-size"
				return scanComponents(id, r, keyProviderType, "rsa", "every RSA signing key is %[1]d bits or more (%[2]d provider(s) checked)", func(c *keycloak.Component) []engine.Finding {
					raw := c.Cfg("keySize")
					if raw == "" {
						return nil
					}
					n, err := strconv.Atoi(raw)
					if err != nil {
						return nil
					}
					if n < minRSAKeySize {
						return []engine.Finding{bad(id, r, c.Label(),
							fmt.Sprintf("key provider %q signs with a %d-bit RSA key", c.Label(), n),
							"create a 2048-bit (or larger) provider, make it active, and keep the old one passive only until the tokens it signed expire")}
					}
					return nil
				}, minRSAKeySize)
			},
		},
		{
			ID:        "keys/hmac-secret-size",
			Title:     "HMAC signing secrets are at least 32 bytes",
			Rationale: "An HMAC secret shorter than the digest it feeds is the weak link in the chain: it is brute-forceable offline from a single token, and a recovered secret mints valid tokens for the whole realm.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "keys/hmac-secret-size"
				return scanComponents(id, r, keyProviderType, "hmac", "every HMAC secret is %[1]d bytes or more (%[2]d provider(s) checked)", func(c *keycloak.Component) []engine.Finding {
					raw := c.Cfg("secretSize")
					if raw == "" {
						return nil
					}
					n, err := strconv.Atoi(raw)
					if err != nil {
						return nil
					}
					if n < minHMACSecretSize {
						return []engine.Finding{warn(id, r, c.Label(),
							fmt.Sprintf("key provider %q uses a %d-byte HMAC secret", c.Label(), n),
							"recreate the provider with a secret of 32 bytes or more")}
					}
					return nil
				}, minHMACSecretSize)
			},
		},
	}
}

// scanComponents applies eval to every component of one provider type whose
// provider id contains providerMatch.
//
// okFormat is rendered with the rule's threshold as %[1]d and the number of
// components actually checked as %[2]d, so the aggregate OK states the bar that
// was applied and to how many providers — never "all fine" over an empty set,
// which is why the match is a parameter here instead of a test inside eval.
func scanComponents(rule string, r *keycloak.Realm, providerType, providerMatch, okFormat string, eval func(*keycloak.Component) []engine.Finding, threshold int) []engine.Finding {
	if reason := r.Unavailable("components"); reason != "" {
		return []engine.Finding{unevaluated(rule, r, "components")}
	}
	var out []engine.Finding
	checked := 0
	for _, c := range r.Components() {
		if c.ProviderType != providerType || !strings.Contains(strings.ToLower(c.ProviderID), providerMatch) {
			continue
		}
		checked++
		comp := c
		out = append(out, eval(&comp)...)
	}
	if len(out) == 0 {
		if checked == 0 {
			return nil // the source carried no such component: nothing to claim
		}
		return []engine.Finding{pass(rule, r, fmt.Sprintf(okFormat, threshold, checked))}
	}
	return out
}
