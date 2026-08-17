package backlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// DefaultAPI is the GitHub REST endpoint. It is a field on [GitHub] so the tests
// can point the whole reconciliation at an httptest server.
const DefaultAPI = "https://api.github.com"

// GitHub mirrors a parsed backlog into the issues and milestones of one
// repository. Every operation is idempotent: running a sync twice on an
// unchanged file must produce no action the second time, because the workflow
// runs it on every push.
type GitHub struct {
	Repo    string // "owner/name"
	Token   string
	BaseURL string
	HTTP    *http.Client
	DryRun  bool
}

// NewGitHub returns a client for one repository.
func NewGitHub(repo, token string) *GitHub {
	return &GitHub{
		Repo:    repo,
		Token:   token,
		BaseURL: DefaultAPI,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// pendingMilestone is the number a milestone gets in a dry run, where nothing is
// created and no real number exists yet. It keeps the preview honest: an issue
// that *would* be moved into a milestone still has to be reported, or a dry run
// would understate the work and read as if only the milestone were missing.
const pendingMilestone = -1

// Action is one thing a sync did, or would do in a dry run.
type Action struct {
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Detail string `json:"detail"`
}

func (a Action) String() string {
	if a.Detail == "" {
		return fmt.Sprintf("`%s` **%s**", a.Kind, a.Ref)
	}
	return fmt.Sprintf("`%s` **%s** — %s", a.Kind, a.Ref, a.Detail)
}

type ghMilestone struct {
	Number      int    `json:"number,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DueOn       string `json:"due_on,omitempty"`
	State       string `json:"state,omitempty"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghIssue struct {
	Number      int          `json:"number"`
	Title       string       `json:"title"`
	Body        string       `json:"body"`
	State       string       `json:"state"`
	Milestone   *ghMilestone `json:"milestone"`
	Labels      []ghLabel    `json:"labels"`
	PullRequest *struct{}    `json:"pull_request"`
}

// APIError is a non-2xx answer from the GitHub API. It carries the status so a
// caller can tolerate the ones that mean "already there".
type APIError struct {
	Status int
	Method string
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// Sync reconciles the repository with the backlog: it creates the milestones the
// file declares, opens an issue for every open item, keeps title, body,
// milestone and labels in step with the file, and closes the issue of an item
// that moved to Done. It never deletes anything, and an issue with no matching
// item is reported and left alone — the file owns the items, not the tracker.
func (g *GitHub) Sync(ctx context.Context, d Doc) ([]Action, error) {
	if g.Repo == "" {
		return nil, errors.New("no repository: pass --repo owner/name (GITHUB_REPOSITORY is used when set)")
	}

	acts, numbers, err := g.syncMilestones(ctx, d)
	if err != nil {
		return acts, err
	}

	if err := g.ensureLabels(ctx, d); err != nil {
		return acts, err
	}

	issues, err := listAll[ghIssue](ctx, g, "/repos/"+g.Repo+"/issues?state=all")
	if err != nil {
		return acts, err
	}
	byID := map[string]ghIssue{}
	for _, is := range issues {
		if is.PullRequest != nil { // the issues endpoint returns pull requests too
			continue
		}
		id, ok := backlogID(is.Title)
		if !ok {
			continue
		}
		if prev, dup := byID[id]; dup {
			acts = append(acts, Action{"duplicate-issue", id, fmt.Sprintf("#%d and #%d both track %s; syncing the older one only", min(prev.Number, is.Number), max(prev.Number, is.Number), id)})
			if prev.Number < is.Number {
				continue
			}
		}
		byID[id] = is
	}

	for _, it := range d.Items {
		is, tracked := byID[it.ID]
		if !tracked {
			// A done item that was never tracked stays untracked: the mirror is for
			// work in flight, not an archaeology of what shipped before it existed.
			if it.Done {
				continue
			}
			act, err := g.createIssue(ctx, it, numbers)
			acts = append(acts, act)
			if err != nil {
				return acts, err
			}
			continue
		}
		act, changed, err := g.updateIssue(ctx, it, is, numbers)
		if changed {
			acts = append(acts, act)
		}
		if err != nil {
			return acts, err
		}
	}

	var orphans []Action
	for id, is := range byID {
		if _, ok := itemByID(d, id); !ok {
			orphans = append(orphans, Action{"orphan-issue", id, fmt.Sprintf("#%d has no item in BACKLOG.md; left untouched", is.Number)})
		}
	}
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Ref < orphans[j].Ref })
	return append(acts, orphans...), nil
}

func (g *GitHub) syncMilestones(ctx context.Context, d Doc) ([]Action, map[string]int, error) {
	var acts []Action
	numbers := map[string]int{}

	existing, err := listAll[ghMilestone](ctx, g, "/repos/"+g.Repo+"/milestones?state=all")
	if err != nil {
		return nil, nil, err
	}
	byTitle := map[string]ghMilestone{}
	for _, m := range existing {
		byTitle[m.Title] = m
	}

	for _, m := range d.Milestones {
		want := map[string]any{"title": m.Title, "description": m.Description}
		if m.Due != "" {
			want["due_on"] = m.Due + "T23:59:59Z"
		}

		cur, ok := byTitle[m.Title]
		if !ok {
			acts = append(acts, Action{"create-milestone", m.Title, m.Description})
			if g.DryRun {
				numbers[m.Title] = pendingMilestone
				continue
			}
			var created ghMilestone
			if err := g.do(ctx, http.MethodPost, "/repos/"+g.Repo+"/milestones", want, &created); err != nil {
				return acts, numbers, err
			}
			numbers[m.Title] = created.Number
			continue
		}

		numbers[m.Title] = cur.Number
		// due_on comes back as a timestamp GitHub picked inside the day, so only the
		// date can be compared — otherwise every run would "update" the milestone.
		if cur.Description == m.Description && dateOf(cur.DueOn) == m.Due {
			continue
		}
		acts = append(acts, Action{"update-milestone", m.Title, "description or due date changed"})
		if g.DryRun {
			continue
		}
		if err := g.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/milestones/%d", g.Repo, cur.Number), want, nil); err != nil {
			return acts, numbers, err
		}
	}
	return acts, numbers, nil
}

// ensureLabels creates the labels the sync uses. A label that already exists
// answers 422, which is the success case here.
func (g *GitHub) ensureLabels(ctx context.Context, d Doc) error {
	if g.DryRun {
		return nil
	}
	wanted := map[string]bool{}
	for _, it := range d.Open() {
		for _, l := range it.Labels() {
			wanted[l] = true
		}
	}
	names := make([]string, 0, len(wanted))
	for name := range wanted {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		body := map[string]any{"name": name, "color": "1d76db", "description": "Mirrored from BACKLOG.md"}
		if name == "backlog" {
			body["color"] = "5319e7"
		}
		err := g.do(ctx, http.MethodPost, "/repos/"+g.Repo+"/labels", body, nil)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *GitHub) createIssue(ctx context.Context, it Item, numbers map[string]int) (Action, error) {
	act := Action{"create-issue", it.ID, it.Title()}
	if g.DryRun {
		return act, nil
	}
	payload := map[string]any{"title": it.Title(), "body": it.Body(g.Repo), "labels": it.Labels()}
	if n := numbers[it.Milestone]; n > 0 {
		payload["milestone"] = n
	}
	return act, g.do(ctx, http.MethodPost, "/repos/"+g.Repo+"/issues", payload, nil)
}

func (g *GitHub) updateIssue(ctx context.Context, it Item, is ghIssue, numbers map[string]int) (Action, bool, error) {
	patch := map[string]any{}
	var changed []string

	if is.Title != it.Title() {
		patch["title"] = it.Title()
		changed = append(changed, "title")
	}
	if strings.TrimSpace(is.Body) != strings.TrimSpace(it.Body(g.Repo)) {
		patch["body"] = it.Body(g.Repo)
		changed = append(changed, "body")
	}

	current := 0
	if is.Milestone != nil {
		current = is.Milestone.Number
	}
	switch want := numbers[it.Milestone]; {
	case it.Milestone == "" && current != 0:
		patch["milestone"] = nil
		changed = append(changed, "milestone cleared")
	case it.Milestone != "" && want > 0 && want != current:
		patch["milestone"] = want
		changed = append(changed, "milestone "+it.Milestone)
	case it.Milestone != "" && want == pendingMilestone && current == 0:
		// Dry run only: the milestone does not exist yet, so there is no number to
		// patch with — but the move is part of what a real run would do.
		changed = append(changed, "milestone "+it.Milestone+" (to be created)")
	}

	if missing := missingLabels(is.Labels, it.Labels()); len(missing) > 0 {
		patch["labels"] = append(labelNames(is.Labels), missing...)
		changed = append(changed, "labels "+strings.Join(missing, ", "))
	}

	kind := "update-issue"
	wantState := "open"
	if it.Done {
		wantState = "closed"
	}
	if is.State != wantState {
		patch["state"] = wantState
		if it.Done {
			patch["state_reason"] = "completed"
			kind = "close-issue"
		} else {
			kind = "reopen-issue"
		}
		changed = append(changed, "state "+wantState)
	}

	// changed drives the report, patch drives the request: in a dry run a pending
	// milestone changes the former without being able to fill the latter.
	if len(changed) == 0 {
		return Action{}, false, nil
	}
	act := Action{kind, it.ID, fmt.Sprintf("#%d: %s", is.Number, strings.Join(changed, ", "))}
	if g.DryRun || len(patch) == 0 {
		return act, true, nil
	}
	return act, true, g.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/issues/%d", g.Repo, is.Number), patch, nil)
}

func (g *GitHub) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("%s %s: %w", method, path, err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, g.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

	client := g.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%s %s: reading response: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{Status: resp.StatusCode, Method: method, Path: path, Body: firstLine(data)}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: decoding response: %w", method, path, err)
	}
	return nil
}

// listAll walks a paginated collection. The cap is a runaway guard: no backlog
// mirror has 2000 issues to reconcile.
func listAll[T any](ctx context.Context, g *GitHub, path string) ([]T, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	var all []T
	for page := 1; page <= 20; page++ {
		var chunk []T
		if err := g.do(ctx, http.MethodGet, fmt.Sprintf("%s%sper_page=100&page=%d", path, sep, page), nil, &chunk); err != nil {
			return nil, err
		}
		all = append(all, chunk...)
		if len(chunk) < 100 {
			break
		}
	}
	return all, nil
}

// backlogID reads the id off an issue title. The "KD-n: " prefix is how an issue
// is matched back to its item, so a renamed title is repaired, not duplicated.
func backlogID(title string) (string, bool) {
	t := strings.TrimSpace(title)
	loc := idRe.FindStringIndex(t)
	if loc == nil || loc[0] != 0 {
		return "", false
	}
	if !strings.HasPrefix(t[loc[1]:], ":") {
		return "", false
	}
	return t[:loc[1]], true
}

func itemByID(d Doc, id string) (Item, bool) {
	for _, it := range d.Items {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

func labelNames(labels []ghLabel) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.Name)
	}
	return out
}

// missingLabels returns the wanted labels an issue does not carry. Labels added
// by hand are kept: the file owns what it declares, not the whole label set.
func missingLabels(have []ghLabel, want []string) []string {
	present := map[string]bool{}
	for _, l := range have {
		present[l.Name] = true
	}
	var missing []string
	for _, w := range want {
		if !present[w] {
			missing = append(missing, w)
		}
	}
	return missing
}

func dateOf(timestamp string) string {
	if len(timestamp) < 10 {
		return ""
	}
	return timestamp[:10]
}

func firstLine(data []byte) string {
	s := strings.TrimSpace(string(data))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const max = 200
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
