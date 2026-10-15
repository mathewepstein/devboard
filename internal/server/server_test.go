package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/seen"
	"github.com/mathewepstein/devboard/internal/state"
)

func localGet(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "127.0.0.1:8765"
	return req
}

func localPost(path, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Content-Type", "application/json")
	return req
}

func testSeen(t *testing.T) *seen.Store {
	t.Helper()
	s, err := seen.Open(filepath.Join(t.TempDir(), "seen.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAckMarksSeen(t *testing.T) {
	store := state.NewStore()
	st := state.Seed(testConfig())
	st.Items = []state.Item{{ID: "member-websites#1:review:abcd", Rule: "review", Number: 1}}
	st.Mentions = []state.Mention{{ID: "CD-9:123", Key: "CD-9"}}
	store.Set(st)
	h, err := New(testConfig(), store, testSeen(t))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, localGet("/api/state"))
	var before state.State
	_ = json.Unmarshal(rr.Body.Bytes(), &before)
	if before.Items[0].Seen || before.Mentions[0].Seen {
		t.Fatal("nothing should be seen yet")
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, localPost("/api/ack", `{"ids":["member-websites#1:review:abcd"]}`))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("ack status %d: %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, localGet("/api/state"))
	var after state.State
	_ = json.Unmarshal(rr.Body.Bytes(), &after)
	if !after.Items[0].Seen || after.Mentions[0].Seen {
		t.Fatalf("seen flags wrong: %+v %+v", after.Items[0], after.Mentions[0])
	}
}

func TestAckRejectsBadBody(t *testing.T) {
	h, _ := New(testConfig(), state.NewStore(), testSeen(t))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, localPost("/api/ack", `nope`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
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
	h, err := New(testConfig(), state.NewStore(), testSeen(t))
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
	h, err := New(testConfig(), state.NewStore(), testSeen(t))
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
	h, err := New(testConfig(), state.NewStore(), testSeen(t))
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
	h, err := New(testConfig(), state.NewStore(), testSeen(t))
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
