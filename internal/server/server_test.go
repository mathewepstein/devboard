package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/state"
)

func localGet(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "127.0.0.1:8765"
	return req
}

func testConfig() *config.Config {
	return &config.Config{
		Jira:   config.Jira{Project: "ACME"},
		GitHub: config.GitHub{Login: "octocat", Org: "acme"},
		Repos: []config.Repo{
			{Name: "Member Websites", Repo: "member-websites", Prefix: "MW"},
			{Name: "Hogo", Repo: "hogo-website"},
		},
	}
}

func TestStateEndpoint(t *testing.T) {
	h, err := New(testConfig(), state.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, localGet("/api/state"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %q", ct)
	}
	var s state.State
	if err := json.Unmarshal(rr.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Repos) != 2 || s.Repos[0].Name != "Member Websites" || s.Repos[1].Prefix != "" {
		t.Fatalf("repos not seeded from config: %+v", s.Repos)
	}
	if s.Repos[0].PRs == nil || s.Items == nil {
		t.Fatal("slices must encode as [] not null")
	}
}

func TestIndexServed(t *testing.T) {
	h, err := New(testConfig(), state.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, localGet("/"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestAssetsServed(t *testing.T) {
	h, err := New(testConfig(), state.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	for path, ct := range map[string]string{"/app.js": "text/javascript", "/app.css": "text/css"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, localGet(path))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status %d", path, rr.Code)
		}
		if got := rr.Header().Get("Content-Type"); len(got) < len(ct) || got[:len(ct)] != ct {
			t.Fatalf("%s content-type %q", path, got)
		}
	}
}

func TestNonLocalhostRejected(t *testing.T) {
	h, err := New(testConfig(), state.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Host = "evil.example:8765"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status %d", rr.Code)
	}
}
