package poll

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/github"
	"github.com/mathewepstein/devboard/internal/items"
	"github.com/mathewepstein/devboard/internal/state"
)

type GitHubSource interface {
	FetchPRs(ctx context.Context, org string, repos []string, project string) (github.Result, error)
}

type Poller struct {
	cfg   *config.Config
	store *state.Store
	gh    GitHubSource

	mu       sync.Mutex
	interval time.Duration
}

func New(cfg *config.Config, store *state.Store, gh GitHubSource) *Poller {
	if store.Get().Repos == nil {
		store.Set(state.Seed(cfg))
	}
	return &Poller{cfg: cfg, store: store, gh: gh, interval: cfg.Poll.Interval.Duration}
}

func (p *Poller) Interval() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interval
}

func (p *Poller) setInterval(d time.Duration) {
	p.mu.Lock()
	p.interval = d
	p.mu.Unlock()
}

func (p *Poller) Run(ctx context.Context) {
	p.Once(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.Interval()):
			p.Once(ctx)
		}
	}
}

func (p *Poller) Once(ctx context.Context) {
	repos := make([]string, len(p.cfg.Repos))
	for i, r := range p.cfg.Repos {
		repos[i] = r.Repo
	}
	res, err := p.gh.FetchPRs(ctx, p.cfg.GitHub.Org, repos, p.cfg.Jira.Project)

	s := p.store.Get()
	if err != nil {
		s.Errors.GitHub = err.Error()
		var rl *github.RateLimitedError
		if errors.As(err, &rl) {
			p.setInterval(rl.RetryAfter + jitter(rl.RetryAfter))
		}
		p.store.Set(s)
		return
	}

	for i := range s.Repos {
		prs := res.PRs[s.Repos[i].GHRepo]
		if prs == nil {
			prs = []state.PR{}
		}
		s.Repos[i].PRs = prs
	}
	s.Items = items.Derive(p.cfg.GitHub.Login, s.Repos)
	next := github.StretchInterval(p.cfg.Poll.Interval.Duration, res.RateLimit.Cost, res.RateLimit.Limit)
	p.setInterval(next)
	s.Budget.GitHub = state.GitHubBudget{
		Cost:      res.RateLimit.Cost,
		Remaining: res.RateLimit.Remaining,
		ResetAt:   res.RateLimit.ResetAt,
		Interval:  next.String(),
	}
	s.Errors.GitHub = ""
	s.PolledAt = time.Now()
	p.store.Set(s)
}

func jitter(d time.Duration) time.Duration {
	return time.Duration(rand.Int64N(int64(d/10) + 1))
}
