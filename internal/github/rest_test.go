package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestViewer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"login":"octocat","id":1}`))
	}))
	defer srv.Close()
	c := NewClient("tok")
	c.RESTEndpoint = srv.URL
	login, err := c.Viewer(context.Background())
	if err != nil || login != "octocat" {
		t.Fatalf("%q %v", login, err)
	}
}

func TestOrgReposPagedSkipsArchived(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/acme/repos" {
			w.WriteHeader(404)
			return
		}
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", `<`+"http://x/orgs/acme/repos?page=2"+`>; rel="next"`)
			_, _ = w.Write([]byte(`[{"name":"b-repo","pushed_at":"2026-10-14T00:00:00Z","archived":false},{"name":"old","pushed_at":"2024-01-01T00:00:00Z","archived":true}]`))
		case "2":
			_, _ = w.Write([]byte(`[{"name":"a-repo","pushed_at":"2026-10-15T00:00:00Z","archived":false}]`))
		}
	}))
	defer srv.Close()
	c := NewClient("tok")
	c.RESTEndpoint = srv.URL
	repos, err := c.OrgRepos(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0].Name != "a-repo" || repos[1].Name != "b-repo" {
		t.Fatalf("want two live repos newest-pushed first, got %+v", repos)
	}
}

func TestVerifyOrg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/orgs/acme" {
			_, _ = w.Write([]byte(`{"login":"acme"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	c := NewClient("tok")
	c.RESTEndpoint = srv.URL
	if err := c.VerifyOrg(context.Background(), "acme"); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyOrg(context.Background(), "nope"); err == nil {
		t.Fatal("unknown org must error")
	}
}

func TestRepoInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/live":
			_, _ = w.Write([]byte(`{"name":"live","archived":false}`))
		case "/repos/acme/old":
			_, _ = w.Write([]byte(`{"name":"old","archived":true}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewClient("tok")
	c.RESTEndpoint = srv.URL
	if r, err := c.RepoInfo(context.Background(), "acme", "live"); err != nil || r.Archived {
		t.Fatalf("%+v %v", r, err)
	}
	if r, err := c.RepoInfo(context.Background(), "acme", "old"); err != nil || !r.Archived {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := c.RepoInfo(context.Background(), "acme", "gone"); err == nil {
		t.Fatal("missing repo must error")
	}
}
