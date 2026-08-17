package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/output"
	"github.com/Allan-Nava/keycloak-doctor/internal/rules"
)

// runRules prints the rule catalogue. It is the documentation of what the tool
// knows, and it lives in the binary so it can never drift from the rules that
// actually run.
func runRules(args []string) error {
	fs := flag.NewFlagSet("rules", flag.ContinueOnError)
	fs.Usage = usage
	only := fs.String("only", "", "list only these rules or categories (comma-separated)")
	format := fs.String("output", "text", "output format: "+strings.Join(output.Formats(), "|"))
	if err := fs.Parse(args); err != nil {
		return err
	}
	selected, err := rules.Select(splitList(*only), nil)
	if err != nil {
		return err
	}
	switch output.Format(*format) {
	case output.JSON:
		type jsonRule struct {
			ID        string `json:"id"`
			Category  string `json:"category"`
			Title     string `json:"title"`
			Rationale string `json:"rationale"`
		}
		out := make([]jsonRule, 0, len(selected))
		for _, r := range selected {
			out = append(out, jsonRule{ID: r.ID, Category: r.Category(), Title: r.Title, Rationale: r.Rationale})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	case output.Markdown:
		fmt.Printf("# keycloak-doctor rules (%d)\n\n", len(selected))
		for _, r := range selected {
			fmt.Printf("## `%s`\n\n**%s**\n\n%s\n\n", r.ID, r.Title, r.Rationale)
		}
		return nil
	case output.Text:
		fmt.Printf("%d rule(s) in %d categories: %s\n", len(selected), len(rules.Categories()), strings.Join(rules.Categories(), ", "))
		for _, r := range selected {
			fmt.Printf("\n%s\n  %s\n  %s\n", r.ID, r.Title, wrap(r.Rationale, 92, "  "))
		}
		return nil
	default:
		return fmt.Errorf("unknown output format %q (want %s)", *format, strings.Join(output.Formats(), ", "))
	}
}

// wrap reflows text to width, indenting continuation lines.
func wrap(text string, width int, indent string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	line := 0
	for i, w := range words {
		if i > 0 {
			if line+1+len(w) > width {
				b.WriteString("\n" + indent)
				line = 0
			} else {
				b.WriteString(" ")
				line++
			}
		}
		b.WriteString(w)
		line += len(w)
	}
	return b.String()
}
