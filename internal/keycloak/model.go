// Package keycloak models the parts of a Keycloak realm an audit needs, and
// loads them either from a realm export file (offline, no credentials) or from
// the Admin REST API of a running server.
//
// The model is deliberately partial: it carries the fields the rules in
// internal/rules reason about, with the JSON names of Keycloak's own
// RealmRepresentation, so both sources decode into the same shape.
//
// Credential hygiene: a realm export carries plaintext client secrets, LDAP bind
// credentials and identity-provider client secrets. Scrub, which every loader
// calls, records only the *presence* of each and drops the value, so nothing
// downstream — findings, JSON output, an error message — can leak one.
package keycloak

import "strings"

// Realm is a Keycloak realm as far as the audit is concerned.
type Realm struct {
	Realm       string `json:"realm"`
	DisplayName string `json:"displayName"`
	Enabled     *bool  `json:"enabled"`

	// Transport and login policy.
	SSLRequired            string `json:"sslRequired"`
	RegistrationAllowed    bool   `json:"registrationAllowed"`
	ResetPasswordAllowed   bool   `json:"resetPasswordAllowed"`
	VerifyEmail            bool   `json:"verifyEmail"`
	LoginWithEmailAllowed  bool   `json:"loginWithEmailAllowed"`
	DuplicateEmailsAllowed bool   `json:"duplicateEmailsAllowed"`
	EditUsernameAllowed    bool   `json:"editUsernameAllowed"`

	// Brute force detection.
	BruteForceProtected bool `json:"bruteForceProtected"`
	PermanentLockout    bool `json:"permanentLockout"`
	FailureFactor       int  `json:"failureFactor"`

	// Tokens and sessions, all in seconds.
	AccessTokenLifespan              int  `json:"accessTokenLifespan"`
	SSOSessionIdleTimeout            int  `json:"ssoSessionIdleTimeout"`
	SSOSessionMaxLifespan            int  `json:"ssoSessionMaxLifespan"`
	OfflineSessionMaxLifespanEnabled bool `json:"offlineSessionMaxLifespanEnabled"`
	OfflineSessionMaxLifespan        int  `json:"offlineSessionMaxLifespan"`
	RevokeRefreshToken               bool `json:"revokeRefreshToken"`
	RefreshTokenMaxReuse             int  `json:"refreshTokenMaxReuse"`

	// Credentials policy.
	PasswordPolicy     string `json:"passwordPolicy"`
	OTPPolicyType      string `json:"otpPolicyType"`
	OTPPolicyAlgorithm string `json:"otpPolicyAlgorithm"`
	OTPPolicyDigits    int    `json:"otpPolicyDigits"`

	// Audit trail.
	EventsEnabled      bool `json:"eventsEnabled"`
	EventsExpiration   int  `json:"eventsExpiration"`
	AdminEventsEnabled bool `json:"adminEventsEnabled"`

	// Bound flows.
	BrowserFlow     string `json:"browserFlow"`
	DirectGrantFlow string `json:"directGrantFlow"`

	Clients             []Client               `json:"clients"`
	IdentityProviders   []IdentityProvider     `json:"identityProviders"`
	AuthenticationFlows []AuthFlow             `json:"authenticationFlows"`
	ComponentsByType    map[string][]Component `json:"components"`
	Attributes          Attrs                  `json:"attributes"`

	// Origin describes where this realm was read from ("file:…", "api:…"). It is
	// set by the loader and used by source-hygiene rules and the output header.
	Origin string `json:"-"`

	// Missing records the sections the source could not provide, keyed by section
	// name ("clients", "components", …) with the reason as value. The Admin API
	// loader sets it when the credentials may not read a section; rules over a
	// missing section report ERROR ("could not evaluate") rather than passing
	// silently, which is the difference between a blind spot and a clean bill.
	Missing map[string]string `json:"-"`
}

// markMissing records that a section of the realm could not be read.
func (r *Realm) markMissing(section, reason string) {
	if r.Missing == nil {
		r.Missing = map[string]string{}
	}
	r.Missing[section] = reason
}

// Unavailable returns the reason a section is missing, or "" when it was read.
func (r *Realm) Unavailable(section string) string { return r.Missing[section] }

// IsEnabled reports whether the realm is enabled. An absent field means enabled:
// the audit must not report a realm as disabled just because the source omitted
// the flag.
func (r *Realm) IsEnabled() bool { return r.Enabled == nil || *r.Enabled }

// Name returns the realm name, or "(unnamed)" when the source has none.
func (r *Realm) Name() string {
	if strings.TrimSpace(r.Realm) == "" {
		return "(unnamed)"
	}
	return r.Realm
}

// Components flattens ComponentsByType, filling ProviderType from the map key
// when the entry does not carry it. A realm export keys components by provider
// type; the Admin API returns them flat with providerType set — this is the one
// shape the rules see.
func (r *Realm) Components() []Component {
	var out []Component
	for typ, comps := range r.ComponentsByType {
		for _, c := range comps {
			if c.ProviderType == "" {
				c.ProviderType = typ
			}
			out = append(out, c)
		}
	}
	return out
}

// Client is an OIDC/SAML client of the realm.
type Client struct {
	ClientID                     string           `json:"clientId"`
	Name                         string           `json:"name"`
	Description                  string           `json:"description"`
	Enabled                      *bool            `json:"enabled"`
	Protocol                     string           `json:"protocol"`
	PublicClient                 bool             `json:"publicClient"`
	BearerOnly                   bool             `json:"bearerOnly"`
	StandardFlowEnabled          bool             `json:"standardFlowEnabled"`
	ImplicitFlowEnabled          bool             `json:"implicitFlowEnabled"`
	DirectAccessGrantsEnabled    bool             `json:"directAccessGrantsEnabled"`
	ServiceAccountsEnabled       bool             `json:"serviceAccountsEnabled"`
	AuthorizationServicesEnabled bool             `json:"authorizationServicesEnabled"`
	FullScopeAllowed             bool             `json:"fullScopeAllowed"`
	ConsentRequired              bool             `json:"consentRequired"`
	RedirectURIs                 []string         `json:"redirectUris"`
	WebOrigins                   []string         `json:"webOrigins"`
	RootURL                      string           `json:"rootUrl"`
	BaseURL                      string           `json:"baseUrl"`
	Attributes                   Attrs            `json:"attributes"`
	ProtocolMappers              []ProtocolMapper `json:"protocolMappers"`

	// Secret is cleared by Scrub; SecretSet records that the source carried one.
	Secret    string `json:"secret"`
	SecretSet bool   `json:"-"`
}

// IsEnabled reports whether the client is enabled (absent means enabled).
func (c *Client) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

// IsOIDC reports whether the client speaks OpenID Connect. An empty protocol is
// OIDC: that is Keycloak's default for clients created without one.
func (c *Client) IsOIDC() bool {
	return c.Protocol == "" || strings.EqualFold(c.Protocol, "openid-connect")
}

// ProtocolMapper maps user data into a token or assertion.
type ProtocolMapper struct {
	Name           string `json:"name"`
	Protocol       string `json:"protocol"`
	ProtocolMapper string `json:"protocolMapper"`
	Config         Attrs  `json:"config"`
}

// IdentityProvider is a configured broker (social or OIDC/SAML).
type IdentityProvider struct {
	Alias                     string `json:"alias"`
	ProviderID                string `json:"providerId"`
	Enabled                   *bool  `json:"enabled"`
	TrustEmail                bool   `json:"trustEmail"`
	LinkOnly                  bool   `json:"linkOnly"`
	FirstBrokerLoginFlowAlias string `json:"firstBrokerLoginFlowAlias"`
	Config                    Attrs  `json:"config"`

	// SecretSet records that the source carried a client secret for this broker;
	// the value itself is dropped by Scrub.
	SecretSet bool `json:"-"`
}

// IsEnabled reports whether the broker is enabled (absent means enabled).
func (i *IdentityProvider) IsEnabled() bool { return i.Enabled == nil || *i.Enabled }

// AuthFlow is an authentication flow with its executions.
type AuthFlow struct {
	Alias       string          `json:"alias"`
	Description string          `json:"description"`
	ProviderID  string          `json:"providerId"`
	TopLevel    *bool           `json:"topLevel"`
	BuiltIn     bool            `json:"builtIn"`
	Executions  []AuthExecution `json:"authenticationExecutions"`
}

// AuthExecution is one step of a flow.
//
// The two sources spell this differently: a realm export names the authenticator
// in "authenticator" and nests subflows through "flowAlias", while the Admin API
// executions endpoint returns an already-flat list naming it "providerId". Both
// are decoded here and Provider normalises them.
type AuthExecution struct {
	Authenticator     string `json:"authenticator"`
	ProviderID        string `json:"providerId"`
	DisplayName       string `json:"displayName"`
	Requirement       string `json:"requirement"`
	FlowAlias         string `json:"flowAlias"`
	AuthenticatorFlow bool   `json:"authenticatorFlow"`
}

// Provider returns the authenticator id of the execution, whichever field the
// source used.
func (e AuthExecution) Provider() string {
	if e.Authenticator != "" {
		return e.Authenticator
	}
	return e.ProviderID
}

// IsDisabled reports whether the execution is switched off, in which case it
// contributes nothing to the flow.
func (e AuthExecution) IsDisabled() bool {
	return strings.EqualFold(e.Requirement, "DISABLED")
}

// Component is a realm component: key providers, user federation, and so on.
type Component struct {
	Name         string              `json:"name"`
	ProviderID   string              `json:"providerId"`
	ProviderType string              `json:"providerType"`
	Config       map[string][]string `json:"config"`

	// SecretKeys lists the config keys whose values Scrub dropped because they
	// are credential material (bindCredential, clientSecret, …).
	SecretKeys []string `json:"-"`
}

// Cfg returns the first value of a component config key, or "" when absent.
// Component config is string→list even for single-valued settings.
func (c *Component) Cfg(key string) string {
	if v := c.Config[key]; len(v) > 0 {
		return strings.TrimSpace(v[0])
	}
	return ""
}

// CfgBool reports whether a component config key holds "true".
func (c *Component) CfgBool(key string) bool {
	return strings.EqualFold(c.Cfg(key), "true")
}

// Label names the component for a finding: its name, falling back to the
// provider id.
func (c *Component) Label() string {
	if strings.TrimSpace(c.Name) != "" {
		return c.Name
	}
	return c.ProviderID
}

// secretConfigKeys are the component config keys that hold credential material.
var secretConfigKeys = []string{"bindCredential", "clientSecret", "password", "secret", "privateKey"}

// Scrub drops every credential value from the model, recording only that one was
// present. Every loader calls it before returning, so no code path downstream of
// the loader can print a secret that was in the source.
func (r *Realm) Scrub() {
	for i := range r.Clients {
		c := &r.Clients[i]
		if strings.TrimSpace(c.Secret) != "" {
			c.SecretSet = true
			c.Secret = ""
		}
	}
	for i := range r.IdentityProviders {
		idp := &r.IdentityProviders[i]
		if idp.Config.Has("clientSecret") {
			idp.SecretSet = true
			delete(idp.Config, "clientSecret")
		}
	}
	for typ, comps := range r.ComponentsByType {
		for i := range comps {
			c := &comps[i]
			for _, key := range secretConfigKeys {
				if v := c.Config[key]; len(v) > 0 && strings.TrimSpace(v[0]) != "" {
					c.SecretKeys = append(c.SecretKeys, key)
					delete(c.Config, key)
				}
			}
		}
		r.ComponentsByType[typ] = comps
	}
}
