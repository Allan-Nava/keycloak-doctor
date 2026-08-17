// backlog validates BACKLOG.md and mirrors it into GitHub milestones and issues.
//
// BACKLOG.md is the single source of truth for the work on keycloak-doctor; the
// tracker is a projection of it, the same way docs/rules.md is a projection of
// the rule catalogue. This tool is what makes that true instead of aspirational:
//
//	go run ./cmd/backlog check                    # validate the file (CI gate)
//	go run ./cmd/backlog export                   # the parsed file as JSON
//	go run ./cmd/backlog sync --dry-run           # what a sync would change
//	go run ./cmd/backlog sync --repo owner/name   # reconcile the tracker
//
// Exit codes: 0 when the file is valid and the sync succeeded, 1 on a validation
// error or a failed API call, 2 on a usage error.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/backlog"
)

const defaultFile = "BACKLOG.md"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "check":
		err = runCheck(os.Args[2:])
	case "export":
		err = runExport(os.Args[2:])
	case "sync":
		err = runSync(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "backlog: unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		var usageErr *usageError
		if errors.As(err, &usageErr) {
			fmt.Fprintln(os.Stderr, "backlog:", err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "backlog:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `backlog — validate BACKLOG.md and mirror it into GitHub issues

usage:
  backlog check  [--file BACKLOG.md]
  backlog export [--file BACKLOG.md]
  backlog sync   [--file BACKLOG.md] [--repo owner/name] [--token-env GITHUB_TOKEN] [--dry-run]

subcommands:
  check   parse BACKLOG.md and report duplicate ids, unknown ids in a milestone
          and items claimed twice. Non-zero exit on any error-level problem.
  export  print the parsed backlog as JSON (items, milestones, assignments).
  sync    create the milestones the file declares, open an issue for every open
          item, keep title, body, milestone and labels in step, and close the
          issue of an item that moved to Done. Idempotent, and it never deletes.

flags:
  --file PATH        backlog file to read (default BACKLOG.md)
  --repo owner/name  target repository (default $GITHUB_REPOSITORY)
  --token-env NAME   env var holding the API token (default GITHUB_TOKEN)
  --api URL          GitHub API base URL (default https://api.github.com)
  --dry-run          report the actions without touching the repository
`)
}

type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func runCheck(argv []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	file := fs.String("file", defaultFile, "backlog file to read")
	if err := fs.Parse(argv); err != nil {
		return &usageError{err.Error()}
	}

	doc, parseErr := load(*file)
	problems := doc.Check()
	report(*file, problems)

	if parseErr != nil {
		return parseErr
	}
	if n := backlog.Errors(problems); n > 0 {
		return fmt.Errorf("%s: %d problem(s) to fix", *file, n)
	}
	fmt.Printf("%s is valid: %d items (%d open), %d milestones\n", *file, len(doc.Items), len(doc.Open()), len(doc.Milestones))
	return nil
}

func runExport(argv []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	file := fs.String("file", defaultFile, "backlog file to read")
	if err := fs.Parse(argv); err != nil {
		return &usageError{err.Error()}
	}

	doc, err := load(*file)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func runSync(argv []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	file := fs.String("file", defaultFile, "backlog file to read")
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "target repository, owner/name")
	tokenEnv := fs.String("token-env", "GITHUB_TOKEN", "env var holding the API token")
	api := fs.String("api", backlog.DefaultAPI, "GitHub API base URL")
	dryRun := fs.Bool("dry-run", false, "report the actions without touching the repository")
	if err := fs.Parse(argv); err != nil {
		return &usageError{err.Error()}
	}

	doc, err := load(*file)
	if err != nil {
		return err
	}
	problems := doc.Check()
	report(*file, problems)
	// A broken file must never reach the tracker: a duplicate id or an unknown id
	// in a milestone would mirror as the wrong issue, and an issue is visible work.
	if n := backlog.Errors(problems); n > 0 {
		return fmt.Errorf("%s: %d problem(s) to fix before syncing", *file, n)
	}

	token := os.Getenv(*tokenEnv)
	if token == "" && !*dryRun {
		return fmt.Errorf("no token: set %s (the value is read from the environment, never passed as a flag)", *tokenEnv)
	}

	gh := backlog.NewGitHub(*repo, token)
	gh.BaseURL = strings.TrimRight(*api, "/")
	gh.DryRun = *dryRun

	actions, err := gh.Sync(context.Background(), doc)
	printActions(actions, *dryRun)
	return err
}

func load(path string) (backlog.Doc, error) {
	src, err := os.ReadFile(path) //nolint:gosec // the path is the operator's own backlog file
	if err != nil {
		return backlog.Doc{}, err
	}
	return backlog.Parse(src)
}

// report prints the problems, as GitHub Actions annotations when running in a
// workflow so they land on the file in the pull request diff.
func report(file string, problems []backlog.Problem) {
	inActions := os.Getenv("GITHUB_ACTIONS") == "true"
	for _, p := range problems {
		switch {
		case inActions:
			level := "error"
			if p.Level == "warn" {
				level = "warning"
			}
			if p.Line > 0 {
				fmt.Printf("::%s file=%s,line=%d::%s\n", level, file, p.Line, p.Message)
			} else {
				fmt.Printf("::%s file=%s::%s\n", level, file, p.Message)
			}
		case p.Line > 0:
			fmt.Fprintf(os.Stderr, "%s: %s:%d: %s\n", strings.ToUpper(p.Level), file, p.Line, p.Message)
		default:
			fmt.Fprintf(os.Stderr, "%s: %s: %s\n", strings.ToUpper(p.Level), file, p.Message)
		}
	}
}

// printActions writes the run as markdown, so the workflow can pipe it straight
// into the job summary.
func printActions(actions []backlog.Action, dryRun bool) {
	heading := "### Backlog sync"
	if dryRun {
		heading += " (dry run — nothing was changed)"
	}
	fmt.Println(heading)
	fmt.Println()
	if len(actions) == 0 {
		fmt.Println("- nothing to do: the tracker already matches `BACKLOG.md`")
		return
	}
	for _, a := range actions {
		fmt.Println("- " + a.String())
	}
}
