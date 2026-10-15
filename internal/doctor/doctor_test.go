package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mathewepstein/devboard/internal/agent"
	"github.com/mathewepstein/devboard/internal/state"
)

var now = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

type fixture struct {
	opts      Options
	cfgPath   string
	polled    time.Time
	errs      state.Errors
	gqlDenied bool
}

func newFixture(t *testing.T, repos string, scope string) *fixture {
	t.Helper()
	f := &fixture{polled: now.Add(-30 * time.Second)}
	jiraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/myself":
			_, _ = w.Write([]byte(`{"accountId":"acct-me","displayName":"Me"}`))
		case "/rest/api/3/project/ACME":
			_, _ = w.Write([]byte(`{"key":"ACME","name":"Acme"}`))
		case "/rest/api/3/project/ACME/statuses":
			_, _ = w.Write([]byte(`[{"statuses":[{"name":"Open","statusCategory":{"key":"new"}},{"name":"Selected for Development","statusCategory":{"key":"new"}}]}]`))
		case "/rest/api/3/search/jql":
			if strings.Contains(r.URL.Query().Get("jql"), "bogus") {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"errorMessages":["Field 'bogus' does not exist"]}`))
				return
			}
			if strings.Contains(r.URL.Query().Get("jql"), "nomatch") {
				_, _ = w.Write([]byte(`{"issues":[],"isLast":true}`))
				return
			}
			_, _ = w.Write([]byte(`{"issues":[{"key":"ACME-1"}],"isLast":false}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(jiraSrv.Close)
	ghSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_, _ = w.Write([]byte(`{"login":"octocat"}`))
		case "/orgs/acme":
			_, _ = w.Write([]byte(`{"login":"acme"}`))
		case "/repos/acme/web":
			_, _ = w.Write([]byte(`{"name":"web","archived":false}`))
		case "/repos/acme/old":
			_, _ = w.Write([]byte(`{"name":"old","archived":true}`))
		case "/graphql":
			if f.gqlDenied {
				_, _ = w.Write([]byte(`{"data":{"r0":null},"errors":[{"path":["r0","pullRequests","nodes",0,"commits","nodes",0,"commit","statusCheckRollup"],"message":"Resource not accessible by personal access token"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"r0":{"pullRequests":{"nodes":[]}},"rateLimit":{"cost":2,"remaining":4990,"limit":5000,"resetAt":"2026-10-15T13:00:00Z"}}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(ghSrv.Close)
	stateSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := state.State{PolledAt: f.polled, Errors: f.errs}
		s.Budget.GitHub.Interval = "1m0s"
		_ = json.NewEncoder(w).Encode(s)
	}))
	t.Cleanup(stateSrv.Close)

	f.cfgPath = filepath.Join(t.TempDir(), "config.toml")
	cfg := `[jira]
base_url = "` + jiraSrv.URL + `"
email = "me@acme.com"
project = "ACME"
account_id = "acct-me"
picked_up_status = "Selected for Development"
[jira.scope]
` + scope + `
[github]
login = "octocat"
org = "acme"
` + repos
	if err := os.WriteFile(f.cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	f.opts = Options{
		ConfigPath:    f.cfgPath,
		Token:         func(string) (string, error) { return "tok", nil },
		Source:        func(string) string { return "keychain" },
		InKeychain:    func(string) bool { return true },
		GitHubREST:    ghSrv.URL,
		GitHubGraphQL: ghSrv.URL + "/graphql",
		Agent:         func() agent.Status { return agent.Status{Installed: true, Loaded: true, State: "running", PID: 42} },
		StateURL:      stateSrv.URL + "/api/state",
		Now:           func() time.Time { return now },
	}
	return f
}

const webRepo = "[[repos]]\nname = \"Web\"\nrepo = \"web\"\nprefix = \"W\"\n"

func byName(checks []Check) map[string]Check {
	m := map[string]Check{}
	for _, c := range checks {
		m[c.Name] = c
	}
	return m
}

func TestAllGreen(t *testing.T) {
	f := newFixture(t, webRepo, `labels = ["FE"]`)
	checks := Run(context.Background(), f.opts)
	for _, c := range checks {
		if c.Level != OK {
			t.Errorf("%s: %s %s", c.Name, c.Level, c.Detail)
		}
	}
	if Failed(checks) {
		t.Fatal("Failed() true on all-green")
	}
	for _, name := range []string{"config", "token jira", "token github", "jira account", "jira project", "jira scope", "jira picked-up status", "github login", "github org", "repo web", "github graphql", "agent", "server"} {
		if _, ok := byName(checks)[name]; !ok {
			t.Errorf("missing check %q", name)
		}
	}
}

func TestRemovedAndArchivedRepos(t *testing.T) {
	repos := webRepo + "[[repos]]\nname = \"Gone\"\nrepo = \"gone\"\n[[repos]]\nname = \"Old\"\nrepo = \"old\"\n"
	f := newFixture(t, repos, "")
	m := byName(Run(context.Background(), f.opts))
	if m["repo web"].Level != OK || m["repo gone"].Level != FAIL || m["repo old"].Level != FAIL {
		t.Fatalf("web %v gone %v old %v", m["repo web"], m["repo gone"], m["repo old"])
	}
	if !strings.Contains(m["repo old"].Detail, "archived") {
		t.Fatalf("archived detail: %q", m["repo old"].Detail)
	}
}

func TestGraphQLFieldDenied(t *testing.T) {
	f := newFixture(t, webRepo, "")
	f.gqlDenied = true
	c := byName(Run(context.Background(), f.opts))["github graphql"]
	if c.Level != FAIL || !strings.Contains(c.Detail, "web statusCheckRollup") || !strings.Contains(c.Detail, "token permissions") {
		t.Fatalf("%+v", c)
	}
}

func TestBrokenScopeJQL(t *testing.T) {
	f := newFixture(t, webRepo, `jql = "bogus = 1"`)
	m := byName(Run(context.Background(), f.opts))
	if c := m["jira scope"]; c.Level != FAIL || !strings.Contains(c.Detail, "bogus") {
		t.Fatalf("%+v", c)
	}
}

func TestScopeMatchingNothingWarns(t *testing.T) {
	f := newFixture(t, webRepo, `labels = ["nomatch"]`)
	m := byName(Run(context.Background(), f.opts))
	if c := m["jira scope"]; c.Level != WARN || !strings.Contains(c.Detail, "matches no issues") {
		t.Fatalf("%+v", c)
	}
}

func TestStalePollAndSourceErrors(t *testing.T) {
	f := newFixture(t, webRepo, "")
	f.polled = now.Add(-4 * time.Minute)
	if c := byName(Run(context.Background(), f.opts))["server"]; c.Level != FAIL || !strings.Contains(c.Detail, "4m") {
		t.Fatalf("stale: %+v", c)
	}
	f.polled = now.Add(-10 * time.Second)
	f.errs = state.Errors{Jira: "jira 401"}
	if c := byName(Run(context.Background(), f.opts))["server"]; c.Level != FAIL || !strings.Contains(c.Detail, "jira 401") {
		t.Fatalf("source error: %+v", c)
	}
}

func TestServerDown(t *testing.T) {
	f := newFixture(t, webRepo, "")
	f.opts.StateURL = "http://127.0.0.1:1/api/state"
	if c := byName(Run(context.Background(), f.opts))["server"]; c.Level != FAIL {
		t.Fatalf("%+v", c)
	}
}

func TestEnvOnlyTokenWarns(t *testing.T) {
	f := newFixture(t, webRepo, "")
	f.opts.Source = func(string) string { return "env:GH_TOKEN" }
	f.opts.InKeychain = func(string) bool { return false }
	c := byName(Run(context.Background(), f.opts))["token github"]
	if c.Level != WARN || !strings.Contains(c.Detail, "GH_TOKEN") {
		t.Fatalf("%+v", c)
	}
}

func TestMissingTokenSkipsSource(t *testing.T) {
	f := newFixture(t, webRepo, "")
	f.opts.Token = func(svc string) (string, error) {
		if svc == "jira" {
			return "", errors.New("token not found in keychain")
		}
		return "tok", nil
	}
	f.opts.Source = func(svc string) string {
		if svc == "jira" {
			return ""
		}
		return "keychain"
	}
	checks := Run(context.Background(), f.opts)
	m := byName(checks)
	if m["token jira"].Level != FAIL {
		t.Fatalf("%+v", m["token jira"])
	}
	if _, ran := m["jira account"]; ran {
		t.Fatal("jira checks must be skipped without a token")
	}
	if m["github org"].Level != OK {
		t.Fatal("github checks must still run")
	}
	if !Failed(checks) {
		t.Fatal("Failed() false with a FAIL")
	}
}

func TestAgentNotInstalled(t *testing.T) {
	f := newFixture(t, webRepo, "")
	f.opts.Agent = func() agent.Status { return agent.Status{} }
	if c := byName(Run(context.Background(), f.opts))["agent"]; c.Level != FAIL || !strings.Contains(c.Detail, "agent install") {
		t.Fatalf("%+v", c)
	}
}

func TestInvalidConfigStops(t *testing.T) {
	f := newFixture(t, "", "")
	checks := Run(context.Background(), f.opts)
	if len(checks) != 1 || checks[0].Name != "config" || checks[0].Level != FAIL {
		t.Fatalf("%+v", checks)
	}
}
