package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sample() *Config {
	return &Config{
		Jira: Jira{
			BaseURL:        "https://acme.atlassian.net",
			Email:          "me@acme.com",
			Project:        "ACME",
			AccountID:      "acct-1",
			PickedUpStatus: "Selected for Development",
			Scope:          Scope{Labels: []string{"FE"}},
		},
		GitHub: GitHub{Login: "octocat", Org: "acme"},
		Repos: []Repo{
			{Name: "Member Websites", Repo: "member-websites", Prefix: "MW"},
			{Name: "Hogo", Repo: "hogo-website"},
		},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	in := sample()
	if err := in.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	out, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Jira.Project != "ACME" || out.GitHub.Login != "octocat" {
		t.Fatalf("identity lost: %+v", out)
	}
	if len(out.Repos) != 2 || out.Repos[0].Prefix != "MW" || out.Repos[1].Prefix != "" {
		t.Fatalf("repos lost: %+v", out.Repos)
	}
	if len(out.Jira.Scope.Labels) != 1 || out.Jira.Scope.Labels[0] != "FE" {
		t.Fatalf("scope lost: %+v", out.Jira.Scope)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := sample().SaveTo(path); err != nil {
		t.Fatal(err)
	}
	out, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Poll.Interval.Duration != 60*time.Second {
		t.Fatalf("interval default: %v", out.Poll.Interval)
	}
	if out.Poll.Port != 8765 {
		t.Fatalf("port default: %d", out.Poll.Port)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := LoadFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if err == nil || !strings.Contains(err.Error(), "devboard init") {
		t.Fatalf("want init hint, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	ok := sample()
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	noRepos := sample()
	noRepos.Repos = nil
	if err := noRepos.Validate(); err == nil {
		t.Fatal("no repos accepted")
	}

	twoScopes := sample()
	twoScopes.Jira.Scope.BoardID = 357
	if err := twoScopes.Validate(); err == nil {
		t.Fatal("two scope kinds accepted")
	}

	noScope := sample()
	noScope.Jira.Scope = Scope{}
	if err := noScope.Validate(); err != nil {
		t.Fatalf("empty scope should mean whole project: %v", err)
	}
}

func TestScopeClause(t *testing.T) {
	cases := []struct {
		s    Scope
		want string
	}{
		{Scope{}, ""},
		{Scope{Labels: []string{"FE"}}, `labels IN ("FE")`},
		{Scope{Labels: []string{"FE", "Web"}}, `labels IN ("FE", "Web")`},
		{Scope{BoardID: 357, JQL: "labels = FE AND type != Epic"}, "(labels = FE AND type != Epic)"},
		{Scope{JQL: "component = web"}, "(component = web)"},
	}
	for _, c := range cases {
		if got := c.s.Clause(); got != c.want {
			t.Errorf("%+v: got %q want %q", c.s, got, c.want)
		}
	}
}

func TestTemplateParses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := writeFile(path, Template()); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(path); err != nil {
		t.Fatalf("template does not parse: %v", err)
	}
}
