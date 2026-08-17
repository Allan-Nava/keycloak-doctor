package rules

import (
	"fmt"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
)

// Thresholds for the realm-level rules. They are named (and documented) rather
// than inlined because they are the opinion of this tool, and the first thing an
// operator will want to argue with.
const (
	// An access token is a bearer credential: the longer it lives, the longer a
	// stolen one works. 5 minutes is Keycloak's default; past 15 the token starts
	// outliving the incident response that would revoke the session.
	warnAccessTokenLifespan = 15 * 60
	badAccessTokenLifespan  = 60 * 60
	// An SSO session that idles for more than a working day means a shared browser
	// stays logged in overnight.
	warnSSOIdleTimeout = 8 * 3600
	warnSSOMaxLifespan = 7 * 24 * 3600
	// Brute force detection that only reacts after this many failures is closer to
	// off than on for a password-spraying run.
	warnFailureFactor = 30
	// Length is the only password rule that reliably survives contact with users.
	minPasswordLength = 12
)

func realmRules() []Rule {
	return []Rule{
		{
			ID:        "realm/enabled",
			Title:     "The audited realm is actually in use",
			Rationale: "A disabled realm answers no login, so its findings are not incidents — but a realm left disabled next to a live one is usually a forgotten migration, and it still holds the users, clients and secrets that become exposure the day someone re-enables it.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/enabled"
				if r.IsEnabled() {
					return []engine.Finding{pass(id, r, "the realm is enabled")}
				}
				return []engine.Finding{warn(id, r, "",
					"the realm is disabled: it serves no login, but still holds its users, clients and secrets",
					"delete the realm if it is a leftover, or document why it is kept")}
			},
		},
		{
			ID:        "realm/ssl-required",
			Title:     "HTTPS is required for every request to the realm",
			Rationale: "With sslRequired=external Keycloak still serves and accepts plaintext HTTP from private addresses, so tokens and codes travel in the clear over any internal hop — a reverse proxy, a service mesh sidecar, a developer's tunnel. With none it accepts plaintext from anywhere.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				switch strings.ToLower(strings.TrimSpace(r.SSLRequired)) {
				case "":
					return nil // the source did not carry the field
				case "all":
					return []engine.Finding{pass("realm/ssl-required", r, "HTTPS is required for all requests (sslRequired=all)")}
				case "none":
					return []engine.Finding{bad("realm/ssl-required", r, "",
						"the realm accepts plaintext HTTP from any address (sslRequired=none)",
						"set Require SSL to 'all' in Realm settings › General")}
				default:
					return []engine.Finding{warn("realm/ssl-required", r, "",
						fmt.Sprintf("plaintext HTTP is still accepted from private addresses (sslRequired=%s)", r.SSLRequired),
						"set Require SSL to 'all' once every hop in front of Keycloak terminates TLS")}
				}
			},
		},
		{
			ID:        "realm/brute-force",
			Title:     "Brute force detection is enabled and reacts early",
			Rationale: "Without brute force detection a Keycloak realm answers password guesses as fast as it can serve them, and the only trace is in the login events nobody reads.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/brute-force"
				if !r.BruteForceProtected {
					return []engine.Finding{bad(id, r, "",
						"brute force detection is disabled: password guessing is unthrottled",
						"enable Brute force detection in Realm settings › Security defenses")}
				}
				var out []engine.Finding
				if r.FailureFactor > warnFailureFactor {
					out = append(out, warn(id, r, "",
						fmt.Sprintf("brute force detection only trips after %d failures", r.FailureFactor),
						fmt.Sprintf("lower the failure factor to %d or less", warnFailureFactor)))
				}
				if r.PermanentLockout {
					out = append(out, warn(id, r, "",
						"permanent lockout is on: a spraying run locks real accounts out until an admin unlocks them",
						"prefer temporary lockout with an increasing wait, unless the lockout is monitored"))
				}
				if len(out) == 0 {
					out = append(out, pass(id, r, fmt.Sprintf("brute force detection is enabled (trips after %d failures)", r.FailureFactor)))
				}
				return out
			},
		},
		{
			ID:        "realm/token-lifespan",
			Title:     "Access tokens are short-lived",
			Rationale: "Keycloak cannot revoke an issued access token: it is valid until it expires, whatever happens to the session behind it. The lifespan is the window an attacker keeps a stolen token.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/token-lifespan"
				n := r.AccessTokenLifespan
				if n <= 0 {
					return nil // absent in the source: nothing measured, nothing claimed
				}
				switch {
				case n > badAccessTokenLifespan:
					return []engine.Finding{bad(id, r, "",
						fmt.Sprintf("access tokens live for %s and cannot be revoked before that", humanSeconds(n)),
						"bring the access token lifespan down to 5–15 minutes and let clients refresh")}
				case n > warnAccessTokenLifespan:
					return []engine.Finding{warn(id, r, "",
						fmt.Sprintf("access tokens live for %s", humanSeconds(n)),
						"5–15 minutes is the usual range; refresh tokens carry the long-lived part")}
				default:
					return []engine.Finding{pass(id, r, fmt.Sprintf("access tokens live for %s", humanSeconds(n)))}
				}
			},
		},
		{
			ID:        "realm/session-lifespan",
			Title:     "SSO sessions expire in a working day, not a season",
			Rationale: "The SSO session is what silently re-issues tokens: a session that idles for weeks turns one browser compromise into standing access.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/session-lifespan"
				var out []engine.Finding
				if r.SSOSessionIdleTimeout > warnSSOIdleTimeout {
					out = append(out, warn(id, r, "",
						fmt.Sprintf("an idle SSO session survives %s", humanSeconds(r.SSOSessionIdleTimeout)),
						"an idle timeout inside a working day (30m–8h) limits shared-browser exposure"))
				}
				if r.SSOSessionMaxLifespan > warnSSOMaxLifespan {
					out = append(out, warn(id, r, "",
						fmt.Sprintf("an SSO session can live %s before re-authentication", humanSeconds(r.SSOSessionMaxLifespan)),
						"cap the max session lifespan so every user re-authenticates periodically"))
				}
				if len(out) == 0 && (r.SSOSessionIdleTimeout > 0 || r.SSOSessionMaxLifespan > 0) {
					out = append(out, pass(id, r, fmt.Sprintf("SSO sessions idle out after %s, max %s",
						humanSeconds(r.SSOSessionIdleTimeout), humanSeconds(r.SSOSessionMaxLifespan))))
				}
				return out
			},
		},
		{
			ID:        "realm/offline-session-expiry",
			Title:     "Offline tokens expire",
			Rationale: "An offline token with no max lifespan never expires — it is a permanent credential handed to whoever asked for offline_access, and it survives every password change.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/offline-session-expiry"
				if !r.OfflineSessionMaxLifespanEnabled {
					return []engine.Finding{warn(id, r, "",
						"offline sessions have no maximum lifespan: an offline token never expires",
						"enable Offline Session Max Limited in Realm settings › Sessions")}
				}
				return []engine.Finding{pass(id, r, fmt.Sprintf("offline sessions expire after %s", humanSeconds(r.OfflineSessionMaxLifespan)))}
			},
		},
		{
			ID:        "realm/refresh-token-rotation",
			Title:     "Refresh tokens are rotated and single-use",
			Rationale: "Without rotation a leaked refresh token is a renewable credential nobody can distinguish from the real client; with rotation, a replay invalidates the chain and shows up.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/refresh-token-rotation"
				if !r.RevokeRefreshToken {
					return []engine.Finding{warn(id, r, "",
						"refresh tokens are not rotated (revokeRefreshToken=false): a leaked one stays usable for the whole session",
						"enable Revoke Refresh Token in Realm settings › Sessions")}
				}
				if r.RefreshTokenMaxReuse > 0 {
					return []engine.Finding{warn(id, r, "",
						fmt.Sprintf("a rotated refresh token may still be reused %d times", r.RefreshTokenMaxReuse),
						"set Refresh Token Max Reuse to 0 so replay is always detected")}
				}
				return []engine.Finding{pass(id, r, "refresh tokens are rotated and single-use")}
			},
		},
		{
			ID:        "realm/password-policy",
			Title:     "A password policy is set, with a modern hash",
			Rationale: "A realm with no password policy accepts a one-character password. The hash settings matter just as much: an offline crack of a weak PBKDF2 iteration count is arithmetic, not research.",
			Eval:      evalPasswordPolicy,
		},
		{
			ID:        "realm/self-registration",
			Title:     "Self-registration does not create unverified identities",
			Rationale: "Open registration without email verification lets anyone create an account with someone else's address — which is also the account an identity broker will happily link to later.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/self-registration"
				if !r.RegistrationAllowed {
					return []engine.Finding{pass(id, r, "self-registration is disabled")}
				}
				if !r.VerifyEmail {
					return []engine.Finding{bad(id, r, "",
						"self-registration is open and email verification is off: anyone can register any address",
						"enable Verify email in Realm settings › Login, or close self-registration")}
				}
				return []engine.Finding{warn(id, r, "",
					"self-registration is open (with email verification on)",
					"confirm that anyone on the network is meant to be able to create an account here")}
			},
		},
		{
			ID:        "realm/email-identity",
			Title:     "An email address identifies at most one account",
			Rationale: "Duplicate emails plus login-with-email is an ambiguous identity: password reset and broker account linking then act on whichever account they happen to find first.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/email-identity"
				var out []engine.Finding
				if r.DuplicateEmailsAllowed && r.LoginWithEmailAllowed {
					out = append(out, bad(id, r, "",
						"duplicate emails are allowed while login with email is on: an address can resolve to more than one account",
						"disable Duplicate emails in Realm settings › Login"))
				}
				if r.EditUsernameAllowed {
					out = append(out, warn(id, r, "",
						"users can change their own username: anything keyed on the username instead of the user id can be re-pointed",
						"keep Edit username off unless downstream systems key on the immutable user id"))
				}
				if len(out) == 0 {
					out = append(out, pass(id, r, "an email address resolves to a single account and usernames are immutable"))
				}
				return out
			},
		},
		{
			ID:        "realm/otp-policy",
			Title:     "The OTP policy is strong enough to be worth having",
			Rationale: "A 4-digit OTP is 10 000 guesses; with brute force detection tuned for passwords, that is a plausible online attack.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/otp-policy"
				if r.OTPPolicyDigits <= 0 {
					return nil
				}
				if r.OTPPolicyDigits < 6 {
					return []engine.Finding{warn(id, r, "",
						fmt.Sprintf("the OTP policy uses %d digits", r.OTPPolicyDigits),
						"use 6 digits or more (Authentication › Policies › OTP Policy)")}
				}
				return []engine.Finding{pass(id, r, fmt.Sprintf("the OTP policy uses %d digits (%s)", r.OTPPolicyDigits, orUnknown(r.OTPPolicyType)))}
			},
		},
		{
			ID:        "realm/audit-events",
			Title:     "Login and admin events are recorded",
			Rationale: "Events are the only record of who logged in and who changed the realm. Turned off, an incident has no timeline and a malicious admin change leaves no trace at all.",
			Eval: func(r *keycloak.Realm) []engine.Finding {
				const id = "realm/audit-events"
				var out []engine.Finding
				if !r.EventsEnabled {
					out = append(out, warn(id, r, "",
						"login events are not recorded",
						"enable user event logging in Realm settings › Sessions › Events, and ship the events off-box"))
				}
				if !r.AdminEventsEnabled {
					out = append(out, warn(id, r, "",
						"admin events are not recorded: changes to the realm leave no audit trail",
						"enable admin event logging in Realm settings › Events"))
				}
				if len(out) == 0 {
					out = append(out, pass(id, r, "login and admin events are recorded"))
				}
				return out
			},
		},
		{
			ID:        "realm/browser-mfa",
			Title:     "The bound browser flow can ask for a second factor",
			Rationale: "A browser flow with no OTP or WebAuthn step means every account in the realm is exactly one password away, however strong the OTP policy is.",
			Eval:      evalBrowserMFA,
		},
	}
}

func evalPasswordPolicy(r *keycloak.Realm) []engine.Finding {
	const id = "realm/password-policy"
	policy := parsePasswordPolicy(r.PasswordPolicy)
	if len(policy) == 0 {
		return []engine.Finding{bad(id, r, "",
			"no password policy is set: the realm accepts any password",
			"add at least length(12) and notUsername in Authentication › Policies › Password policy")}
	}
	var out []engine.Finding
	if n, ok := policy.num("length"); !ok {
		out = append(out, warn(id, r, "",
			"the password policy sets no minimum length",
			fmt.Sprintf("add length(%d) to the password policy", minPasswordLength)))
	} else if n < minPasswordLength {
		out = append(out, warn(id, r, "",
			fmt.Sprintf("the minimum password length is %d", n),
			fmt.Sprintf("raise it to %d or more; length beats composition rules", minPasswordLength)))
	}
	if _, ok := policy["notUsername"]; !ok {
		out = append(out, warn(id, r, "",
			"the password policy allows the password to be the username",
			"add notUsername to the password policy"))
	}
	if alg, ok := policy["hashAlgorithm"]; ok {
		if floor, known := minHashIterations[strings.ToLower(alg)]; known {
			if n, ok := policy.num("hashIterations"); ok && n < floor {
				out = append(out, warn(id, r, "",
					fmt.Sprintf("%s is configured with %d iterations", alg, n),
					fmt.Sprintf("at least %d iterations are recommended for %s, or move to argon2", floor, alg)))
			}
		}
	}
	if len(out) == 0 {
		out = append(out, pass(id, r, fmt.Sprintf("the password policy is set (%s)", r.PasswordPolicy)))
	}
	return out
}

// minHashIterations is the OWASP-recommended floor per PBKDF2 variant. Argon2
// (Keycloak's default since 24) is not listed: its cost is not an iteration count
// and comparing the numbers would be meaningless.
var minHashIterations = map[string]int{
	"pbkdf2":        1300000,
	"pbkdf2-sha1":   1300000,
	"pbkdf2-sha256": 600000,
	"pbkdf2-sha512": 210000,
}

func evalBrowserMFA(r *keycloak.Realm) []engine.Finding {
	const id = "realm/browser-mfa"
	if reason := r.Unavailable("authenticationFlows"); reason != "" {
		return []engine.Finding{unevaluated(id, r, "authenticationFlows")}
	}
	alias := r.BrowserFlow
	if alias == "" {
		alias = "browser"
	}
	authenticators, found := reachableAuthenticators(r, alias)
	if !found {
		return nil // the source carried no flow by that name: nothing measured
	}
	for _, a := range authenticators {
		if isSecondFactor(a) {
			return []engine.Finding{pass(id, r, fmt.Sprintf("the %q flow can ask for a second factor (%s)", alias, a))}
		}
	}
	return []engine.Finding{warn(id, r, "",
		fmt.Sprintf("the bound browser flow %q has no OTP or WebAuthn step: a password is the only factor", alias),
		"add a Conditional OTP or WebAuthn subflow to the browser flow")}
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown type"
	}
	return s
}
