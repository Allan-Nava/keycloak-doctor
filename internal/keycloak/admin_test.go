package keycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testSecret = "test-only-client-secret"

// fakeKeycloak serves the handful of Admin API endpoints the loader reads. The
// tests never touch a real server: a module that needs live infrastructure to be
// tested cannot be trusted to run in CI.
func fakeKeycloak(t *testing.T, forbid map[string]bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("grant_type") != "client_credentials" || r.PostForm.Get("client_secret") != testSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":60}`))
	})
	serve := func(section, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if forbid[section] {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc("/admin/realms", serve("realms", `[{"realm":"demo"},{"realm":"master"}]`))
	mux.HandleFunc("/admin/realms/demo", serve("realm", `{
		"realm":"demo","enabled":true,"sslRequired":"all","bruteForceProtected":true,
		"accessTokenLifespan":300,"browserFlow":"browser"}`))
	mux.HandleFunc("/admin/realms/demo/clients", serve("clients", `[{
		"clientId":"web","enabled":true,"publicClient":true,"standardFlowEnabled":true,
		"redirectUris":["https://app.example.com/cb"],"secret":"`+testSecret+`",
		"attributes":{"pkce.code.challenge.method":"S256"}}]`))
	mux.HandleFunc("/admin/realms/demo/identity-provider/instances", serve("idps", `[{
		"alias":"corp","providerId":"oidc","enabled":true,"config":{"tokenUrl":"https://idp.example.com/token"}}]`))
	mux.HandleFunc("/admin/realms/demo/components", serve("components", `[{
		"name":"rsa-primary","providerId":"rsa-generated","providerType":"org.keycloak.keys.KeyProvider",
		"config":{"keySize":["4096"]}}]`))
	mux.HandleFunc("/admin/realms/demo/authentication/flows", serve("flows", `[{"alias":"browser","topLevel":true,"builtIn":true}]`))
	mux.HandleFunc("/admin/realms/demo/authentication/flows/browser/executions", serve("flows", `[
		{"providerId":"auth-cookie","requirement":"ALTERNATIVE","level":0},
		{"displayName":"forms","requirement":"ALTERNATIVE","level":0,"authenticationFlow":true},
		{"providerId":"auth-otp-form","requirement":"REQUIRED","level":1}]`))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testAdmin(t *testing.T, baseURL string) *Admin {
	t.Helper()
	admin, err := NewAdmin(AdminOptions{
		BaseURL: baseURL, ClientID: "audit", ClientSecret: testSecret, Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new admin: %v", err)
	}
	return admin
}

func TestAdminFetchRealmAssemblesEverySection(t *testing.T) {
	srv := fakeKeycloak(t, nil)
	admin := testAdmin(t, srv.URL)

	realm, err := admin.FetchRealm(context.Background(), "demo")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if realm.Realm != "demo" || realm.SSLRequired != "all" {
		t.Fatalf("realm not decoded: %+v", realm)
	}
	if len(realm.Clients) != 1 || realm.Clients[0].ClientID != "web" {
		t.Fatalf("clients not assembled: %+v", realm.Clients)
	}
	if len(realm.IdentityProviders) != 1 {
		t.Errorf("identity providers not assembled: %+v", realm.IdentityProviders)
	}
	comps := realm.Components()
	if len(comps) != 1 || comps[0].ProviderType != "org.keycloak.keys.KeyProvider" {
		t.Errorf("components not assembled by type: %+v", comps)
	}
	if len(realm.AuthenticationFlows) != 1 || len(realm.AuthenticationFlows[0].Executions) != 3 {
		t.Fatalf("flow executions not assembled: %+v", realm.AuthenticationFlows)
	}
	// The executions endpoint names the authenticator providerId, the export names
	// it authenticator: the rules must see one shape.
	if got := realm.AuthenticationFlows[0].Executions[2].Provider(); got != "auth-otp-form" {
		t.Errorf("Provider() = %q, want auth-otp-form", got)
	}
	if !strings.HasPrefix(realm.Origin, "api:") {
		t.Errorf("origin = %q, want an api: prefix", realm.Origin)
	}
	// The API hands out client secrets to a token that may read them; they must not
	// survive the loader either.
	if realm.Clients[0].Secret != "" || !realm.Clients[0].SecretSet {
		t.Errorf("client secret not scrubbed: %+v", realm.Clients[0])
	}
}

// A token with view-realm but not view-clients must still audit everything else,
// and the sections it could not read must be marked rather than silently empty.
func TestAdminFetchRealmMarksForbiddenSections(t *testing.T) {
	srv := fakeKeycloak(t, map[string]bool{"clients": true})
	admin := testAdmin(t, srv.URL)

	realm, err := admin.FetchRealm(context.Background(), "demo")
	if err != nil {
		t.Fatalf("a forbidden section must not fail the fetch: %v", err)
	}
	if realm.Unavailable("clients") == "" {
		t.Error("the clients section should be marked unavailable")
	}
	if realm.Unavailable("components") != "" {
		t.Error("the components section was readable and must not be marked")
	}
}

func TestAdminRealmNames(t *testing.T) {
	srv := fakeKeycloak(t, nil)
	got, err := testAdmin(t, srv.URL).RealmNames(context.Background())
	if err != nil {
		t.Fatalf("realm names: %v", err)
	}
	if len(got) != 2 || got[0] != "demo" || got[1] != "master" {
		t.Errorf("realm names = %v", got)
	}
}

// An authentication error must say what failed without echoing the credential.
func TestAdminLoginErrorHidesTheSecret(t *testing.T) {
	srv := fakeKeycloak(t, nil)
	admin, err := NewAdmin(AdminOptions{
		BaseURL: srv.URL, ClientID: "audit", ClientSecret: "wrong-secret-value", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new admin: %v", err)
	}
	err = admin.Login(context.Background())
	if err == nil {
		t.Fatal("wrong credentials should fail")
	}
	if strings.Contains(err.Error(), "wrong-secret-value") {
		t.Errorf("the error echoes the secret: %v", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("the error should carry the status: %v", err)
	}
}

func TestNewAdminValidatesOptions(t *testing.T) {
	cases := map[string]AdminOptions{
		"no url":         {ClientID: "a", ClientSecret: "b"},
		"bad url":        {BaseURL: "not-a-url", ClientID: "a", ClientSecret: "b"},
		"no credentials": {BaseURL: "https://sso.example.com"},
		"half a client":  {BaseURL: "https://sso.example.com", ClientID: "a"},
	}
	for name, opts := range cases {
		if _, err := NewAdmin(opts); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
	admin, err := NewAdmin(AdminOptions{BaseURL: "https://sso.example.com/", Username: "admin", Password: "x"})
	if err != nil {
		t.Fatalf("password grant should be accepted: %v", err)
	}
	if admin.opts.AuthRealm != "master" {
		t.Errorf("auth realm should default to master, got %q", admin.opts.AuthRealm)
	}
}
