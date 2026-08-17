package keycloak

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const insecureFixture = "../../testdata/insecure-realm.json"

func TestLoadFileSingleRealm(t *testing.T) {
	realms, err := LoadFile(insecureFixture)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(realms) != 1 {
		t.Fatalf("got %d realms, want 1", len(realms))
	}
	r := realms[0]
	if r.Realm != "demo" {
		t.Errorf("realm name = %q", r.Realm)
	}
	if !strings.HasPrefix(r.Origin, "file:") {
		t.Errorf("origin = %q, want a file: prefix", r.Origin)
	}
	if len(r.Clients) != 3 {
		t.Errorf("got %d clients, want 3", len(r.Clients))
	}
	if !r.IsEnabled() {
		t.Error("the realm should read as enabled")
	}
	if got := len(r.Components()); got != 2 {
		t.Errorf("got %d components, want 2", got)
	}
	for _, c := range r.Components() {
		if c.ProviderType == "" {
			t.Errorf("component %q has no provider type: the map key was not applied", c.Name)
		}
	}
}

// The loader is the only place credentials exist. Everything downstream — rules,
// findings, output — must be unable to print one, so the values are dropped here
// and only their presence survives.
func TestLoadFileScrubsCredentials(t *testing.T) {
	realms, err := LoadFile(insecureFixture)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	r := realms[0]
	secretsSeen := 0
	for _, c := range r.Clients {
		if c.Secret != "" {
			t.Errorf("client %q still carries a secret value", c.ClientID)
		}
		if c.SecretSet {
			secretsSeen++
		}
	}
	if secretsSeen != 1 {
		t.Errorf("got %d clients flagged as having a secret, want 1", secretsSeen)
	}
	for _, idp := range r.IdentityProviders {
		if idp.Config.Has("clientSecret") {
			t.Errorf("broker %q still carries a client secret", idp.Alias)
		}
		if !idp.SecretSet {
			t.Errorf("broker %q should be flagged as having had a secret", idp.Alias)
		}
	}
	for _, c := range r.Components() {
		if c.Cfg("bindCredential") != "" {
			t.Errorf("component %q still carries a bind credential", c.Name)
		}
		if c.ProviderID == "ldap" && len(c.SecretKeys) == 0 {
			t.Errorf("component %q should record the credential key it dropped", c.Name)
		}
	}
}

func TestLoadFileArrayAndDirectory(t *testing.T) {
	dir := t.TempDir()
	one, err := os.ReadFile(insecureFixture)
	if err != nil {
		t.Fatal(err)
	}
	two, err := os.ReadFile("../../testdata/hardened-realm.json")
	if err != nil {
		t.Fatal(err)
	}

	// An array of realms in one file, the shape of a full export.
	arrayPath := filepath.Join(dir, "all-realms.json")
	array := append([]byte("["), one...)
	array = append(array, ',')
	array = append(array, two...)
	array = append(array, ']')
	if err := os.WriteFile(arrayPath, array, 0o600); err != nil {
		t.Fatal(err)
	}
	realms, err := LoadFile(arrayPath)
	if err != nil {
		t.Fatalf("array form: %v", err)
	}
	if len(realms) != 2 {
		t.Fatalf("array form: got %d realms, want 2", len(realms))
	}

	// A directory export: realm files next to the user files it must skip.
	exportDir := filepath.Join(dir, "export")
	if err := os.Mkdir(exportDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exportDir, "demo-realm.json"), one, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exportDir, "demo-users-0.json"), []byte(`{"users":[{"username":"alice"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exportDir, "notes.json"), []byte(`{"unrelated":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	realms, err = LoadFile(exportDir)
	if err != nil {
		t.Fatalf("directory form: %v", err)
	}
	if len(realms) != 1 || realms[0].Realm != "demo" {
		t.Fatalf("directory form: got %d realms (%v), want just demo", len(realms), RealmNames(realms))
	}
}

func TestLoadFileRejectsNonRealmFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, []byte(`{"users":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Error("a file with no realm should be an error when named explicitly")
	}
}

func TestSelectRealms(t *testing.T) {
	realms := []*Realm{{Realm: "demo"}, {Realm: "master"}}
	got, err := SelectRealms(realms, []string{"MASTER"})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(got) != 1 || got[0].Realm != "master" {
		t.Errorf("case-insensitive selection failed: %v", RealmNames(got))
	}
	if all, err := SelectRealms(realms, nil); err != nil || len(all) != 2 {
		t.Errorf("no names should select everything, got %d (%v)", len(all), err)
	}
	// A typo must fail loudly: auditing nothing and reporting success is the worst
	// possible outcome for a security tool.
	if _, err := SelectRealms(realms, []string{"prod"}); err == nil {
		t.Error("an unmatched realm name should be an error")
	}
}
