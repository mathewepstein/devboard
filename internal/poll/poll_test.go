package poll

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/github"
	"github.com/mathewepstein/devboard/internal/state"
)

type fakeGH struct {
	res github.Result
	err error
}

func (f *fakeGH) FetchPRs(context.Context, string, []string, string) (github.Result, error) {
	return f.res, f.err
}

func cfg() *config.Config {
	return &config.Config{
		Jira:   config.Jira{Project: "CD"},
		GitHub: config.GitHub{Login: "octocat", Org: "acme"},
		Repos:  []config.Repo{{Name: "MW", Repo: "member-websites", Prefix: "MW"}, {Name: "Hogo", Repo: "hogo"}},
		Poll:   config.Poll{Interval: config.Duration{Duration: time.Minute}},
	}
}

func TestOnceFillsState(t *testing.T) {
	gh := &fakeGH{res: github.Result{
		PRs:       map[string][]state.PR{"member-websites": {{Number: 1, Title: "a"}}},
		RateLimit: github.RateLimit{Cost: 10, Remaining: 4990, Limit: 5000},
	}}
	store := state.NewStore()
	p := New(cfg(), store, gh)
	p.Once(context.Background())

	s := store.Get()
	if s.PolledAt.IsZero() {
		t.Fatal("polledAt not set")
	}
	if len(s.Repos) != 2 || len(s.Repos[0].PRs) != 1 || s.Repos[0].PRs[0].Number != 1 {
		t.Fatalf("repos: %+v", s.Repos)
	}
	if s.Repos[1].PRs == nil {
		t.Fatal("repo with no PRs must have [] not nil")
	}
	if s.Budget.GitHub.Cost != 10 || s.Budget.GitHub.Interval != "1m0s" {
		t.Fatalf("budget: %+v", s.Budget.GitHub)
	}
	if s.Errors.GitHub != "" {
		t.Fatalf("unexpected error: %q", s.Errors.GitHub)
	}
}

func TestOnceKeepsLastGoodOnError(t *testing.T) {
	gh := &fakeGH{res: github.Result{
		PRs:       map[string][]state.PR{"member-websites": {{Number: 1}}},
		RateLimit: github.RateLimit{Cost: 10, Limit: 5000},
	}}
	store := state.NewStore()
	p := New(cfg(), store, gh)
	p.Once(context.Background())
	first := store.Get().PolledAt

	gh.err = errors.New("boom")
	p.Once(context.Background())
	s := store.Get()
	if len(s.Repos[0].PRs) != 1 {
		t.Fatal("last-good PRs lost on error")
	}
	if s.Errors.GitHub != "boom" {
		t.Fatalf("error not surfaced: %q", s.Errors.GitHub)
	}
	if !s.PolledAt.Equal(first) {
		t.Fatal("polledAt must not advance on a failed poll")
	}
}

func TestIntervalStretchesWithCost(t *testing.T) {
	gh := &fakeGH{res: github.Result{RateLimit: github.RateLimit{Cost: 200, Limit: 5000}}}
	p := New(cfg(), state.NewStore(), gh)
	p.Once(context.Background())
	if p.Interval() <= time.Minute {
		t.Fatalf("interval should stretch at 200 pts/min: %v", p.Interval())
	}
}

func TestRateLimitedBackoff(t *testing.T) {
	gh := &fakeGH{err: &github.RateLimitedError{RetryAfter: 90 * time.Second}}
	p := New(cfg(), state.NewStore(), gh)
	p.Once(context.Background())
	if p.Interval() < 90*time.Second {
		t.Fatalf("must honor Retry-After: %v", p.Interval())
	}
}
