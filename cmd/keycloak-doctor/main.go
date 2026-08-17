// keycloak-doctor audits the configuration of a Keycloak realm.
//
//	keycloak-doctor audit realm-export.json
//	keycloak-doctor audit --url https://sso.example.com --realm prod \
//	    --client-id audit --client-secret-env KC_AUDIT_SECRET
//	keycloak-doctor rules
//
// Exit code: 0 even when the audit reports WARN or BAD findings — an audit that
// ran IS a success, and the report is the deliverable. Pass --exit-on to gate a
// pipeline. Non-zero otherwise only for systemic errors: an unreadable source,
// credentials that do not work, an unknown rule or a bad flag.
package main

import (
	"errors"
	"fmt"
	"os"
)

var version = "dev" // injected at build time via -ldflags "-X main.version=..."

// exitError asks main to exit with a specific code without printing anything: it
// is how --exit-on gates a pipeline, as opposed to a real failure.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(64)
	}
	var err error
	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println("keycloak-doctor", version)
		return
	case "audit":
		err = runAudit(os.Args[2:])
	case "rules":
		err = runRules(os.Args[2:])
	case "help", "--help", "-h":
		usage()
		return
	default:
		usage()
		os.Exit(64)
	}
	if err == nil {
		return
	}
	var ee *exitError
	if errors.As(err, &ee) {
		os.Exit(ee.code)
	}
	fmt.Fprintln(os.Stderr, "keycloak-doctor:", err)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  keycloak-doctor audit <realm-export.json|export-dir> [--realm NAME,...] [--output text|markdown|json]
                        [--out-file PATH] [--only RULE|CATEGORY,...] [--skip RULE|CATEGORY,...]
                        [--min-severity ok|warn|bad|error] [--no-color] [--exit-on warn|bad|error] [--exit-code N]
  keycloak-doctor audit --url https://sso.example.com [--realm NAME,... | --all-realms]
                        --client-id ID --client-secret-env VAR [--auth-realm master]
                        [--username USER --password-env VAR] [--insecure] [--timeout 15s] [...same output flags]
  keycloak-doctor rules [--only RULE|CATEGORY,...] [--output text|markdown|json]   # what every rule checks, and why
  keycloak-doctor version

Credentials are read from the environment, never from a flag value: --client-secret-env
and --password-env name the variable that holds them.`)
}
