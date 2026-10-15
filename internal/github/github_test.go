package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildQuery(t *testing.T) {
	q := BuildQuery([]string{"member-websites", "cd-widgets"})
	for _, want := range []string{
		`r0: repository(owner: $owner, name: "member-websites")`,
		`r1: repository(owner: $owner, name: "cd-widgets")`,
		"rateLimit { cost remaining limit resetAt }",
		"pullRequests(states: OPEN, first: 30",
		"fragment PRs on Repository",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query missing %q\n%s", want, q)
		}
	}
}

func TestParseResponse(t *testing.T) {
	body, err := os.ReadFile("testdata/prs.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := parseResponse(body, []string{"member-websites", "cd-widgets"}, "CD")
	if err != nil {
		t.Fatal(err)
	}
	if res.RateLimit.Cost != 10 || res.RateLimit.Remaining != 4987 || res.RateLimit.Limit != 5000 {
		t.Fatalf("rate limit: %+v", res.RateLimit)
	}
	if len(res.PRs["cd-widgets"]) != 0 {
		t.Fatalf("cd-widgets should be empty, got %d", len(res.PRs["cd-widgets"]))
	}
	prs := res.PRs["member-websites"]
	if len(prs) != 3 {
		t.Fatalf("want 3 PRs, got %d", len(prs))
	}

	p := prs[0]
	if p.Number != 378 || p.Author != "octocat" || p.ReviewDecision != "CHANGES_REQUESTED" || p.Checks != "fail" {
		t.Errorf("pr 378: %+v", p)
	}
	if !reflect.DeepEqual(p.TicketKeys, []string{"CD-26811"}) {
		t.Errorf("pr 378 keys: %v", p.TicketKeys)
	}
	if !reflect.DeepEqual(p.Reviewers, []string{"jdoe:CHANGES_REQUESTED"}) {
		t.Errorf("pr 378 reviewers: %v", p.Reviewers)
	}

	p = prs[1]
	if p.Mergeable != "CONFLICTING" || p.Checks != "ok" {
		t.Errorf("pr 366: %+v", p)
	}
	if !reflect.DeepEqual(p.Requested, []string{"octocat", "frontend"}) {
		t.Errorf("pr 366 requested: %v", p.Requested)
	}
	if !reflect.DeepEqual(p.TicketKeys, []string{"CD-26700"}) {
		t.Errorf("pr 366 keys from branch: %v", p.TicketKeys)
	}

	p = prs[2]
	if !p.Draft || p.ReviewDecision != "" || p.Checks != "none" || len(p.TicketKeys) != 0 {
		t.Errorf("pr 380: %+v", p)
	}
}

func TestParseErrors(t *testing.T) {
	body := []byte(`{"errors":[{"message":"Could not resolve to a Repository with the name 'acme/nope'."}],"data":{"r0":null,"rateLimit":{"cost":1,"remaining":4999,"limit":5000,"resetAt":"2026-10-15T15:00:00Z"}}}`)
	_, err := parseResponse(body, []string{"nope"}, "CD")
	if err == nil || !strings.Contains(err.Error(), "acme/nope") {
		t.Fatalf("want graphql error surfaced, got %v", err)
	}
}

func TestTicketKeys(t *testing.T) {
	got := TicketKeys("CD", "CD-1 and cd-2 again CD-1", "feature/CD-3-thing")
	if !reflect.DeepEqual(got, []string{"CD-1", "CD-2", "CD-3"}) {
		t.Fatalf("got %v", got)
	}
	if got := TicketKeys("CD", "ABCD-9 nothing", "x"); len(got) != 0 {
		t.Fatalf("prefix must be word-bounded, got %v", got)
	}
}

func TestFetchPRs(t *testing.T) {
	body, _ := os.ReadFile("testdata/prs.json")
	var gotAuth, gotOwner string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Variables map[string]string `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotOwner = req.Variables["owner"]
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := NewClient("tok")
	c.Endpoint = srv.URL
	res, err := c.FetchPRs(context.Background(), "acme", []string{"member-websites", "cd-widgets"}, "CD")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tok" || gotOwner != "acme" {
		t.Fatalf("auth %q owner %q", gotAuth, gotOwner)
	}
	if len(res.PRs["member-websites"]) != 3 {
		t.Fatalf("prs: %d", len(res.PRs["member-websites"]))
	}
}

func TestFetchRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient("tok")
	c.Endpoint = srv.URL
	_, err := c.FetchPRs(context.Background(), "acme", []string{"x"}, "CD")
	var rl *RateLimitedError
	if !asRateLimited(err, &rl) || rl.RetryAfter != 7*time.Second {
		t.Fatalf("want RateLimitedError 7s, got %v", err)
	}
}

func TestStretchInterval(t *testing.T) {
	base := 60 * time.Second
	if got := StretchInterval(base, 10, 5000); got != base {
		t.Fatalf("10 pts/min is 12%% of budget, must not stretch: %v", got)
	}
	got := StretchInterval(base, 100, 5000)
	if got <= base {
		t.Fatalf("100 pts/min is 120%%, must stretch: %v", got)
	}
	if projected := float64(100) * 3600 / got.Seconds(); projected > 0.6*5000+1 {
		t.Fatalf("stretched interval still over 60%%: %v → %.0f pts/h", got, projected)
	}
	if got := StretchInterval(base, 0, 0); got != base {
		t.Fatalf("zero limit must fall back to base: %v", got)
	}
}
