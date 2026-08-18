package engine

import (
	"testing"
)

func TestSortFindingsIsWorstFirstAndStable(t *testing.T) {
	in := []Finding{
		{Rule: "b/two", Realm: "r1", Status: OK, Message: "ok"},
		{Rule: "a/one", Realm: "r1", Status: WARN, Message: "warn"},
		{Rule: "a/one", Realm: "r1", Target: "z", Status: BAD, Message: "bad z"},
		{Rule: "a/one", Realm: "r1", Target: "a", Status: BAD, Message: "bad a"},
		{Rule: "c/three", Realm: "r1", Status: ERROR, Message: "not evaluated"},
	}
	got := SortFindings(in)
	want := []string{"not evaluated", "bad a", "bad z", "warn", "ok"}
	for i, w := range want {
		if got[i].Message != w {
			t.Fatalf("position %d: got %q, want %q", i, got[i].Message, w)
		}
	}
}

func TestAtLeast(t *testing.T) {
	cases := []struct {
		status, threshold Status
		want              bool
	}{
		{BAD, WARN, true},
		{WARN, BAD, false},
		{OK, OK, true},
		{ERROR, BAD, true},
		{OK, "", true},
	}
	for _, c := range cases {
		if got := AtLeast(c.status, c.threshold); got != c.want {
			t.Errorf("AtLeast(%s, %s) = %v, want %v", c.status, c.threshold, got, c.want)
		}
	}
}

func TestKnown(t *testing.T) {
	for _, s := range []Status{OK, WARN, BAD, ERROR} {
		if !Known(s) {
			t.Errorf("%s should be known", s)
		}
	}
	for _, s := range []Status{"", "bad", "CRITICAL"} {
		if Known(s) {
			t.Errorf("%q should not be known", s)
		}
	}
}

func TestDedupKeepsFirstOccurrence(t *testing.T) {
	f := Finding{Rule: "a/one", Realm: "r", Status: BAD, Message: "m"}
	got := Dedup([]Finding{f, f, {Rule: "a/two", Realm: "r", Status: OK, Message: "x"}, f})
	if len(got) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(got), got)
	}
	if got[0] != f {
		t.Errorf("first finding changed: %+v", got[0])
	}
}

func TestMinSeverity(t *testing.T) {
	in := []Finding{
		{Status: OK}, {Status: WARN}, {Status: BAD}, {Status: ERROR},
	}
	if got := MinSeverity(in, ""); len(got) != 4 {
		t.Errorf("empty threshold should keep everything, got %d", len(got))
	}
	if got := MinSeverity(in, WARN); len(got) != 3 {
		t.Errorf("warn threshold should keep 3, got %d", len(got))
	}
	if got := MinSeverity(in, ERROR); len(got) != 1 {
		t.Errorf("error threshold should keep 1, got %d", len(got))
	}
}

func TestWorstAndSummarize(t *testing.T) {
	if got := Worst(nil); got != OK {
		t.Errorf("worst of nothing = %s, want OK", got)
	}
	in := []Finding{{Status: OK}, {Status: OK}, {Status: WARN}, {Status: BAD}}
	if got := Worst(in); got != BAD {
		t.Errorf("worst = %s, want BAD", got)
	}
	sum := Summarize(in)
	if sum[OK] != 2 || sum[WARN] != 1 || sum[BAD] != 1 || sum[ERROR] != 0 {
		t.Errorf("unexpected summary: %+v", sum)
	}
}

func TestMarkNewFlagsWhatTheBaselineDoesNotHave(t *testing.T) {
	baseline := []Finding{
		{Rule: "client/pkce", Realm: "prod", Target: "spa", Status: BAD, Message: "was already broken"},
		{Rule: "realm/enabled", Realm: "prod", Status: OK, Message: "the realm is enabled"},
		{Rule: "keys/rsa-size", Realm: "prod", Target: "legacy", Status: WARN},
	}
	findings := []Finding{
		// same finding, different message: the message carries counts and lifespans
		// that move without the posture moving, so it must not make it "new".
		{Rule: "client/pkce", Realm: "prod", Target: "spa", Status: BAD, Message: "still broken, reworded"},
		// present as OK in the baseline: "not there" and "there and passing" are
		// different, and this is the case that regressed once.
		{Rule: "realm/enabled", Realm: "prod", Status: OK},
		// WARN that became BAD: a regression, so new.
		{Rule: "keys/rsa-size", Realm: "prod", Target: "legacy", Status: BAD},
		// never seen.
		{Rule: "realm/brute-force", Realm: "prod", Status: BAD},
		// same rule, another target: a different finding.
		{Rule: "client/pkce", Realm: "prod", Target: "another-spa", Status: BAD},
	}

	if got, want := MarkNew(findings, baseline), 3; got != want {
		t.Errorf("MarkNew = %d, want %d", got, want)
	}
	for i, want := range []bool{false, false, true, true, true} {
		if findings[i].New != want {
			t.Errorf("findings[%d] (%s %s) New = %v, want %v", i, findings[i].Rule, findings[i].Target, findings[i].New, want)
		}
	}

	only := OnlyNew(findings)
	if len(only) != 3 {
		t.Fatalf("OnlyNew kept %d, want 3", len(only))
	}
	for _, f := range only {
		if !f.New {
			t.Errorf("OnlyNew kept a finding that is not new: %+v", f)
		}
	}
}

func TestMarkNewImprovementIsNotNew(t *testing.T) {
	baseline := []Finding{{Rule: "client/pkce", Realm: "prod", Target: "spa", Status: BAD}}
	findings := []Finding{{Rule: "client/pkce", Realm: "prod", Target: "spa", Status: WARN}}
	if n := MarkNew(findings, baseline); n != 0 || findings[0].New {
		t.Errorf("a BAD that became a WARN is an improvement, not a regression: n=%d new=%v", n, findings[0].New)
	}
}

func TestMarkNewWithAnEmptyBaseline(t *testing.T) {
	findings := []Finding{{Rule: "a/b", Realm: "prod", Status: BAD}, {Rule: "c/d", Realm: "prod", Status: OK}}
	if got := MarkNew(findings, nil); got != 2 {
		t.Errorf("MarkNew against no baseline = %d, want every finding (2)", got)
	}
}

func TestMarkNewKeepsTheWorstBaselineEntry(t *testing.T) {
	// A directory export can list the same realm twice; the baseline then holds two
	// entries for one finding, and the worst of them is the state being compared to.
	baseline := []Finding{
		{Rule: "a/b", Realm: "prod", Status: WARN},
		{Rule: "a/b", Realm: "prod", Status: BAD},
	}
	findings := []Finding{{Rule: "a/b", Realm: "prod", Status: BAD}}
	if got := MarkNew(findings, baseline); got != 0 {
		t.Errorf("MarkNew = %d, want 0: the baseline already had this at BAD", got)
	}
}
