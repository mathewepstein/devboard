// Package doctor checks config, tokens, APIs, the agent, and the live server.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mathewepstein/devboard/internal/agent"
	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/github"
	"github.com/mathewepstein/devboard/internal/jira"
	"github.com/mathewepstein/devboard/internal/state"
)

type Level string

const (
	OK   Level = "ok"
	WARN Level = "warn"
	FAIL Level = "fail"
)

type Check struct {
	Name   string
	Level  Level
	Detail string
}

type Options struct {
	ConfigPath string
	Token      func(service string) (string, error)
	Source     func(service string) string
	InKeychain func(service string) bool
	GitHubREST string
	Agent      func() agent.Status
	StateURL   string
	Now        func() time.Time
}

func Failed(checks []Check) bool {
	for _, c := range checks {
		if c.Level == FAIL {
			return true
		}
	}
	return false
}

func Run(ctx context.Context, o Options) []Check {
	var out []Check
	add := func(name string, lvl Level, format string, a ...any) Check {
		c := Check{Name: name, Level: lvl, Detail: fmt.Sprintf(format, a...)}
		out = append(out, c)
		return c
	}
	pass := func(name, format string, a ...any) { add(name, OK, format, a...) }
	fail := func(name string, err error) Check { return add(name, FAIL, "%v", err) }

	cfg, err := config.LoadFrom(o.ConfigPath)
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		fail("config", err)
		return out
	}
	pass("config", "%s", o.ConfigPath)

	jiraTok := token(o, "jira", add)
	ghTok := token(o, "github", add)

	if jiraTok != "" {
		jiraChecks(ctx, cfg, jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.Email, jiraTok), pass, add, fail)
	}
	if ghTok != "" {
		gc := github.NewClient(ghTok)
		if o.GitHubREST != "" {
			gc.RESTEndpoint = o.GitHubREST
		}
		githubChecks(ctx, cfg, gc, pass, add, fail)
	}

	a := o.Agent()
	switch {
	case !a.Installed:
		add("agent", FAIL, "not installed; run `devboard agent install`")
	case !a.Loaded:
		add("agent", FAIL, "plist present but not loaded; run `devboard agent install`")
	case a.State != "running":
		add("agent", FAIL, "loaded, state %q; see %s", a.State, "~/Library/Logs/devboard/devboard.log")
	default:
		pass("agent", "running, pid %d", a.PID)
	}

	out = append(out, ServerCheck(ctx, o, cfg))
	return out
}

func token(o Options, svc string, add func(string, Level, string, ...any) Check) string {
	name := "token " + svc
	tok, err := o.Token(svc)
	if err != nil || tok == "" {
		add(name, FAIL, "none in env or keychain; run `devboard init`")
		return ""
	}
	src := o.Source(svc)
	if strings.HasPrefix(src, "env:") && !o.InKeychain(svc) {
		add(name, WARN, "%s only; the agent has no shell env, run `devboard init` to store it in the keychain", strings.TrimPrefix(src, "env:"))
	} else {
		add(name, OK, "%s", src)
	}
	return tok
}

type addFn = func(string, Level, string, ...any) Check

func jiraChecks(ctx context.Context, cfg *config.Config, jc *jira.Client, pass func(string, string, ...any), add addFn, fail func(string, error) Check) {
	me, err := jc.Myself(ctx)
	if err != nil {
		fail("jira account", err)
		return
	}
	pass("jira account", "%s", me.DisplayName)
	name, err := jc.VerifyProject(ctx, cfg.Jira.Project)
	if err != nil {
		fail("jira project", err)
		return
	}
	pass("jira project", "%s (%s)", cfg.Jira.Project, name)
	if clause := cfg.Jira.Scope.Clause(); clause == "" {
		pass("jira scope", "whole project")
	} else if err := jc.ValidateJQL(ctx, "project = "+cfg.Jira.Project+" AND "+clause); err != nil {
		fail("jira scope", fmt.Errorf("%s rejected: %w", clause, err))
	} else {
		pass("jira scope", "%s", clause)
	}
	if cfg.Jira.PickedUpStatus == "" {
		add("jira picked-up status", WARN, "jira.picked_up_status not set")
		return
	}
	statuses, err := jc.Statuses(ctx, cfg.Jira.Project)
	if err != nil {
		fail("jira picked-up status", err)
		return
	}
	for _, s := range statuses {
		if s.Name == cfg.Jira.PickedUpStatus {
			pass("jira picked-up status", "%s", s.Name)
			return
		}
	}
	add("jira picked-up status", FAIL, "%q is not a status in %s", cfg.Jira.PickedUpStatus, cfg.Jira.Project)
}

func githubChecks(ctx context.Context, cfg *config.Config, gc *github.Client, pass func(string, string, ...any), add addFn, fail func(string, error) Check) {
	login, err := gc.Viewer(ctx)
	if err != nil {
		fail("github login", err)
		return
	}
	if cfg.GitHub.Login != "" && !strings.EqualFold(login, cfg.GitHub.Login) {
		add("github login", WARN, "token is %s, config tracks %s", login, cfg.GitHub.Login)
	} else {
		pass("github login", "%s", login)
	}
	if err := gc.VerifyOrg(ctx, cfg.GitHub.Org); err != nil {
		fail("github org", err)
		return
	}
	pass("github org", "%s", cfg.GitHub.Org)
	for _, r := range cfg.Repos {
		name := "repo " + r.Repo
		info, err := gc.RepoInfo(ctx, cfg.GitHub.Org, r.Repo)
		switch {
		case err != nil:
			fail(name, err)
		case info.Archived:
			add(name, FAIL, "%s/%s is archived", cfg.GitHub.Org, r.Repo)
		default:
			pass(name, "%s/%s", cfg.GitHub.Org, r.Repo)
		}
	}
}

func ServerCheck(ctx context.Context, o Options, cfg *config.Config) Check {
	add := func(_ string, lvl Level, format string, a ...any) Check {
		return Check{Name: "server", Level: lvl, Detail: fmt.Sprintf(format, a...)}
	}
	url := o.StateURL
	if url == "" {
		url = fmt.Sprintf("http://127.0.0.1:%d/api/state", cfg.Poll.Port)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return add("server", FAIL, "%s unreachable: %v", url, err)
	}
	defer resp.Body.Close()
	var s state.State
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return add("server", FAIL, "parse %s: %v", url, err)
	}
	if s.PolledAt.IsZero() {
		return add("server", WARN, "up, no poll completed yet")
	}
	interval := cfg.Poll.Interval.Duration
	if d, err := time.ParseDuration(s.Budget.GitHub.Interval); err == nil && d > 0 {
		interval = d
	}
	age := o.Now().Sub(s.PolledAt).Round(time.Second)
	var problems []string
	if age > 3*interval {
		problems = append(problems, fmt.Sprintf("last poll %s ago (interval %s)", age, interval))
	}
	if s.Errors.GitHub != "" {
		problems = append(problems, "github: "+s.Errors.GitHub)
	}
	if s.Errors.Jira != "" {
		problems = append(problems, "jira: "+s.Errors.Jira)
	}
	if len(problems) > 0 {
		return add("server", FAIL, "%s", strings.Join(problems, "; "))
	}
	return add("server", OK, "last poll %s ago", age)
}
