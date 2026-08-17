package rules

import (
	"testing"

	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

func TestClassifyRedirect(t *testing.T) {
	cases := map[string]wildcardScope{
		"https://app.example.com/callback":   noWildcard,
		"":                                   noWildcard,
		"*":                                  hostWildcard,
		"https://*":                          hostWildcard,
		"https://*.example.com/cb":           hostWildcard,
		"http://*":                           hostWildcard,
		"*://app.example.com/cb":             hostWildcard,
		"https://app.example.com/cb/*":       pathWildcard,
		"https://app.example.com/cb?next=*":  pathWildcard,
		"/callback/*":                        pathWildcard,
		"myapp://callback":                   noWildcard,
		"myapp://callback/*":                 pathWildcard,
		"https://app.example.com:8443/cb/*":  pathWildcard,
		"https://app.example.com*.evil.test": hostWildcard,
	}
	for uri, want := range cases {
		if got := classifyRedirect(uri); got != want {
			t.Errorf("classifyRedirect(%q) = %v, want %v", uri, got, want)
		}
	}
}

func TestPlainHTTPHost(t *testing.T) {
	// Loopback over http is the specified case for native and development clients,
	// so it must not be reported; a real hostname must be.
	quiet := []string{
		"https://app.example.com/cb",
		"http://localhost:8080/cb",
		"http://127.0.0.1/cb",
		"http://[::1]:3000/cb",
		"http://app.localhost/cb",
		"myapp://callback",
	}
	for _, uri := range quiet {
		if got := plainHTTPHost(uri); got != "" {
			t.Errorf("plainHTTPHost(%q) = %q, want no finding", uri, got)
		}
	}
	loud := map[string]string{
		"http://app.example.com/cb":     "app.example.com",
		"http://10.0.0.5:8080/cb":       "10.0.0.5",
		"HTTP://APP.EXAMPLE.COM/cb":     "APP.EXAMPLE.COM",
		"http://idp.internal/authorize": "idp.internal",
	}
	for uri, want := range loud {
		if got := plainHTTPHost(uri); got != want {
			t.Errorf("plainHTTPHost(%q) = %q, want %q", uri, got, want)
		}
	}
}

func TestParsePasswordPolicy(t *testing.T) {
	got := parsePasswordPolicy("length(12) and notUsername(undefined) and hashAlgorithm(pbkdf2-sha512) and hashIterations(210000)")
	if n, ok := got.num("length"); !ok || n != 12 {
		t.Errorf("length = %d (%v)", n, ok)
	}
	if _, ok := got["notUsername"]; !ok {
		t.Error("notUsername should be present even with an undefined argument")
	}
	if got["hashAlgorithm"] != "pbkdf2-sha512" {
		t.Errorf("hashAlgorithm = %q", got["hashAlgorithm"])
	}
	if n, ok := got.num("hashIterations"); !ok || n != 210000 {
		t.Errorf("hashIterations = %d (%v)", n, ok)
	}
	if parsePasswordPolicy("") != nil {
		t.Error("an empty policy should parse to nil, which is what the rule reports on")
	}
	if _, ok := parsePasswordPolicy("length(abc)").num("length"); ok {
		t.Error("a non-numeric argument should not parse as a number")
	}
}

func TestHumanSeconds(t *testing.T) {
	cases := map[int]string{0: "0s", 45: "45s", 300: "5m", 3600: "1h", 36000: "10h", 2592000: "30d", 5184000: "60d", 90: "90s"}
	for in, want := range cases {
		if got := humanSeconds(in); got != want {
			t.Errorf("humanSeconds(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSecondFactor(t *testing.T) {
	for _, a := range []string{"auth-otp-form", "conditional-user-configured-otp", "webauthn-authenticator", "auth-recovery-authn-code-form", "WEBAUTHN-AUTHENTICATOR-PASSWORDLESS"} {
		if !isSecondFactor(a) {
			t.Errorf("%q should count as a second factor", a)
		}
	}
	for _, a := range []string{"auth-cookie", "auth-username-password-form", "identity-provider-redirector"} {
		if isSecondFactor(a) {
			t.Errorf("%q should not count as a second factor", a)
		}
	}
}

// The two sources shape flows differently; the walk must handle both, and a
// DISABLED step must not count as protection.
func TestReachableAuthenticators(t *testing.T) {
	exportShape := &keycloak.Realm{
		AuthenticationFlows: []keycloak.AuthFlow{
			{Alias: "browser", Executions: []keycloak.AuthExecution{
				{Authenticator: "auth-cookie", Requirement: "ALTERNATIVE"},
				{AuthenticatorFlow: true, FlowAlias: "forms", Requirement: "ALTERNATIVE"},
			}},
			{Alias: "forms", Executions: []keycloak.AuthExecution{
				{Authenticator: "auth-username-password-form", Requirement: "REQUIRED"},
				{Authenticator: "auth-otp-form", Requirement: "DISABLED"},
			}},
		},
	}
	got, found := reachableAuthenticators(exportShape, "browser")
	if !found {
		t.Fatal("the browser flow should be found")
	}
	for _, a := range got {
		if a == "auth-otp-form" {
			t.Error("a DISABLED execution must not be reported as reachable")
		}
	}
	if len(got) != 2 {
		t.Errorf("reachable = %v, want the cookie and password steps", got)
	}

	apiShape := &keycloak.Realm{
		AuthenticationFlows: []keycloak.AuthFlow{
			{Alias: "browser", Executions: []keycloak.AuthExecution{
				{ProviderID: "auth-cookie", Requirement: "ALTERNATIVE"},
				{DisplayName: "forms", Requirement: "ALTERNATIVE"},
				{ProviderID: "auth-otp-form", Requirement: "REQUIRED"},
			}},
		},
	}
	got, found = reachableAuthenticators(apiShape, "browser")
	if !found || len(got) != 2 {
		t.Fatalf("flat API shape: reachable = %v (found=%v)", got, found)
	}

	if _, found := reachableAuthenticators(exportShape, "no-such-flow"); found {
		t.Error("a missing flow must report not found rather than an empty flow")
	}
}

// A subflow that points back at its parent must not loop forever.
func TestReachableAuthenticatorsHandlesCycles(t *testing.T) {
	realm := &keycloak.Realm{
		AuthenticationFlows: []keycloak.AuthFlow{
			{Alias: "a", Executions: []keycloak.AuthExecution{{AuthenticatorFlow: true, FlowAlias: "b"}}},
			{Alias: "b", Executions: []keycloak.AuthExecution{
				{AuthenticatorFlow: true, FlowAlias: "a"},
				{Authenticator: "auth-otp-form"},
			}},
		},
	}
	got, found := reachableAuthenticators(realm, "a")
	if !found || len(got) != 1 || got[0] != "auth-otp-form" {
		t.Fatalf("reachable = %v (found=%v)", got, found)
	}
}
