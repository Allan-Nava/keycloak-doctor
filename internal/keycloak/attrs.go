package keycloak

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Attrs is a Keycloak attribute/config map.
//
// Keycloak documents these as string→string, but exports in the wild are not
// consistent: depending on the version and on how the object was created, the
// same key can come back as a JSON string, a number or a bool
// ("pkce.code.challenge.method": "S256" next to "saml.server.signature": true).
// A plain map[string]string fails to unmarshal on the whole realm the moment one
// value is not a string, which would take the audit down over a formatting
// detail — so every scalar is coerced to its text form here.
type Attrs map[string]string

// UnmarshalJSON decodes an attribute map, coercing scalar values to strings.
func (a *Attrs) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	out := make(Attrs, len(raw))
	for k, v := range raw {
		out[k] = scalarString(v)
	}
	*a = out
	return nil
}

// scalarString renders a JSON scalar as text; anything else (object, array) is
// kept verbatim so a rule can still match on it.
func scalarString(v json.RawMessage) string {
	s := strings.TrimSpace(string(v))
	if s == "" || s == "null" {
		return ""
	}
	var str string
	if err := json.Unmarshal(v, &str); err == nil {
		return str
	}
	return s
}

// Get returns the value for key, or "" when absent.
func (a Attrs) Get(key string) string { return a[key] }

// Has reports whether key is present with a non-empty value.
func (a Attrs) Has(key string) bool { return strings.TrimSpace(a[key]) != "" }

// Bool reports whether key holds a true-ish value ("true", case-insensitive).
func (a Attrs) Bool(key string) bool {
	return strings.EqualFold(strings.TrimSpace(a[key]), "true")
}

// Int parses key as an integer; ok is false when the key is absent or not numeric.
func (a Attrs) Int(key string) (n int, ok bool) {
	v := strings.TrimSpace(a[key])
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}
