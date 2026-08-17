package rules

import (
	"strconv"
	"strings"
)

// passwordPolicy is a parsed Keycloak password policy: policy name → argument.
type passwordPolicy map[string]string

// num reads a policy argument as an integer.
func (p passwordPolicy) num(name string) (int, bool) {
	v := strings.TrimSpace(p[name])
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parsePasswordPolicy parses the string form Keycloak stores a policy in:
//
//	length(12) and notUsername(undefined) and hashAlgorithm(argon2)
//
// Policies without an argument keep an empty value, so presence and value are
// distinguishable (notUsername has no meaningful argument but its presence is the
// whole point).
func parsePasswordPolicy(s string) passwordPolicy {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := passwordPolicy{}
	for _, part := range strings.Split(s, " and ") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, arg := part, ""
		if i := strings.Index(part, "("); i > 0 && strings.HasSuffix(part, ")") {
			name, arg = strings.TrimSpace(part[:i]), strings.TrimSpace(part[i+1:len(part)-1])
			if arg == "undefined" {
				arg = ""
			}
		}
		out[name] = arg
	}
	return out
}
