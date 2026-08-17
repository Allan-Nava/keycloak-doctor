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
