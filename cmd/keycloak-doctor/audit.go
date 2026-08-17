package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
	"github.com/Allan-Nava/keycloak-doctor/internal/keycloak"
	"github.com/Allan-Nava/keycloak-doctor/internal/output"
	"github.com/Allan-Nava/keycloak-doctor/internal/rules"
)

func runAudit(args []string) error {
	// `audit realm-export.json --output json` is the shape people type, but the
	// stdlib flag package stops parsing at the first non-flag argument — so the
	// leading source path is pulled out before the flags are parsed.
	positional := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positional, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.Usage = usage
	var (
		file        = fs.String("file", "", "realm export file or directory to audit")
		serverURL   = fs.String("url", "", "Keycloak base URL to audit through the Admin REST API")
		realmNames  = fs.String("realm", "", "comma-separated realm names (default: every realm in the source)")
		allRealms   = fs.Bool("all-realms", false, "audit every realm the credentials can see")
		clientID    = fs.String("client-id", "", "service account client id for the Admin API")
		secretEnv   = fs.String("client-secret-env", "", "environment variable holding the client secret")
		username    = fs.String("username", "", "admin username (password grant)")
		passwordEnv = fs.String("password-env", "", "environment variable holding the admin password")
		authRealm   = fs.String("auth-realm", "master", "realm to authenticate against")
		insecure    = fs.Bool("insecure", false, "skip TLS verification of the Keycloak server")
		timeout     = fs.Duration("timeout", 15*time.Second, "per-request timeout for the Admin API")
		format      = fs.String("output", "text", "output format: "+strings.Join(output.Formats(), "|"))
		outFile     = fs.String("out-file", "", "write the output to this file instead of stdout")
		only        = fs.String("only", "", "run only these rules or categories (comma-separated)")
		skip        = fs.String("skip", "", "skip these rules or categories (comma-separated)")
		minSeverity = fs.String("min-severity", "", "report only findings at or above this status")
		noColor     = fs.Bool("no-color", false, "disable ANSI colours in the text output")
		exitOn      = fs.String("exit-on", "", "exit non-zero when a finding reaches this status: warn|bad|error")
		exitCode    = fs.Int("exit-code", 2, "exit code used by --exit-on")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if rest := fs.Args(); len(rest) > 0 {
		return fmt.Errorf("unexpected argument %q (the source path goes first: `audit %s --output json`)", rest[0], rest[0])
	}
	// `audit realm-export.json` is the short form of `audit --file realm-export.json`.
	if positional != "" {
		if *file != "" || *serverURL != "" {
			return fmt.Errorf("two sources given: %q and %q", positional, firstNonEmpty(*file, *serverURL))
		}
		*file = positional
	}

	threshold, err := parseStatus(*minSeverity)
	if err != nil {
		return err
	}
	gate, err := parseStatus(*exitOn)
	if err != nil {
		return err
	}
	selected, err := rules.Select(splitList(*only), splitList(*skip))
	if err != nil {
		return err
	}
	names := splitList(*realmNames)

	started := time.Now()
	var (
		realms []*keycloak.Realm
		source string
	)
	switch {
	case *file != "" && *serverURL != "":
		return errors.New("pass either --file or --url, not both")
	case *file != "":
		realms, err = keycloak.LoadFile(*file)
		if err != nil {
			return err
		}
		if realms, err = keycloak.SelectRealms(realms, names); err != nil {
			return err
		}
		source = "file:" + *file
	case *serverURL != "":
		if len(names) == 0 && !*allRealms {
			return errors.New("pass --realm NAME (repeatable, comma-separated) or --all-realms")
		}
		realms, err = fetchLive(*serverURL, names, liveCreds{
			clientID: *clientID, secretEnv: *secretEnv,
			username: *username, passwordEnv: *passwordEnv,
			authRealm: *authRealm, insecure: *insecure, timeout: *timeout,
		})
		if err != nil {
			return err
		}
		source = "api:" + *serverURL
	default:
		return errors.New("no source: pass a realm export path, or --url for a live server")
	}

	findings := engine.MinSeverity(rules.Audit(realms, selected), threshold)
	res := engine.Result{
		Findings: findings,
		Realms:   keycloak.RealmNames(realms),
		Rules:    len(selected),
		Source:   source,
		Started:  started,
		Duration: time.Since(started),
	}

	w := io.Writer(os.Stdout)
	color := !*noColor && os.Getenv("NO_COLOR") == ""
	if *outFile != "" {
		f, err := os.Create(*outFile)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		w = f
		color = false // a file is read later, by something that does not want escapes
	}
	if err := output.Render(w, output.Format(*format), res, output.Options{Color: color, Version: version}); err != nil {
		return err
	}
	// The gate is evaluated on the reported findings, so --min-severity and
	// --exit-on stay consistent with each other and with what the operator saw.
	if gate != "" && engine.AtLeast(engine.Worst(findings), gate) {
		return &exitError{code: *exitCode}
	}
	return nil
}

// liveCreds carries the Admin API credential flags. Secrets are named, not
// passed: the values are read from the environment here, so they never appear in
// a command line, a shell history or a process listing.
type liveCreds struct {
	clientID, secretEnv   string
	username, passwordEnv string
	authRealm             string
	insecure              bool
	timeout               time.Duration
}

func fetchLive(serverURL string, names []string, creds liveCreds) ([]*keycloak.Realm, error) {
	secret, err := fromEnv(creds.secretEnv)
	if err != nil {
		return nil, err
	}
	password, err := fromEnv(creds.passwordEnv)
	if err != nil {
		return nil, err
	}
	admin, err := keycloak.NewAdmin(keycloak.AdminOptions{
		BaseURL:      serverURL,
		AuthRealm:    creds.authRealm,
		ClientID:     creds.clientID,
		ClientSecret: secret,
		Username:     creds.username,
		Password:     password,
		Insecure:     creds.insecure,
		Timeout:      creds.timeout,
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), creds.timeout*10)
	defer cancel()
	return admin.FetchRealms(ctx, names)
}

// fromEnv reads a secret from the named environment variable. An empty name means
// "not provided"; a name whose variable is unset or empty is an error, because the
// alternative is a run that silently falls back to another grant.
func fromEnv(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", nil
	}
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("environment variable %s is unset or empty", name)
	}
	return v, nil
}

func parseStatus(s string) (engine.Status, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	st := engine.Status(strings.ToUpper(strings.TrimSpace(s)))
	if !engine.Known(st) {
		return "", fmt.Errorf("unknown status %q (want ok, warn, bad or error)", s)
	}
	return st, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
