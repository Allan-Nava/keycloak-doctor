package rules

import (
	"net/url"
	"strings"
)

// wildcardScope says how far a wildcard in a redirect URI reaches.
type wildcardScope int

const (
	noWildcard wildcardScope = iota
	// pathWildcard: the wildcard is in the path or query, so the host is still
	// pinned — the risk is an open redirect inside a host you control.
	pathWildcard
	// hostWildcard: the wildcard covers the scheme or the authority, so the
	// authorization code can be sent to a host the attacker picks.
	hostWildcard
)

// classifyRedirect locates a wildcard in a redirect URI.
//
// Keycloak's matcher treats "*" as "anything from here on", so where the wildcard
// sits is the whole question: "https://app.example.com/cb/*" widens a path,
// "https://*.example.com/cb" or "*" hands the code to whoever answers.
func classifyRedirect(uri string) wildcardScope {
	u := strings.TrimSpace(uri)
	if u == "" || !strings.Contains(u, "*") {
		return noWildcard
	}
	rest := u
	if i := strings.Index(u, "://"); i >= 0 {
		if strings.Contains(u[:i], "*") {
			return hostWildcard
		}
		rest = u[i+3:]
	} else if !strings.HasPrefix(u, "/") {
		// No scheme and not root-relative: a bare "*" or a custom-scheme pattern.
		// Nothing pins the target.
		return hostWildcard
	} else {
		// Root-relative URIs are resolved against the client's root URL, so the
		// host stays pinned.
		return pathWildcard
	}
	authority := rest
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		authority = rest[:j]
	}
	if strings.Contains(authority, "*") {
		return hostWildcard
	}
	return pathWildcard
}

// plainHTTPHost returns the host of a redirect URI served over plaintext HTTP,
// or "" when the URI is not plaintext HTTP or points at the loopback interface
// (where http:// is the normal, and specified, case for native and dev clients).
func plainHTTPHost(uri string) string {
	u, err := url.Parse(strings.TrimSpace(uri))
	if err != nil || !strings.EqualFold(u.Scheme, "http") {
		return ""
	}
	host := u.Hostname()
	if isLoopback(host) {
		return ""
	}
	if host == "" {
		return u.Host
	}
	return host
}

func isLoopback(host string) bool {
	h := strings.ToLower(host)
	switch h {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return strings.HasSuffix(h, ".localhost")
}

// hasPlainHTTPEndpoint reports whether a broker/component URL is plaintext HTTP
// to a non-loopback host.
func hasPlainHTTPEndpoint(rawURL string) bool {
	return plainHTTPHost(rawURL) != ""
}
