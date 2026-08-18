package backlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// fakeGitHub is enough of the issues API to reconcile against: milestones,
// labels and issues in memory, with the same shapes the real endpoints return.
// No test touches the network.
type fakeGitHub struct {
	t          *testing.T
	milestones []ghMilestone
	issues     []ghIssue
	labels     map[string]bool
	calls      []string
	tokenSeen  string
}

func newFake(t *testing.T) (*fakeGitHub, *GitHub) {
	t.Helper()
	f := &fakeGitHub{t: t, labels: map[string]bool{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	gh := NewGitHub("owner/name", "token-not-a-real-secret")
	gh.BaseURL = srv.URL
	return f, gh
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.tokenSeen = r.Header.Get("Authorization")
	path := strings.TrimPrefix(r.URL.Path, "/repos/owner/name")
	f.calls = append(f.calls, r.Method+" "+path)

	decode := func(out any) {
		if err := json.NewDecoder(r.Body).Decode(out); err != nil {
			f.t.Fatalf("decoding %s %s: %v", r.Method, path, err)
		}
	}
	write := func(status int, body any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			f.t.Fatalf("encoding response: %v", err)
		}
	}
	page := r.URL.Query().Get("page")

	switch {
	case r.Method == http.MethodGet && path == "/milestones":
		if page != "1" {
			write(http.StatusOK, []ghMilestone{})
			return
		}
		write(http.StatusOK, f.milestones)

	case r.Method == http.MethodPost && path == "/milestones":
		var in ghMilestone
		decode(&in)
		in.Number = len(f.milestones) + 1
		in.State = "open"
		if in.DueOn != "" {
			// GitHub normalises the timestamp inside the day; mimic that, because it
			// is what makes a naive comparison non-idempotent.
			in.DueOn = in.DueOn[:10] + "T08:00:00Z"
		}
		f.milestones = append(f.milestones, in)
		write(http.StatusCreated, in)

	case r.Method == http.MethodPatch && strings.HasPrefix(path, "/milestones/"):
		var in ghMilestone
		decode(&in)
		m := f.milestone(path)
		m.Title, m.Description = in.Title, in.Description
		if in.DueOn != "" {
			m.DueOn = in.DueOn[:10] + "T08:00:00Z"
		}
		write(http.StatusOK, m)

	case r.Method == http.MethodPost && path == "/labels":
		var in struct{ Name string }
		decode(&in)
		if f.labels[in.Name] {
			write(http.StatusUnprocessableEntity, map[string]string{"message": "Validation Failed"})
			return
		}
		f.labels[in.Name] = true
		write(http.StatusCreated, in)

	case r.Method == http.MethodGet && path == "/issues":
		if page != "1" {
			write(http.StatusOK, []ghIssue{})
			return
		}
		write(http.StatusOK, f.issues)

	case r.Method == http.MethodPost && path == "/issues":
		var in struct {
			Title     string   `json:"title"`
			Body      string   `json:"body"`
			Labels    []string `json:"labels"`
			Milestone int      `json:"milestone"`
		}
		decode(&in)
		is := ghIssue{Number: len(f.issues) + 1, Title: in.Title, Body: in.Body, State: "open"}
		for _, l := range in.Labels {
			is.Labels = append(is.Labels, ghLabel{Name: l})
		}
		if in.Milestone > 0 {
			is.Milestone = &ghMilestone{Number: in.Milestone}
		}
		f.issues = append(f.issues, is)
		write(http.StatusCreated, is)

	case r.Method == http.MethodPatch && strings.HasPrefix(path, "/issues/"):
		var in map[string]any
		decode(&in)
		is := f.issue(path)
		if v, ok := in["title"].(string); ok {
			is.Title = v
		}
		if v, ok := in["body"].(string); ok {
			is.Body = v
		}
		if v, ok := in["state"].(string); ok {
			is.State = v
		}
		if v, ok := in["labels"].([]any); ok {
			is.Labels = nil
			for _, l := range v {
				is.Labels = append(is.Labels, ghLabel{Name: l.(string)})
			}
		}
		if v, present := in["milestone"]; present {
			switch n := v.(type) {
			case float64:
				is.Milestone = &ghMilestone{Number: int(n)}
			default:
				is.Milestone = nil
			}
		}
		write(http.StatusOK, is)

	default:
		f.t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		write(http.StatusNotFound, map[string]string{"message": "Not Found"})
	}
}

func (f *fakeGitHub) milestone(path string) *ghMilestone {
	n := numberFrom(f.t, path)
	for i := range f.milestones {
		if f.milestones[i].Number == n {
			return &f.milestones[i]
		}
	}
	f.t.Fatalf("no milestone %d", n)
	return nil
}

func (f *fakeGitHub) issue(path string) *ghIssue {
	n := numberFrom(f.t, path)
	for i := range f.issues {
		if f.issues[i].Number == n {
			return &f.issues[i]
		}
	}
	f.t.Fatalf("no issue %d", n)
	return nil
}

func (f *fakeGitHub) byTitlePrefix(id string) *ghIssue {
	for i := range f.issues {
		if got, ok := backlogID(f.issues[i].Title); ok && got == id {
			return &f.issues[i]
		}
	}
	f.t.Fatalf("no issue tracking %s", id)
	return nil
}

func numberFrom(t *testing.T, path string) int {
	t.Helper()
	parts := strings.Split(path, "/")
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		t.Fatalf("no number in %q: %v", path, err)
	}
	return n
}

func kinds(actions []Action) string {
	var out []string
	for _, a := range actions {
		out = append(out, a.Kind+":"+a.Ref)
	}
	return strings.Join(out, " ")
}

func sync(t *testing.T, gh *GitHub, doc Doc) []Action {
	t.Helper()
	actions, err := gh.Sync(context.Background(), doc)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	return actions
}

func TestSyncCreatesMilestonesAndIssues(t *testing.T) {
	f, gh := newFake(t)
	doc := parseSample(t)

	actions := sync(t, gh, doc)

	// Two milestones, and an issue for each of the four open items — never for the
	// done item, which was shipped before the mirror existed.
	if got, want := len(f.milestones), 2; got != want {
		t.Errorf("milestones created = %d, want %d", got, want)
	}
	if got, want := len(f.issues), 4; got != want {
		t.Fatalf("issues created = %d, want %d (%s)", got, want, kinds(actions))
	}
	if got, want := len(actions), 6; got != want {
		t.Errorf("actions = %d, want %d (%s)", got, want, kinds(actions))
	}

	kd10 := f.byTitlePrefix("KD-10")
	if kd10.Milestone == nil || kd10.Milestone.Number != 1 {
		t.Errorf("KD-10 is not in the first milestone: %+v", kd10.Milestone)
	}
	if got := strings.Join(labelNames(kd10.Labels), ","); got != "backlog,area/sources-and-integration" {
		t.Errorf("KD-10 labels = %q", got)
	}
	if !f.labels["backlog"] || !f.labels["area/rules"] {
		t.Errorf("labels not ensured: %v", f.labels)
	}
	if f.tokenSeen != "Bearer token-not-a-real-secret" {
		t.Errorf("Authorization header = %q", f.tokenSeen)
	}

	kd1 := f.byTitlePrefix("KD-1")
	if kd1.Milestone != nil {
		t.Errorf("KD-1 has no milestone in the file but got %+v", kd1.Milestone)
	}
	if !strings.Contains(kd1.Body, "BACKLOG.md") {
		t.Errorf("issue body has no pointer back to the file:\n%s", kd1.Body)
	}
}

// The workflow runs on every push, so a second run on an unchanged file must be
// a no-op — otherwise every push would rewrite every issue and spam its watchers.
func TestSyncIsIdempotent(t *testing.T) {
	f, gh := newFake(t)
	doc := parseSample(t)

	sync(t, gh, doc)
	before := len(f.issues)

	actions := sync(t, gh, doc)
	if len(actions) != 0 {
		t.Errorf("second sync acted: %s", kinds(actions))
	}
	if len(f.issues) != before {
		t.Errorf("second sync created issues: %d, want %d", len(f.issues), before)
	}
	if len(f.milestones) != 2 {
		t.Errorf("second sync created milestones: %d, want 2", len(f.milestones))
	}
}

func TestSyncClosesItemMovedToDone(t *testing.T) {
	f, gh := newFake(t)
	sync(t, gh, parseSample(t))

	// KD-1 moves from Open to Done, the way finishing an item is recorded.
	moved := strings.Replace(sample, "- **KD-1** — `client/consent`", "MOVED", 1)
	moved = strings.Replace(moved, "## Done\n", "## Done\n\n- **KD-1** — `client/consent`: report a client that skips consent. Needs the client scopes.\n", 1)
	moved = strings.Replace(moved, "MOVED: report a client that skips consent. Needs the client scopes.\n", "", 1)
	doc, err := Parse([]byte(moved))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if it, _ := itemByID(doc, "KD-1"); !it.Done {
		t.Fatal("fixture did not move KD-1 to Done")
	}

	actions := sync(t, gh, doc)
	if got, want := kinds(actions), "close-issue:KD-1"; got != want {
		t.Errorf("actions = %q, want %q", got, want)
	}
	if got := f.byTitlePrefix("KD-1").State; got != "closed" {
		t.Errorf("KD-1 issue state = %q, want closed", got)
	}

	// And it stays closed: a done item must not be reopened on the next push.
	if actions := sync(t, gh, doc); len(actions) != 0 {
		t.Errorf("sync after closing acted again: %s", kinds(actions))
	}
}

func TestSyncRepairsEditedIssueAndReportsOrphan(t *testing.T) {
	f, gh := newFake(t)
	doc := parseSample(t)
	sync(t, gh, doc)

	// Somebody edited the issue by hand and added a label of their own; the file is
	// the source of truth, so title and body go back — and the extra label stays.
	kd11 := f.byTitlePrefix("KD-11")
	kd11.Title = "KD-11: renamed by hand"
	kd11.Body = "rewritten by hand"
	kd11.Labels = append(kd11.Labels, ghLabel{Name: "help wanted"})

	// An issue that looks like a backlog item but has no item behind it is reported
	// and left alone: closing somebody's issue is not this tool's call.
	f.issues = append(f.issues, ghIssue{Number: 99, Title: "KD-42: an id that is not in the file", State: "open"})
	// A pull request comes back from the same endpoint and must be ignored.
	f.issues = append(f.issues, ghIssue{Number: 100, Title: "KD-1: a pull request", State: "open", PullRequest: &struct{}{}})

	actions := sync(t, gh, doc)
	if got, want := kinds(actions), "update-issue:KD-11 orphan-issue:KD-42"; got != want {
		t.Errorf("actions = %q, want %q", got, want)
	}

	fixed := f.byTitlePrefix("KD-11")
	want, _ := itemByID(doc, "KD-11")
	if fixed.Title != want.Title() {
		t.Errorf("title = %q, want %q", fixed.Title, want.Title())
	}
	if !strings.Contains(fixed.Body, "GitHub Action wrapping the audit") {
		t.Errorf("body not restored:\n%s", fixed.Body)
	}
	if got := strings.Join(labelNames(fixed.Labels), ","); !strings.Contains(got, "help wanted") {
		t.Errorf("hand-added label was dropped: %q", got)
	}
}

func TestSyncDryRunTouchesNothing(t *testing.T) {
	f, gh := newFake(t)
	gh.DryRun = true

	actions := sync(t, gh, parseSample(t))
	if len(actions) == 0 {
		t.Fatal("dry run reported no actions on an empty repository")
	}
	if len(f.issues) != 0 || len(f.milestones) != 0 || len(f.labels) != 0 {
		t.Errorf("dry run mutated the repository: %d issues, %d milestones, %d labels", len(f.issues), len(f.milestones), len(f.labels))
	}
	for _, call := range f.calls {
		if !strings.HasPrefix(call, "GET ") {
			t.Errorf("dry run issued a mutating call: %s", call)
		}
	}
}

// A dry run must report everything a real run would do. When the milestone does
// not exist yet there is no number to patch the issues with, and an earlier
// version of this reported only "create-milestone" — which read as if the seven
// issues were already in it.
func TestDryRunReportsIssuesMovedIntoAMilestoneThatDoesNotExistYet(t *testing.T) {
	f, gh := newFake(t)
	doc := parseSample(t)

	sync(t, gh, doc) // the issues and both milestones now exist

	// Drop the milestones, keep the issues: the state of a repository where a new
	// milestone was just declared in BACKLOG.md.
	f.milestones = nil
	for i := range f.issues {
		f.issues[i].Milestone = nil
	}
	gh.DryRun = true

	actions := sync(t, gh, doc)
	if got, want := kinds(actions), "create-milestone:v0.2.0 create-milestone:v0.3.0 update-issue:KD-8 update-issue:KD-10 update-issue:KD-11"; got != want {
		t.Errorf("actions = %q, want %q", got, want)
	}
	for _, a := range actions {
		if a.Kind == "update-issue" && !strings.Contains(a.Detail, "to be created") {
			t.Errorf("%s does not say the milestone is still to be created: %q", a.Ref, a.Detail)
		}
	}
	// And it is still a dry run: nothing was written.
	if len(f.milestones) != 0 {
		t.Errorf("the dry run created %d milestone(s)", len(f.milestones))
	}
}

// A released milestone leaves BACKLOG.md — the file is what is still planned —
// and the issues that closed under it must keep it. Stripping it would erase the
// record of what shipped in that release, which is exactly what happened once.
func TestSyncKeepsAMilestoneTheFileNoLongerDeclares(t *testing.T) {
	f, gh := newFake(t)
	doc := parseSample(t)
	sync(t, gh, doc)

	before := f.byTitlePrefix("KD-10").Milestone
	if before == nil {
		t.Fatal("KD-10 did not get its milestone in the first place")
	}

	// v0.2.0 shipped: its bullet goes away, and its items move to Done.
	shipped := strings.Replace(sample, "- **v0.2.0** (due 2026-09-30) — Pull-request gating: the audit as a required check.\n  Items: KD-10, KD-11.\n", "", 1)
	trimmed, err := Parse([]byte(shipped))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if it, _ := itemByID(trimmed, "KD-10"); it.Milestone != "" {
		t.Fatalf("the fixture still claims KD-10 in a milestone: %q", it.Milestone)
	}

	for _, a := range sync(t, gh, trimmed) {
		if strings.Contains(a.Detail, "milestone cleared") {
			t.Errorf("the sync stripped a shipped milestone: %s", a)
		}
	}
	if after := f.byTitlePrefix("KD-10").Milestone; after == nil || after.Number != before.Number {
		t.Errorf("KD-10 lost its milestone: %+v, want %+v", after, before)
	}
}

// Moving an item between two milestones the file declares still works.
func TestSyncMovesAnItemBetweenDeclaredMilestones(t *testing.T) {
	f, gh := newFake(t)
	sync(t, gh, parseSample(t))

	moved := strings.Replace(sample, "- **v0.3.0** — Desired state sources. Items: KD-8.", "- **v0.3.0** — Desired state sources. Items: KD-8, KD-10.", 1)
	moved = strings.Replace(moved, "Items: KD-10, KD-11.", "Items: KD-11.", 1)
	doc, err := Parse([]byte(moved))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := kinds(sync(t, gh, doc)); got != "update-issue:KD-10" {
		t.Fatalf("actions = %q, want the one move", got)
	}
	kd10 := f.byTitlePrefix("KD-10")
	if kd10.Milestone == nil || kd10.Milestone.Number != 2 {
		t.Errorf("KD-10 is in %+v, want the second milestone", kd10.Milestone)
	}
}

func TestSyncMilestoneDescriptionUpdate(t *testing.T) {
	f, gh := newFake(t)
	sync(t, gh, parseSample(t))

	changed := strings.Replace(sample, "Pull-request gating: the audit as a required check.", "A rewritten scope.", 1)
	doc, err := Parse([]byte(changed))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	actions := sync(t, gh, doc)
	if got, want := kinds(actions), "update-milestone:v0.2.0"; got != want {
		t.Fatalf("actions = %q, want %q", got, want)
	}
	if got := f.milestones[0].Description; got != "A rewritten scope" {
		t.Errorf("description = %q", got)
	}
}

func TestSyncSurfacesAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintln(w, `{"message":"Resource not accessible by integration"}`)
	}))
	defer srv.Close()

	gh := NewGitHub("owner/name", "token-not-a-real-secret")
	gh.BaseURL = srv.URL

	_, err := gh.Sync(context.Background(), parseSample(t))
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want an *APIError", err)
	}
	if apiErr.Status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", apiErr.Status)
	}
	// The message has to name the failing call, and must not carry the token.
	if !strings.Contains(err.Error(), "/milestones") || strings.Contains(err.Error(), "token-not-a-real-secret") {
		t.Errorf("error = %q", err)
	}
}

func TestSyncNeedsARepository(t *testing.T) {
	gh := NewGitHub("", "")
	if _, err := gh.Sync(context.Background(), Doc{}); err == nil {
		t.Fatal("Sync accepted an empty repository")
	}
}
