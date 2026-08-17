package rules

import (
	"testing"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

// realmWith builds a minimal realm around one client, for rule-level table tests.
func realmWith(c keycloak.Client) *keycloak.Realm {
	return &keycloak.Realm{Realm: "t", Clients: []keycloak.Client{c}}
}

func TestClientPKCERule(t *testing.T) {
	cases := []struct {
		name   string
		client keycloak.Client
		want   engine.Status
	}{
		{"public code flow without PKCE", keycloak.Client{ClientID: "web", PublicClient: true, StandardFlowEnabled: true}, engine.BAD},
		{"public code flow with plain", keycloak.Client{ClientID: "web", PublicClient: true, StandardFlowEnabled: true,
			Attributes: keycloak.Attrs{"pkce.code.challenge.method": "plain"}}, engine.BAD},
		{"public code flow with S256", keycloak.Client{ClientID: "web", PublicClient: true, StandardFlowEnabled: true,
			Attributes: keycloak.Attrs{"pkce.code.challenge.method": "S256"}}, engine.OK},
		// A confidential client authenticates with its secret, so PKCE is advisable
		// rather than load-bearing: not this rule's finding.
		{"confidential client", keycloak.Client{ClientID: "api", StandardFlowEnabled: true}, engine.OK},
		{"bearer only", keycloak.Client{ClientID: "api", PublicClient: true, BearerOnly: true, StandardFlowEnabled: true}, engine.OK},
		{"saml client", keycloak.Client{ClientID: "saml", PublicClient: true, StandardFlowEnabled: true, Protocol: "saml"}, engine.OK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := worstFor(t, "client/pkce", realmWith(c.client)); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestClientRedirectWildcardSeverityFollowsScope(t *testing.T) {
	host := realmWith(keycloak.Client{ClientID: "web", StandardFlowEnabled: true, RedirectURIs: []string{"https://*.example.com/cb"}})
	if got := worstFor(t, "client/redirect-wildcard", host); got != engine.BAD {
		t.Errorf("host wildcard = %s, want BAD", got)
	}
	path := realmWith(keycloak.Client{ClientID: "web", StandardFlowEnabled: true, RedirectURIs: []string{"https://app.example.com/cb/*"}})
	if got := worstFor(t, "client/redirect-wildcard", path); got != engine.WARN {
		t.Errorf("path wildcard = %s, want WARN", got)
	}
	// A bearer-only client never redirects, so its redirect URIs are inert.
	bearer := realmWith(keycloak.Client{ClientID: "api", BearerOnly: true, StandardFlowEnabled: true, RedirectURIs: []string{"*"}})
	if got := worstFor(t, "client/redirect-wildcard", bearer); got != engine.OK {
		t.Errorf("bearer-only client = %s, want OK", got)
	}
}

func TestClientDirectGrantSeverityDependsOnClientType(t *testing.T) {
	public := realmWith(keycloak.Client{ClientID: "app", PublicClient: true, DirectAccessGrantsEnabled: true})
	if got := worstFor(t, "client/direct-grant", public); got != engine.BAD {
		t.Errorf("public client = %s, want BAD", got)
	}
	confidential := realmWith(keycloak.Client{ClientID: "app", DirectAccessGrantsEnabled: true})
	if got := worstFor(t, "client/direct-grant", confidential); got != engine.WARN {
		t.Errorf("confidential client = %s, want WARN", got)
	}
}

func TestClientFullScopeSeverityDependsOnServiceAccount(t *testing.T) {
	service := realmWith(keycloak.Client{ClientID: "worker", ServiceAccountsEnabled: true, FullScopeAllowed: true})
	if got := worstFor(t, "client/full-scope", service); got != engine.BAD {
		t.Errorf("service account = %s, want BAD", got)
	}
	plain := realmWith(keycloak.Client{ClientID: "web", StandardFlowEnabled: true, FullScopeAllowed: true})
	if got := worstFor(t, "client/full-scope", plain); got != engine.WARN {
		t.Errorf("regular client = %s, want WARN", got)
	}
}

func TestClientTokenLifespanOverride(t *testing.T) {
	cases := map[string]struct {
		lifespan string
		want     engine.Status
	}{
		"no override":    {"", engine.OK},
		"within realm":   {"300", engine.OK},
		"twenty minutes": {"1200", engine.WARN},
		"a day":          {"86400", engine.BAD},
		"not a number":   {"forever", engine.OK},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			client := keycloak.Client{ClientID: "web", StandardFlowEnabled: true}
			if c.lifespan != "" {
				client.Attributes = keycloak.Attrs{"access.token.lifespan": c.lifespan}
			}
			if got := worstFor(t, "client/token-lifespan-override", realmWith(client)); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestMapperSensitiveAttribute(t *testing.T) {
	loud := []string{"api_key", "password_hash", "user-secret", "privateKey", "otp_seed", "REFRESH_TOKEN"}
	for _, attr := range loud {
		realm := realmWith(keycloak.Client{ClientID: "web", ProtocolMappers: []keycloak.ProtocolMapper{
			{Name: "m", ProtocolMapper: "oidc-usermodel-attribute-mapper", Config: keycloak.Attrs{"user.attribute": attr, "claim.name": "x"}},
		}})
		if got := worstFor(t, "mapper/sensitive-attribute", realm); got != engine.BAD {
			t.Errorf("attribute %q = %s, want BAD", attr, got)
		}
	}
	quiet := []string{"email", "department", "locale", "employeeNumber"}
	for _, attr := range quiet {
		realm := realmWith(keycloak.Client{ClientID: "web", ProtocolMappers: []keycloak.ProtocolMapper{
			{Name: "m", ProtocolMapper: "oidc-usermodel-attribute-mapper", Config: keycloak.Attrs{"user.attribute": attr}},
		}})
		if got := worstFor(t, "mapper/sensitive-attribute", realm); got != engine.OK {
			t.Errorf("attribute %q = %s, want OK", attr, got)
		}
	}
}

func TestMapperHardcodedAudience(t *testing.T) {
	foreign := realmWith(keycloak.Client{ClientID: "web", ProtocolMappers: []keycloak.ProtocolMapper{
		{Name: "aud", ProtocolMapper: "oidc-audience-mapper", Config: keycloak.Attrs{"included.client.audience": "payments-api"}},
	}})
	if got := worstFor(t, "mapper/hardcoded-audience", foreign); got != engine.WARN {
		t.Errorf("foreign audience = %s, want WARN", got)
	}
	own := realmWith(keycloak.Client{ClientID: "web", ProtocolMappers: []keycloak.ProtocolMapper{
		{Name: "aud", ProtocolMapper: "oidc-audience-mapper", Config: keycloak.Attrs{"included.client.audience": "web"}},
	}})
	if got := worstFor(t, "mapper/hardcoded-audience", own); got != engine.OK {
		t.Errorf("own audience = %s, want OK", got)
	}
}
