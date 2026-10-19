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
	"github.com/mathewepstein/devboard/internal/jira"
	"github.com/mathewepstein/devboard/internal/seen"
	"github.com/mathewepstein/devboard/internal/state"
)

const versionsTTL = 5 * time.Minute

type GitHubSource interface {
	FetchPRs(ctx context.Context, org string, repos []string, project string) (github.Result, error)
}

type JiraSource interface {
	Versions(ctx context.Context, project string) ([]jira.Version, error)
	VersionTickets(ctx context.Context, version, scope string) ([]state.Ticket, error)
	Selected(ctx context.Context, project, scope, status string) ([]state.Selected, error)
	Assigned(ctx context.Context, project, accountID string) ([]state.Assigned, error)
	Mentions(ctx context.Context, project, accountID, displayName string) ([]state.Mention, error)
}

type Poller struct {
	cfg   *config.Config
	store *state.Store
	gh    GitHubSource
	Jira  JiraSource
	Seen  *seen.Store

	mu         sync.Mutex
	interval   time.Duration
	versionsAt time.Time
	versions   []jira.Version
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

type jiraResult struct {
	versions []jira.Version
	tickets  map[string][]state.Ticket
	selected []state.Selected
	assigned []state.Assigned
	mentions []state.Mention
}

func (p *Poller) Once(ctx context.Context) {
	var (
		ghRes github.Result
		ghErr error
		jr    jiraResult
		jrErr error
		wg    sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ghRes, ghErr = p.fetchGitHub(ctx)
	}()
	if p.Jira != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			jr, jrErr = p.fetchJira(ctx)
		}()
	}
	wg.Wait()

	s := p.store.Get()
	ok := false

	if ghErr != nil {
		s.Errors.GitHub = ghErr.Error()
		var rl *github.RateLimitedError
		if errors.As(ghErr, &rl) {
			p.setInterval(rl.RetryAfter + jitter(rl.RetryAfter))
		}
	} else {
		ok = true
		s.Errors.GitHub = ""
		for i := range s.Repos {
			prs := ghRes.PRs[s.Repos[i].GHRepo]
			if prs == nil {
				prs = []state.PR{}
			}
			s.Repos[i].PRs = prs
		}
		s.Items = items.Derive(p.cfg.GitHub.Login, s.Repos)
		next := github.StretchInterval(p.cfg.Poll.Interval.Duration, ghRes.RateLimit.Cost, ghRes.RateLimit.Limit)
		p.setInterval(next)
		s.Budget.GitHub = state.GitHubBudget{
			Cost: ghRes.RateLimit.Cost, Remaining: ghRes.RateLimit.Remaining,
			ResetAt: ghRes.RateLimit.ResetAt, Interval: next.String(),
		}
	}

	if p.Jira != nil {
		if jrErr != nil {
			s.Errors.Jira = jrErr.Error()
		} else {
			ok = true
			s.Errors.Jira = ""
			for i := range s.Repos {
				r := &s.Repos[i]
				live, next, queued := jira.MatchVersions(jr.versions, r.Prefix)
				r.Live, r.Next, r.Queued = toVersion(live, jr.tickets), toVersion(next, jr.tickets), []string{}
				for _, q := range queued {
					r.Queued = append(r.Queued, q.Name)
				}
			}
			s.Selected, s.Assigned, s.Mentions = jr.selected, jr.assigned, jr.mentions
		}
	}

	if ok {
		s.PolledAt = time.Now()
	}
	p.store.Set(s)
	if ok && p.Seen != nil && ghErr == nil && jrErr == nil {
		_ = p.Seen.Prune(s.IDs())
	}
}

func (p *Poller) fetchGitHub(ctx context.Context) (github.Result, error) {
	repos := make([]string, len(p.cfg.Repos))
	for i, r := range p.cfg.Repos {
		repos[i] = r.Repo
	}
	return p.gh.FetchPRs(ctx, p.cfg.GitHub.Org, repos, p.cfg.Jira.Project)
}

func (p *Poller) fetchJira(ctx context.Context) (jiraResult, error) {
	j := p.cfg.Jira
	scope := j.Scope.Clause()
	var res jiraResult

	p.mu.Lock()
	cached, fresh := p.versions, time.Since(p.versionsAt) < versionsTTL
	p.mu.Unlock()
	if fresh {
		res.versions = cached
	} else {
		vs, err := p.Jira.Versions(ctx, j.Project)
		if err != nil {
			return res, err
		}
		res.versions = vs
	}

	res.tickets = map[string][]state.Ticket{}
	for _, r := range p.cfg.Repos {
		live, next, _ := jira.MatchVersions(res.versions, r.Prefix)
		for _, v := range []*jira.Version{live, next} {
			if v == nil {
				continue
			}
			if _, done := res.tickets[v.Name]; done {
				continue
			}
			tk, err := p.Jira.VersionTickets(ctx, v.Name, scope)
			if err != nil {
				return res, err
			}
			res.tickets[v.Name] = tk
		}
	}

	var err error
	if res.selected, err = p.Jira.Selected(ctx, j.Project, scope, j.PickedUpStatus); err != nil {
		return res, err
	}
	if res.assigned, err = p.Jira.Assigned(ctx, j.Project, j.AccountID); err != nil {
		return res, err
	}
	if res.mentions, err = p.Jira.Mentions(ctx, j.Project, j.AccountID, j.DisplayName); err != nil {
		return res, err
	}

	if !fresh {
		p.mu.Lock()
		p.versions, p.versionsAt = res.versions, time.Now()
		p.mu.Unlock()
	}
	return res, nil
}

func toVersion(v *jira.Version, tickets map[string][]state.Ticket) *state.Version {
	if v == nil {
		return nil
	}
	out := &state.Version{Name: v.Name, ReleaseDate: v.ReleaseDate}
	if tickets != nil {
		out.Tickets = tickets[v.Name]
		if out.Tickets == nil {
			out.Tickets = []state.Ticket{}
		}
	}
	return out
}

func jitter(d time.Duration) time.Duration {
	return time.Duration(rand.Int64N(int64(d/10) + 1))
}
