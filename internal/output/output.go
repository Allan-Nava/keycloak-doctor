// Package output renders an audit result. Text is for a terminal, Markdown for a
// report or a pull request comment, JSON for whatever gates the pipeline.
//
// The renderers only ever see findings, never the realm model, so nothing that
// was in the source can reach the output except through a rule's own message.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
)

// Format is an output format name.
type Format string

// The supported formats.
const (
	Text     Format = "text"
	Markdown Format = "markdown"
	JSON     Format = "json"
	SARIF    Format = "sarif"
)

// Formats lists the supported format names, for usage text and validation.
func Formats() []string {
	return []string{string(Text), string(Markdown), string(JSON), string(SARIF)}
}

// Options tunes rendering.
type Options struct {
	// Color turns on ANSI colouring of the text format. The caller decides: the
	// renderer never inspects the terminal, so tests are deterministic.
	Color bool
	// Version is stamped in the header.
	Version string
	// Rules is the catalogue, for the formats that describe the tool as well as its
	// findings (SARIF). It is passed in rather than imported so this package still
	// cannot reach the rules or the realm model.
	Rules []RuleInfo
}

// Render writes the result in the named format.
func Render(w io.Writer, format Format, res engine.Result, opts Options) error {
	switch format {
	case Text:
		return renderText(w, res, opts)
	case Markdown:
		return renderMarkdown(w, res, opts)
	case JSON:
		return renderJSON(w, res, opts)
	case SARIF:
		return renderSARIF(w, res, opts)
	default:
		return fmt.Errorf("unknown output format %q (want %s)", format, strings.Join(Formats(), ", "))
	}
}

const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiGreen  = "\033[32m"
	ansiPurple = "\033[35m"
	ansiDim    = "\033[2m"
)

func colorFor(s engine.Status) string {
	switch s {
	case engine.OK:
		return ansiGreen
	case engine.WARN:
		return ansiYellow
	case engine.BAD:
		return ansiRed
	case engine.ERROR:
		return ansiPurple
	default:
		return ""
	}
}

func renderText(w io.Writer, res engine.Result, opts Options) error {
	paint := func(s engine.Status, text string) string {
		if !opts.Color {
			return text
		}
		return colorFor(s) + text + ansiReset
	}
	dim := func(text string) string {
		if !opts.Color {
			return text
		}
		return ansiDim + text + ansiReset
	}

	header := fmt.Sprintf("keycloak-doctor %s · %s · %d rule(s) · %s · %s",
		orDash(opts.Version), plural(len(res.Realms), "realm"), res.Rules, orDash(res.Source), res.Duration.Round(time.Millisecond))
	if res.Suppressed > 0 {
		// Never a silent suppression: the count sits in the header, where the run is
		// described, not in a footnote.
		header += fmt.Sprintf(" · %d suppressed", res.Suppressed)
	}
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if len(res.Findings) == 0 {
		_, err := fmt.Fprintln(w, "\nno findings")
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	// Widths are computed over the rows actually printed, so a narrow run stays
	// narrow instead of padding to the widest rule id in the catalogue.
	ruleW, scopeW := 0, 0
	scopes := make([]string, len(res.Findings))
	for i, f := range res.Findings {
		scopes[i] = scope(f)
		ruleW = max(ruleW, len(f.Rule))
		scopeW = max(scopeW, len(scopes[i]))
	}
	for i, f := range res.Findings {
		message := f.Message
		if f.New {
			// Only when a baseline was given: it marks what changed since it was taken.
			message = "NEW · " + message
		}
		line := fmt.Sprintf("%-5s  %-*s  %-*s  %s", paint(f.Status, string(f.Status)), ruleW, f.Rule, scopeW, scopes[i], message)
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
		// The remediation sits on its own line with a fixed indent rather than under
		// the message column: on a realm with long rule ids and long client names,
		// aligning it there pushes the text off the right edge of the terminal.
		if f.Remediation != "" && f.Status != engine.OK {
			if _, err := fmt.Fprintln(w, dim("       → "+f.Remediation)); err != nil {
				return err
			}
		}
	}
	sum := engine.Summarize(res.Findings)
	worst := engine.Worst(res.Findings)
	_, err := fmt.Fprintf(w, "\n%s — worst: %s\n", summaryLine(sum), paint(worst, string(worst)))
	return err
}

func renderMarkdown(w io.Writer, res engine.Result, opts Options) error {
	sum := engine.Summarize(res.Findings)
	worst := engine.Worst(res.Findings)
	b := &strings.Builder{}

	fmt.Fprintf(b, "# Keycloak audit — %s\n\n", strings.Join(res.Realms, ", "))
	fmt.Fprintf(b, "- **Worst status**: `%s`\n", worst)
	fmt.Fprintf(b, "- **Findings**: %s\n", summaryLine(sum))
	fmt.Fprintf(b, "- **Rules**: %d\n", res.Rules)
	fmt.Fprintf(b, "- **Source**: `%s`\n", orDash(res.Source))
	if res.Suppressed > 0 {
		fmt.Fprintf(b, "- **Suppressed**: %s removed by the suppression file\n", plural(res.Suppressed, "finding"))
	}
	if n := len(engine.OnlyNew(res.Findings)); n > 0 {
		fmt.Fprintf(b, "- **New since the baseline**: %d\n", n)
	}
	fmt.Fprintf(b, "- **Run**: %s in %s (keycloak-doctor %s)\n\n",
		res.Started.UTC().Format(time.RFC3339), res.Duration.Round(time.Millisecond), orDash(opts.Version))

	attention := engine.MinSeverity(res.Findings, engine.WARN)
	if len(attention) == 0 {
		b.WriteString("Nothing to look at: every rule passed.\n\n")
	} else {
		b.WriteString("## Needs attention\n\n")
		for _, f := range attention {
			fmt.Fprintf(b, "- **%s** `%s` — %s: %s%s\n", f.Status, f.Rule, scope(f), newMark(f), f.Message)
			if f.Remediation != "" {
				fmt.Fprintf(b, "  - _%s_\n", f.Remediation)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## All findings\n\n")
	b.WriteString("| Status | Rule | Realm | Target | Finding |\n|---|---|---|---|---|\n")
	for _, f := range res.Findings {
		fmt.Fprintf(b, "| %s | `%s` | %s | %s | %s%s |\n",
			f.Status, f.Rule, mdCell(f.Realm), mdCell(f.Target), newMark(f), mdCell(f.Message))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// jsonResult is the JSON shape. It is a documented contract: `worst` is what a
// pipeline gates on, `summary` is what a dashboard counts.
type jsonResult struct {
	Version  string                `json:"version,omitempty"`
	Source   string                `json:"source,omitempty"`
	Realms   []string              `json:"realms"`
	Rules    int                   `json:"rules"`
	Worst    engine.Status         `json:"worst"`
	Summary  map[engine.Status]int `json:"summary"`
	Findings []engine.Finding      `json:"findings"`
	// Suppressed is part of the contract too: a pipeline that gates on `worst` has
	// to be able to see that findings were removed from the run.
	Suppressed int       `json:"suppressed,omitempty"`
	Started    time.Time `json:"started"`
	Duration   int64     `json:"duration_ns"`
}

func renderJSON(w io.Writer, res engine.Result, opts Options) error {
	out := jsonResult{
		Version:    opts.Version,
		Source:     res.Source,
		Realms:     res.Realms,
		Rules:      res.Rules,
		Worst:      engine.Worst(res.Findings),
		Summary:    engine.Summarize(res.Findings),
		Findings:   res.Findings,
		Suppressed: res.Suppressed,
		Started:    res.Started,
		Duration:   res.Duration.Nanoseconds(),
	}
	if out.Realms == nil {
		out.Realms = []string{}
	}
	if out.Findings == nil {
		out.Findings = []engine.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// scope renders the realm and target of a finding as one column.
func scope(f engine.Finding) string {
	if f.Target == "" {
		return f.Realm
	}
	return f.Realm + " · " + f.Target
}

// summaryLine renders the per-status counts in severity order, skipping zeros.
func summaryLine(sum map[engine.Status]int) string {
	order := []engine.Status{engine.BAD, engine.ERROR, engine.WARN, engine.OK}
	var parts []string
	for _, s := range order {
		if n := sum[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	if len(parts) == 0 {
		return "no findings"
	}
	return strings.Join(parts, " · ")
}

// newMark labels a finding the baseline did not contain.
func newMark(f engine.Finding) string {
	if f.New {
		return "**NEW** "
	}
	return ""
}

func mdCell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
