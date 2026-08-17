package keycloak

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadFile reads realms from a Keycloak export.
//
// It accepts the three shapes an export comes in:
//
//   - a single realm object (`kc.sh export --realm x --file x.json`),
//   - an array of realm objects (a full export written to one file),
//   - a directory, where every *.json file is read and the ones that are not a
//     realm (the `*-users-N.json` companions of a directory export) are skipped.
//
// Every realm returned has been scrubbed of credential values.
func LoadFile(path string) ([]*Realm, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return loadDir(path)
	}
	realms, err := loadOneFile(path)
	if err != nil {
		return nil, err
	}
	if len(realms) == 0 {
		return nil, fmt.Errorf("%s: no realm found (is this a Keycloak realm export?)", path)
	}
	return realms, nil
}

func loadDir(dir string) ([]*Realm, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	var out []*Realm
	for _, e := range entries {
		realms, err := loadOneFile(e)
		if err != nil {
			// A directory export mixes realm files with user files and whatever
			// else the operator left there; only a hard read error is fatal.
			if os.IsNotExist(err) || os.IsPermission(err) {
				return nil, err
			}
			continue
		}
		out = append(out, realms...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no realm export found in the directory", dir)
	}
	return out, nil
}

// loadOneFile decodes one file, returning the realms it holds. A file that parses
// as JSON but carries no realm name is not a realm export and yields no realms
// rather than an error, so a directory sweep can skip it.
func loadOneFile(path string) ([]*Realm, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the path is the operator's own argument
	if err != nil {
		return nil, err
	}
	origin := "file:" + filepath.Base(path)
	switch first := firstToken(b); first {
	case '[':
		var many []*Realm
		if err := json.Unmarshal(b, &many); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return finish(many, origin), nil
	case '{':
		var one Realm
		if err := json.Unmarshal(b, &one); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return finish([]*Realm{&one}, origin), nil
	default:
		return nil, fmt.Errorf("%s: not a JSON object or array", path)
	}
}

// finish drops non-realm entries, stamps the origin and scrubs credentials.
func finish(realms []*Realm, origin string) []*Realm {
	out := realms[:0:0]
	for _, r := range realms {
		if r == nil || strings.TrimSpace(r.Realm) == "" {
			continue
		}
		r.Origin = origin
		r.Scrub()
		out = append(out, r)
	}
	return out
}

func firstToken(b []byte) byte {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n', 0xef, 0xbb, 0xbf: // whitespace and a UTF-8 BOM
			continue
		default:
			return c
		}
	}
	return 0
}

// SelectRealms keeps the realms whose name matches one of names (case-insensitive).
// With no names every realm is kept. An unmatched name is an error: silently
// auditing nothing because of a typo in --realm is the worst outcome for a
// security tool.
func SelectRealms(realms []*Realm, names []string) ([]*Realm, error) {
	if len(names) == 0 {
		return realms, nil
	}
	out := realms[:0:0]
	for _, want := range names {
		found := false
		for _, r := range realms {
			if strings.EqualFold(r.Realm, want) {
				out = append(out, r)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("realm %q not found in the source (available: %s)", want, strings.Join(RealmNames(realms), ", "))
		}
	}
	return out, nil
}

// RealmNames lists the realm names in order.
func RealmNames(realms []*Realm) []string {
	out := make([]string, 0, len(realms))
	for _, r := range realms {
		out = append(out, r.Name())
	}
	return out
}
