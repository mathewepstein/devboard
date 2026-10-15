package items

import (
	"testing"
	"time"

	"github.com/mathewepstein/devboard/internal/state"
)

func repo(prs ...state.PR) []state.Repo {
	return []state.Repo{{Name: "MW", GHRepo: "member-websites", PRs: prs}}
}

func find(items []state.Item, rule string, number int) *state.Item {
	for i := range items {
		if items[i].Rule == rule && items[i].Number == number {
			return &items[i]
		}
	}
	return nil
}

func TestReviewRequested(t *testing.T) {
	got := Derive("me", repo(
		state.PR{Number: 1, Author: "other", Requested: []string{"me"}},
		state.PR{Number: 2, Author: "other", Requested: []string{"someone"}},
		state.PR{Number: 3, Author: "me", Requested: []string{"me"}},
	))
	if len(got) != 1 || got[0].Rule != "review" || got[0].Number != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestFixRules(t *testing.T) {
	got := Derive("me", repo(
		state.PR{Number: 1, Author: "me", ReviewDecision: "CHANGES_REQUESTED", Checks: "ok"},
		state.PR{Number: 2, Author: "me", Checks: "fail"},
		state.PR{Number: 3, Author: "me", ReviewDecision: "CHANGES_REQUESTED", Checks: "fail"},
		state.PR{Number: 4, Author: "other", ReviewDecision: "CHANGES_REQUESTED", Checks: "fail"},
		state.PR{Number: 5, Author: "me", Draft: true, Checks: "fail"},
	))
	if it := find(got, "fix", 1); it == nil || it.Detail != "changes requested" {
		t.Errorf("pr 1: %+v", it)
	}
	if it := find(got, "fix", 2); it == nil || it.Detail != "checks failing" {
		t.Errorf("pr 2: %+v", it)
	}
	if it := find(got, "fix", 3); it == nil || it.Detail != "changes requested, checks failing" {
		t.Errorf("pr 3: %+v", it)
	}
	if find(got, "fix", 4) != nil {
		t.Error("other author's PR must not be a fix item")
	}
	if find(got, "fix", 5) != nil {
		t.Error("draft must not be a fix item")
	}
	if len(got) != 3 {
		t.Fatalf("want 3 items, got %d: %+v", len(got), got)
	}
}

func TestMergeRule(t *testing.T) {
	got := Derive("me", repo(
		state.PR{Number: 1, Author: "me", ReviewDecision: "APPROVED", Checks: "ok"},
		state.PR{Number: 2, Author: "me", ReviewDecision: "APPROVED", Checks: "none"},
		state.PR{Number: 3, Author: "me", ReviewDecision: "APPROVED", Checks: "fail"},
		state.PR{Number: 4, Author: "me", ReviewDecision: "APPROVED", Draft: true},
		state.PR{Number: 5, Author: "me", ReviewDecision: "APPROVED", Mergeable: "CONFLICTING"},
		state.PR{Number: 6, Author: "other", ReviewDecision: "APPROVED"},
	))
	if it := find(got, "merge", 1); it == nil || it.Detail != "approved, checks ok" {
		t.Errorf("pr 1: %+v", it)
	}
	if it := find(got, "merge", 2); it == nil || it.Detail != "approved" {
		t.Errorf("pr 2: %+v", it)
	}
	if find(got, "merge", 3) != nil || find(got, "fix", 3) == nil {
		t.Error("approved with failing checks is a fix, not a merge")
	}
	if find(got, "merge", 4) != nil {
		t.Error("draft never merges")
	}
	if it := find(got, "fix", 5); it == nil || it.Detail != "conflict" {
		t.Errorf("approved but conflicting is a fix: %+v", it)
	}
	if find(got, "merge", 6) != nil {
		t.Error("other author's approval is not my merge")
	}
}

func TestFingerprintChangesWithState(t *testing.T) {
	a := Derive("me", repo(state.PR{Number: 1, Author: "me", ReviewDecision: "CHANGES_REQUESTED", HeadSHA: "aaa"}))
	b := Derive("me", repo(state.PR{Number: 1, Author: "me", ReviewDecision: "CHANGES_REQUESTED", HeadSHA: "bbb"}))
	c := Derive("me", repo(state.PR{Number: 1, Author: "me", ReviewDecision: "CHANGES_REQUESTED", HeadSHA: "aaa"}))
	if a[0].ID == b[0].ID {
		t.Fatal("new push must change the id")
	}
	if a[0].ID != c[0].ID {
		t.Fatal("same state must give the same id")
	}
	if a[0].ID[:len("member-websites#1:fix:")] != "member-websites#1:fix:" {
		t.Fatalf("id must be readable: %s", a[0].ID)
	}
}

func TestOrdering(t *testing.T) {
	old := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	newer := old.Add(48 * time.Hour)
	got := Derive("me", repo(
		state.PR{Number: 1, Author: "other", Requested: []string{"me"}, UpdatedAt: newer},
		state.PR{Number: 2, Author: "me", Checks: "fail", UpdatedAt: old},
		state.PR{Number: 3, Author: "me", ReviewDecision: "APPROVED", UpdatedAt: old},
		state.PR{Number: 4, Author: "me", Checks: "fail", UpdatedAt: newer},
	))
	want := []int{3, 4, 2, 1}
	for i, n := range want {
		if got[i].Number != n {
			t.Fatalf("order: got %v want %v", nums(got), want)
		}
	}
}

func TestEmptyIsSliceNotNil(t *testing.T) {
	if got := Derive("me", nil); got == nil {
		t.Fatal("must return [] for JSON")
	}
}

func nums(items []state.Item) []int {
	out := make([]int, len(items))
	for i, it := range items {
		out[i] = it.Number
	}
	return out
}
