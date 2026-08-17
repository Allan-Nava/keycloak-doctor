package keycloak

import (
	"encoding/json"
	"testing"
)

// Keycloak exports are not consistent about attribute value types. A single
// non-string value must not take the whole realm down, which is what a plain
// map[string]string would do.
func TestAttrsCoercesScalars(t *testing.T) {
	const raw = `{
		"pkce.code.challenge.method": "S256",
		"access.token.lifespan": 900,
		"saml.server.signature": true,
		"nothing": null
	}`
	var a Attrs
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := a.Get("pkce.code.challenge.method"); got != "S256" {
		t.Errorf("string value = %q", got)
	}
	if n, ok := a.Int("access.token.lifespan"); !ok || n != 900 {
		t.Errorf("number value = %d, ok=%v", n, ok)
	}
	if !a.Bool("saml.server.signature") {
		t.Error("bool value should read as true")
	}
	if a.Has("nothing") {
		t.Error("a null value should not count as present")
	}
	if a.Has("absent") {
		t.Error("an absent key should not count as present")
	}
	if _, ok := a.Int("pkce.code.challenge.method"); ok {
		t.Error("a non-numeric value should not parse as an int")
	}
}

func TestClientAttributesSurviveABoolValue(t *testing.T) {
	const raw = `{"clientId":"web","attributes":{"oauth2.device.authorization.grant.enabled":false}}`
	var c Client
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("a bool attribute must not fail the client: %v", err)
	}
	if c.Attributes.Bool("oauth2.device.authorization.grant.enabled") {
		t.Error("false should not read as true")
	}
}
