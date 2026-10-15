package poll

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/github"
	"github.com/mathewepstein/devboard/internal/jira"
	"github.com/mathewepstein/devboard/internal/state"
)

type fakeGH struct {
	res github.Result
	err error
}

func (f *fakeGH) FetchPRs(context.Context, string, []string, string) (github.Result, error) {
	return f.res, f.err
}

type fakeJira struct {
	versionsCalls int
	err           error
}

func (f *fakeJira) Versions(context.Context, string) ([]jira.Version, error) {
	f.versionsCalls++
	return []jira.Version{
		{Name: "MW 1.10.0", Released: true, ReleaseDate: "2026-10-02"},
		{Name: "MW 1.11.0"},
		{Name: "MW 1.12.0"},
	}, f.err
}

func (f *fakeJira) VersionTickets(_ context.Context, version, _ string) ([]state.Ticket, error) {
	return []state.Ticket{{Key: "CD-1", Status: "Done", Category: "done"}}, nil
}

func (f *fakeJira) Selected(context.Context, string, string, string) ([]state.Selected, error) {
	return []state.Selected{{Key: "CD-7"}}, nil
}

func (f *fakeJira) Assigned(context.Context, string, string) ([]state.Assigned, error) {
	return []state.Assigned{{Key: "CD-8"}}, nil
}

func (f *fakeJira) Mentions(context.Context, string, string, string) ([]state.Mention, error) {
	return []state.Mention{{Key: "CD-9"}}, nil
}

func TestOnceFillsJira(t *testing.T) {
	gh := &fakeGH{res: github.Result{RateLimit: github.RateLimit{Cost: 7, Limit: 5000}}}
	jr := &fakeJira{}
	store := state.NewStore()
	p := New(cfg(), store, gh)
	p.Jira = jr
	p.Once(context.Background())
	s := store.Get()
	mw := s.Repos[0]
	if mw.Live == nil || mw.Live.Name != "MW 1.10.0" || mw.Next == nil || mw.Next.Name != "MW 1.11.0" {
		t.Fatalf("versions: live=%+v next=%+v", mw.Live, mw.Next)
	}
	if len(mw.Queued) != 1 || mw.Queued[0] != "MW 1.12.0" {
		t.Fatalf("queued: %v", mw.Queued)
	}
	if len(mw.Next.Tickets) != 1 || mw.Next.Tickets[0].Key != "CD-1" {
		t.Fatalf("tickets: %+v", mw.Next.Tickets)
	}
	if s.Repos[1].Live != nil || s.Repos[1].Next != nil {
		t.Fatal("repo without prefix must have no versions")
	}
	if len(s.Selected) != 1 || len(s.Assigned) != 1 || len(s.Mentions) != 1 {
		t.Fatalf("panels: %d %d %d", len(s.Selected), len(s.Assigned), len(s.Mentions))
	}
	if s.Errors.Jira != "" {
		t.Fatalf("jira error: %q", s.Errors.Jira)
	}

	p.Once(context.Background())
	if jr.versionsCalls != 1 {
		t.Fatalf("versions must be cached within TTL, got %d calls", jr.versionsCalls)
	}
}

func TestJiraErrorKeepsLastGood(t *testing.T) {
	gh := &fakeGH{res: github.Result{RateLimit: github.RateLimit{Cost: 7, Limit: 5000}}}
	jr := &fakeJira{}
	store := state.NewStore()
	p := New(cfg(), store, gh)
	p.Jira = jr
	p.Once(context.Background())
	jr.err = errors.New("jira down")
	p.versionsAt = time.Time{}
	p.Once(context.Background())
	s := store.Get()
	if s.Errors.Jira != "jira down" {
		t.Fatalf("error not surfaced: %q", s.Errors.Jira)
	}
	if s.Repos[0].Live == nil || len(s.Mentions) != 1 {
		t.Fatal("last-good Jira data lost")
	}
	if s.Errors.GitHub != "" || s.PolledAt.IsZero() {
		t.Fatal("a Jira failure must not block the GitHub half")
	}
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

func TestOnceDerivesItems(t *testing.T) {
	gh := &fakeGH{res: github.Result{
		PRs: map[string][]state.PR{"member-websites": {
			{Number: 7, Author: "other", Requested: []string{"octocat"}},
			{Number: 8, Author: "octocat", ReviewDecision: "APPROVED", Checks: "ok"},
		}},
		RateLimit: github.RateLimit{Cost: 10, Limit: 5000},
	}}
	store := state.NewStore()
	New(cfg(), store, gh).Once(context.Background())
	items := store.Get().Items
	if len(items) != 2 || items[0].Rule != "merge" || items[1].Rule != "review" {
		t.Fatalf("items: %+v", items)
	}
	if items[0].Repo != "member-websites" {
		t.Fatalf("repo not carried: %+v", items[0])
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
