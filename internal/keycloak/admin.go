package keycloak

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AdminOptions configures a client of the Keycloak Admin REST API.
//
// Credentials are never taken as literals from the command line: the CLI reads
// them from the environment and passes them here.
type AdminOptions struct {
	BaseURL      string // https://keycloak.example.com (with or without /auth)
	AuthRealm    string // realm holding the admin client or user; defaults to master
	ClientID     string // service account client for client_credentials
	ClientSecret string
	Username     string // alternative: password grant, usually with client admin-cli
	Password     string
	Insecure     bool // skip TLS verification (self-signed lab servers)
	Timeout      time.Duration
}

// Admin reads realm configuration from a running Keycloak.
type Admin struct {
	opts  AdminOptions
	http  *http.Client
	token string
}

// NewAdmin validates the options and builds a client. It performs no IO.
func NewAdmin(opts AdminOptions) (*Admin, error) {
	if strings.TrimSpace(opts.BaseURL) == "" {
		return nil, errors.New("no server URL")
	}
	u, err := url.Parse(strings.TrimRight(opts.BaseURL, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("invalid server URL %q", opts.BaseURL)
	}
	if opts.AuthRealm == "" {
		opts.AuthRealm = "master"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	hasClient := opts.ClientID != "" && opts.ClientSecret != ""
	hasUser := opts.Username != "" && opts.Password != ""
	if !hasClient && !hasUser {
		return nil, errors.New("no credentials: pass --client-id with --client-secret-env, or --username with --password-env")
	}
	opts.BaseURL = u.String()
	tr := &http.Transport{}
	if opts.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in via --insecure, for lab servers with a self-signed chain
	}
	return &Admin{opts: opts, http: &http.Client{Timeout: opts.Timeout, Transport: tr}}, nil
}

// Login obtains an access token. It is called by the fetch methods on demand, so
// callers rarely need it directly.
func (a *Admin) Login(ctx context.Context) error {
	form := url.Values{}
	if a.opts.ClientID != "" && a.opts.ClientSecret != "" {
		form.Set("grant_type", "client_credentials")
		form.Set("client_id", a.opts.ClientID)
		form.Set("client_secret", a.opts.ClientSecret)
	} else {
		form.Set("grant_type", "password")
		form.Set("client_id", orDefault(a.opts.ClientID, "admin-cli"))
		form.Set("username", a.opts.Username)
		form.Set("password", a.opts.Password)
	}
	endpoint := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", a.opts.BaseURL, url.PathEscape(a.opts.AuthRealm))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// The response body of a failed token request is not echoed: it can carry
		// back parts of the request. The status is enough to tell apart wrong
		// credentials (401) from a client that is not allowed the grant (400).
		return fmt.Errorf("authentication against realm %q failed: HTTP %d", a.opts.AuthRealm, resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return fmt.Errorf("token response is not JSON: %w", err)
	}
	if body.AccessToken == "" {
		return errors.New("token response carried no access_token")
	}
	a.token = body.AccessToken
	return nil
}

// RealmNames lists the realms the credentials can see.
func (a *Admin) RealmNames(ctx context.Context) ([]string, error) {
	var realms []struct {
		Realm string `json:"realm"`
	}
	if err := a.get(ctx, "/admin/realms", &realms); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(realms))
	for _, r := range realms {
		if r.Realm != "" {
			out = append(out, r.Realm)
		}
	}
	sort.Strings(out)
	return out, nil
}

// FetchRealms reads the named realms; with no names it reads every realm the
// credentials can see.
func (a *Admin) FetchRealms(ctx context.Context, names []string) ([]*Realm, error) {
	if len(names) == 0 {
		discovered, err := a.RealmNames(ctx)
		if err != nil {
			return nil, err
		}
		if len(discovered) == 0 {
			return nil, errors.New("the credentials can see no realm (needs the view-realm role)")
		}
		names = discovered
	}
	out := make([]*Realm, 0, len(names))
	for _, name := range names {
		r, err := a.FetchRealm(ctx, name)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// FetchRealm assembles one realm from the Admin API into the same model a realm
// export decodes into.
//
// A section the credentials may not read (HTTP 403) is recorded in Realm.Missing
// rather than failing the run: a token with view-realm but not view-clients still
// audits everything else, and the rules over the missing section report that they
// could not be evaluated instead of silently passing.
func (a *Admin) FetchRealm(ctx context.Context, name string) (*Realm, error) {
	base := "/admin/realms/" + url.PathEscape(name)
	var realm Realm
	if err := a.get(ctx, base, &realm); err != nil {
		return nil, err
	}
	if realm.Realm == "" {
		realm.Realm = name
	}
	realm.Origin = "api:" + a.opts.BaseURL

	if err := a.getOptional(ctx, &realm, "clients", base+"/clients", &realm.Clients); err != nil {
		return nil, err
	}
	if err := a.getOptional(ctx, &realm, "identityProviders", base+"/identity-provider/instances", &realm.IdentityProviders); err != nil {
		return nil, err
	}
	var components []Component
	if err := a.getOptional(ctx, &realm, "components", base+"/components", &components); err != nil {
		return nil, err
	}
	if len(components) > 0 {
		realm.ComponentsByType = map[string][]Component{}
		for _, c := range components {
			typ := c.ProviderType
			if typ == "" {
				typ = "unknown"
			}
			realm.ComponentsByType[typ] = append(realm.ComponentsByType[typ], c)
		}
	}
	if err := a.fetchFlows(ctx, &realm, base); err != nil {
		return nil, err
	}
	realm.Scrub()
	return &realm, nil
}

// fetchFlows reads the top-level flows and, for each, its executions. The
// executions endpoint returns an already-flat list (subflows included), which is
// exactly what the flow rules walk.
func (a *Admin) fetchFlows(ctx context.Context, realm *Realm, base string) error {
	var flows []AuthFlow
	if err := a.getOptional(ctx, realm, "authenticationFlows", base+"/authentication/flows", &flows); err != nil {
		return err
	}
	for i := range flows {
		if flows[i].Alias == "" {
			continue
		}
		var execs []AuthExecution
		endpoint := base + "/authentication/flows/" + url.PathEscape(flows[i].Alias) + "/executions"
		if err := a.get(ctx, endpoint, &execs); err != nil {
			// One unreadable flow must not fail the realm: the flow rules see it
			// as a flow with no executions, and the section-level Missing note
			// (set when the flow list itself is unreadable) covers the blind case.
			continue
		}
		flows[i].Executions = execs
	}
	realm.AuthenticationFlows = flows
	return nil
}

// getOptional fetches a section, recording a 403 in realm.Missing instead of
// failing.
func (a *Admin) getOptional(ctx context.Context, realm *Realm, section, path string, out any) error {
	err := a.get(ctx, path, out)
	if err == nil {
		return nil
	}
	var he *httpError
	if errors.As(err, &he) && (he.status == http.StatusForbidden || he.status == http.StatusUnauthorized) {
		realm.markMissing(section, fmt.Sprintf("HTTP %d from %s", he.status, path))
		return nil
	}
	return err
}

func (a *Admin) get(ctx context.Context, path string, out any) error {
	if a.token == "" {
		if err := a.Login(ctx); err != nil {
			return err
		}
	}
	endpoint := a.opts.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &httpError{status: resp.StatusCode, path: path}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out); err != nil {
		return fmt.Errorf("GET %s: response is not the expected JSON: %w", path, err)
	}
	return nil
}

type httpError struct {
	status int
	path   string
}

func (e *httpError) Error() string { return fmt.Sprintf("GET %s: HTTP %d", e.path, e.status) }

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
