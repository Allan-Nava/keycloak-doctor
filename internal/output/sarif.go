package output

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
)

// SARIF 2.1.0, for GitHub code scanning: the audit becomes alerts on the
// repository that holds the realm definition, next to the code, instead of a
// report somebody has to open a job log to read.
//
// Two mappings are deliberate. An ERROR finding — a rule that could not be
// evaluated — is a real result at level "note", not a skipped rule: a blind spot
// has to be visible in the same list as the findings, or the alert page reads as
// a clean bill. And an OK finding is a result of kind "pass" at level "none",
// which code scanning does not raise an alert for but a SARIF reader still sees,
// so the file stays a faithful projection of the run.
const (
	sarifSchema  = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion = "2.1.0"
	toolURI      = "https://allan-nava.github.io/keycloak-doctor/"
)

// RuleInfo is what the SARIF tool descriptor needs about a rule. The output
// package never imports the catalogue — the caller passes it in Options.Rules —
// so a renderer still cannot reach the realm model.
type RuleInfo struct {
	ID        string
	Title     string
	Rationale string
}

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Invocations []sarifInvocation `json:"invocations,omitempty"`
	Artifacts   []sarifArtifact   `json:"artifacts,omitempty"`
	Results     []sarifResult     `json:"results"`
	Properties  map[string]any    `json:"properties,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name,omitempty"`
	ShortDescription     sarifText      `json:"shortDescription"`
	FullDescription      *sarifText     `json:"fullDescription,omitempty"`
	Help                 *sarifText     `json:"help,omitempty"`
	DefaultConfiguration *sarifConfig   `json:"defaultConfiguration,omitempty"`
	Properties           map[string]any `json:"properties,omitempty"`
}

type sarifConfig struct {
	Level string `json:"level"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifInvocation struct {
	CommandLine         string `json:"commandLine,omitempty"`
	ExecutionSuccessful bool   `json:"executionSuccessful"`
	StartTimeUTC        string `json:"startTimeUtc,omitempty"`
	EndTimeUTC          string `json:"endTimeUtc,omitempty"`
}

type sarifArtifact struct {
	Location sarifArtifactLocation `json:"location"`
	Roles    []string              `json:"roles,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Kind                string            `json:"kind,omitempty"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
	Properties          map[string]any    `json:"properties,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation *sarifPhysicalLocation `json:"physicalLocation,omitempty"`
	LogicalLocations []sarifLogicalLocation `json:"logicalLocations,omitempty"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

type sarifLogicalLocation struct {
	Name               string `json:"name"`
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`
	Kind               string `json:"kind,omitempty"`
}

func renderSARIF(w io.Writer, res engine.Result, opts Options) error {
	catalogue, index := sarifRules(res, opts)
	artifact := sourceArtifact(res.Source)

	results := make([]sarifResult, 0, len(res.Findings))
	for _, f := range res.Findings {
		level, kind := sarifLevel(f.Status)
		r := sarifResult{
			RuleID:    f.Rule,
			RuleIndex: index[f.Rule],
			Level:     level,
			Kind:      kind,
			Message:   sarifText{Text: sarifMessage(f)},
			Locations: []sarifLocation{{
				LogicalLocations: []sarifLogicalLocation{logicalLocation(f)},
			}},
			// Code scanning keeps an alert across runs by its fingerprint, so it has to
			// be stable and it must not contain the message: a lifespan or a count in
			// the text would otherwise close the alert and open a new one on every run.
			PartialFingerprints: map[string]string{
				"keycloakDoctor/v1": fingerprint(f),
			},
			Properties: map[string]any{
				"realm":  f.Realm,
				"status": string(f.Status),
			},
		}
		if f.Target != "" {
			r.Properties["target"] = f.Target
		}
		if f.Remediation != "" {
			r.Properties["remediation"] = f.Remediation
		}
		if f.New {
			r.Properties["new"] = true
		}
		// A realm export is a file in the repository being scanned, so the alert can
		// be anchored to it. A live server is not a file, and inventing a location
		// for it would put an alert on whatever file happened to be named.
		if artifact != "" {
			r.Locations[0].PhysicalLocation = &sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: artifact},
				Region:           &sarifRegion{StartLine: 1},
			}
		}
		results = append(results, r)
	}

	run := sarifRun{
		Tool: sarifTool{Driver: sarifDriver{
			Name:           "keycloak-doctor",
			Version:        opts.Version,
			InformationURI: toolURI,
			Rules:          catalogue,
		}},
		Invocations: []sarifInvocation{{
			ExecutionSuccessful: true,
			StartTimeUTC:        res.Started.UTC().Format("2006-01-02T15:04:05.000Z"),
			EndTimeUTC:          res.Started.Add(res.Duration).UTC().Format("2006-01-02T15:04:05.000Z"),
		}},
		Results: results,
		Properties: map[string]any{
			"realms":     res.Realms,
			"rules":      res.Rules,
			"worst":      string(engine.Worst(res.Findings)),
			"suppressed": res.Suppressed,
		},
	}
	if artifact != "" {
		run.Artifacts = []sarifArtifact{{
			Location: sarifArtifactLocation{URI: artifact},
			Roles:    []string{"analysisTarget"},
		}}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sarifLog{Schema: sarifSchema, Version: sarifVersion, Runs: []sarifRun{run}})
}

// sarifRules builds the tool descriptor. Every rule of the catalogue is listed,
// not only the ones that produced a result, because the descriptor is what tells
// a reader of the file what the tool looks for at all. When the caller gave no
// catalogue, the ids seen in the findings are used instead of dropping the
// section.
func sarifRules(res engine.Result, opts Options) ([]sarifRule, map[string]int) {
	infos := opts.Rules
	if len(infos) == 0 {
		seen := map[string]bool{}
		for _, f := range res.Findings {
			if seen[f.Rule] {
				continue
			}
			seen[f.Rule] = true
			infos = append(infos, RuleInfo{ID: f.Rule, Title: f.Rule})
		}
	}

	rules := make([]sarifRule, 0, len(infos))
	index := make(map[string]int, len(infos))
	for _, info := range infos {
		index[info.ID] = len(rules)
		rule := sarifRule{
			ID:               info.ID,
			Name:             sarifName(info.ID),
			ShortDescription: sarifText{Text: orDash(info.Title)},
			DefaultConfiguration: &sarifConfig{
				// The level of a result is decided per finding, not per rule: the same
				// rule is a BAD on a public client and a WARN on a confidential one.
				Level: "warning",
			},
			Properties: map[string]any{
				"tags":     []string{"security", "keycloak", "configuration", sarifCategory(info.ID)},
				"category": sarifCategory(info.ID),
			},
		}
		if info.Rationale != "" {
			rule.FullDescription = &sarifText{Text: info.Rationale}
			rule.Help = &sarifText{Text: info.Rationale}
		}
		rules = append(rules, rule)
	}
	return rules, index
}

// sarifMessage keeps the remediation in the alert body: an alert that says what is
// wrong without saying what to do is a ticket nobody can close.
func sarifMessage(f engine.Finding) string {
	msg := f.Message
	if f.Target != "" {
		msg = f.Target + ": " + msg
	}
	if f.Remediation != "" {
		msg += " → " + f.Remediation
	}
	return msg
}

func logicalLocation(f engine.Finding) sarifLogicalLocation {
	name, kind := f.Realm, "namespace"
	fqn := f.Realm
	if f.Target != "" {
		name, kind = f.Target, "member"
		fqn = f.Realm + "/" + f.Target
	}
	return sarifLogicalLocation{Name: name, FullyQualifiedName: fqn, Kind: kind}
}

// sarifLevel maps a status onto a SARIF level and kind. OK is the only status that
// is not a failure; ERROR is a failure at "note" because a rule that could not run
// is a gap in the audit, and a gap has to be visible.
func sarifLevel(s engine.Status) (level, kind string) {
	switch s {
	case engine.BAD:
		return "error", ""
	case engine.WARN:
		return "warning", ""
	case engine.ERROR:
		return "note", ""
	case engine.OK:
		return "none", "pass"
	default:
		return "none", ""
	}
}

// sourceArtifact turns the run's source into a repository-relative path, or "" for
// a live server. A path outside the workspace is dropped rather than emitted as an
// absolute URI: code scanning rejects a file it cannot find in the checkout.
func sourceArtifact(source string) string {
	path, ok := strings.CutPrefix(source, "file:")
	if !ok {
		return ""
	}
	path = strings.TrimPrefix(path, "./")
	if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "..") {
		return ""
	}
	return path
}

// sarifName is the CamelCase name SARIF wants next to the id ("client/pkce" ->
// "ClientPkce"). Rule ids are ASCII lowercase and hyphenated, so this needs no
// unicode casing rules.
func sarifName(id string) string {
	b := &strings.Builder{}
	upper := true
	for _, r := range id {
		switch {
		case r == '/' || r == '-' || r == '_':
			upper = true
		case upper:
			b.WriteString(strings.ToUpper(string(r)))
			upper = false
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sarifCategory(id string) string {
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	return id
}

func fingerprint(f engine.Finding) string {
	sum := sha256.Sum256([]byte(f.Rule + "|" + f.Realm + "|" + f.Target))
	return hex.EncodeToString(sum[:8])
}
