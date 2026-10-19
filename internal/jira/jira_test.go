package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const me = "acct-me"

func ts(d time.Duration) string {
	return time.Now().Add(d).Format("2006-01-02T15:04:05.000-0700")
}

// routes: substring of path or decoded jql → body
func newServer(t *testing.T, routes map[string]string) (*Client, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if u, p, ok := r.BasicAuth(); !ok || u != "me@acme.com" || p != "tok" {
			w.WriteHeader(401)
			return
		}
		q, _ := url.QueryUnescape(r.URL.RawQuery)
		key := r.URL.Path + "?" + q
		for sub, body := range routes {
			if strings.Contains(key, sub) {
				_, _ = w.Write([]byte(body))
				return
			}
		}
		t.Errorf("unrouted request: %s", key)
		w.WriteHeader(500)
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "me@acme.com", "tok"), &calls
}

func TestMatchVersions(t *testing.T) {
	vs := []Version{
		{Name: "MW 1.9.0", Released: true, ReleaseDate: "2026-09-01"},
		{Name: "MW 1.10.0", Released: true, ReleaseDate: "2026-10-02"},
		{Name: "MW 1.12.0", Released: false},
		{Name: "MW 1.11.0", Released: false},
		{Name: "MW 2.0.0", Released: false},
		{Name: "MWX 1.0.0", Released: false},
		{Name: "CDW 1.29.0", Released: false, ReleaseDate: "2026-10-19"},
	}
	live, next, queued := MatchVersions(vs, "MW")
	if live == nil || live.Name != "MW 1.10.0" {
		t.Fatalf("live: %+v", live)
	}
	if next == nil || next.Name != "MW 1.11.0" {
		t.Fatalf("next: %+v", next)
	}
	if len(queued) != 2 || queued[0].Name != "MW 1.12.0" || queued[1].Name != "MW 2.0.0" {
		t.Fatalf("queued: %+v", queued)
	}
	if l, n, q := MatchVersions(vs, ""); l != nil || n != nil || len(q) != 0 {
		t.Fatal("empty prefix must match nothing")
	}
	if l, n, _ := MatchVersions(vs, "PH"); l != nil || n != nil {
		t.Fatal("unknown prefix must match nothing")
	}
}

func TestMatchBareVersions(t *testing.T) {
	vs := []Version{
		{Name: "10.1.0", Released: true, ReleaseDate: "2026-09-20"},
		{Name: "10.0.0", Released: true, ReleaseDate: "2026-08-01"},
		{Name: "10.3.0", Released: false},
		{Name: "10.2.0", Released: false},
		{Name: "v11.0.0", Released: false},
		{Name: "MW 1.11.0", Released: false},
		{Name: "Sprint 42", Released: false},
	}
	live, next, queued := MatchVersions(vs, BarePrefix)
	if live == nil || live.Name != "10.1.0" {
		t.Fatalf("live: %+v", live)
	}
	if next == nil || next.Name != "10.2.0" {
		t.Fatalf("next: %+v", next)
	}
	if len(queued) != 2 || queued[0].Name != "10.3.0" || queued[1].Name != "v11.0.0" {
		t.Fatalf("queued: %+v", queued)
	}
}

func TestVersionsPaged(t *testing.T) {
	c, calls := newServer(t, map[string]string{
		"startAt=0&status=released":   `{"values":[{"id":"1","name":"MW 1.10.0","released":true,"releaseDate":"2026-10-02"}],"isLast":true}`,
		"startAt=0&status=unreleased": `{"values":[{"id":"2","name":"MW 1.11.0","released":false}],"isLast":false}`,
		"startAt=1&status=unreleased": `{"values":[{"id":"3","name":"CDW 1.29.0","released":false,"releaseDate":"2026-10-19"}],"isLast":true}`,
	})
	vs, err := c.Versions(context.Background(), "CD")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 3 || *calls != 3 {
		t.Fatalf("versions %d calls %d", len(vs), *calls)
	}
}

func TestVersionTickets(t *testing.T) {
	c, _ := newServer(t, map[string]string{
		`jql=fixVersion = "MW 1.11.0" AND labels IN ("FE")`: `{"issues":[
			{"key":"CD-1","fields":{"summary":"One","status":{"name":"In QA","statusCategory":{"key":"indeterminate"}},"assignee":{"displayName":"A. Pham","accountId":"x"},"issuetype":{"name":"Story"}}},
			{"key":"CD-2","fields":{"summary":"Two","status":{"name":"Open","statusCategory":{"key":"new"}},"assignee":null,"issuetype":{"name":"Bug"}}}
		],"isLast":true}`,
	})
	got, err := c.VersionTickets(context.Background(), "MW 1.11.0", `labels IN ("FE")`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "CD-1" || got[0].Status != "In QA" || got[0].Category != "indeterminate" || got[0].Assignee != "A. Pham" {
		t.Fatalf("tickets: %+v", got)
	}
	if got[1].Assignee != "" || !strings.HasSuffix(got[1].URL, "/browse/CD-2") {
		t.Fatalf("ticket 2: %+v", got[1])
	}
}

func TestVersionTicketsNoScope(t *testing.T) {
	c, _ := newServer(t, map[string]string{
		`jql=fixVersion = "MW 1.11.0"&`: `{"issues":[],"isLast":true}`,
	})
	if _, err := c.VersionTickets(context.Background(), "MW 1.11.0", ""); err != nil {
		t.Fatal(err)
	}
}

func TestSelected(t *testing.T) {
	c, _ := newServer(t, map[string]string{
		`status CHANGED TO "Selected for Development" AFTER -7d`: fmt.Sprintf(`{"issues":[
			{"key":"CD-9","fields":{"summary":"Nine","status":{"name":"Blocked","statusCategory":{"key":"new"}},"assignee":{"displayName":"Mathew Epstein"},"issuetype":{"name":"Task"}},
			 "changelog":{"histories":[
				{"created":"%s","items":[{"field":"status","fromString":"Selected for Development","toString":"Blocked"}]},
				{"created":"%s","items":[{"field":"status","fromString":"Open","toString":"Selected for Development"}]}
			 ]}},
			{"key":"CD-8","fields":{"summary":"Eight","status":{"name":"Selected for Development","statusCategory":{"key":"new"}},"assignee":{"displayName":"Ana Ruiz"},"issuetype":{"name":"Task"}},
			 "changelog":{"histories":[
				{"created":"%s","items":[{"field":"status","fromString":"Open","toString":"Selected for Development"}]}
			 ]}}
		],"isLast":true}`, ts(-1*time.Hour), ts(-2*time.Hour), ts(-2*time.Hour)),
	})
	got, err := c.Selected(context.Background(), "CD", `labels IN ("FE")`, "Selected for Development")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "CD-8" || got[0].Assignee != "Ana Ruiz" {
		t.Fatalf("only issues still in the status: %+v", got)
	}
	if d := time.Since(got[0].MovedAt); d < 110*time.Minute || d > 130*time.Minute {
		t.Fatalf("movedAt should be the transition into the status, ~2h ago; got %v ago", d)
	}
	if !strings.HasPrefix(got[0].ID, "CD-8:") {
		t.Fatalf("selected id: %q", got[0].ID)
	}
}

func TestAssigned(t *testing.T) {
	c, _ := newServer(t, map[string]string{
		`assignee CHANGED TO "acct-me" AFTER -14d`: fmt.Sprintf(`{"issues":[
			{"key":"CD-5","fields":{"summary":"Five","status":{"name":"In Progress","statusCategory":{"key":"indeterminate"}},"assignee":{"displayName":"Me","accountId":"acct-me"},"issuetype":{"name":"Bug"}},
			 "changelog":{"histories":[
				{"created":"%s","items":[{"field":"assignee","to":"acct-me","toString":"Me"}]},
				{"created":"%s","items":[{"field":"assignee","to":"acct-other","toString":"Other"}]}
			 ]}},
			{"key":"CD-6","fields":{"summary":"Six","status":{"name":"Open","statusCategory":{"key":"new"}},"assignee":{"displayName":"Me","accountId":"acct-me"},"issuetype":{"name":"Bug"}},
			 "changelog":{"histories":[
				{"created":"%s","items":[{"field":"assignee","to":"acct-me","toString":"Me"}]}
			 ]}}
		],"isLast":true}`, ts(-3*time.Hour), ts(-48*time.Hour), ts(-3*time.Hour)),
	})
	got, err := c.Assigned(context.Background(), "CD", me)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "CD-6" || got[0].Type != "Bug" || got[0].Status != "Open" {
		t.Fatalf("only to-do issues: %+v", got)
	}
	if d := time.Since(got[0].AssignedAt); d < 170*time.Minute || d > 190*time.Minute {
		t.Fatalf("assignedAt ~3h ago; got %v ago", d)
	}
	if !strings.HasPrefix(got[0].ID, "CD-6:") {
		t.Fatalf("assigned id: %q", got[0].ID)
	}
}

func mentionComment(created, author, authorName string, mentions ...string) string {
	var nodes []string
	for _, m := range mentions {
		nodes = append(nodes, fmt.Sprintf(`{"type":"mention","attrs":{"id":"%s","text":"@x"}}`, m))
	}
	return fmt.Sprintf(`{"created":"%s","author":{"accountId":"%s","displayName":"%s"},"body":{"type":"doc","content":[{"type":"paragraph","content":[%s]}]}}`,
		created, author, authorName, strings.Join(nodes, ","))
}

func TestMentions(t *testing.T) {
	issue := func(key string) string {
		return fmt.Sprintf(`{"key":"%s","fields":{"summary":"S %s","status":{"name":"Open","statusCategory":{"key":"new"}}}}`, key, key)
	}
	c, _ := newServer(t, map[string]string{
		`comment ~ "Me Name"`: `{"issues":[` + strings.Join([]string{issue("CD-1"), issue("CD-2"), issue("CD-3"), issue("CD-4")}, ",") + `],"isLast":true}`,
		"/issue/CD-1/comment": `{"comments":[` + mentionComment(ts(-2*time.Hour), "acct-sara", "Sara T.", me) + `]}`,
		"/issue/CD-2/comment": `{"comments":[` + mentionComment(ts(-1*time.Hour), me, "Me Name") + "," + mentionComment(ts(-2*time.Hour), "acct-sara", "Sara T.", me) + `]}`,
		"/issue/CD-3/comment": `{"comments":[` + mentionComment(ts(-2*time.Hour), me, "Me Name", me) + `]}`,
		"/issue/CD-4/comment": `{"comments":[` + mentionComment(ts(-20*24*time.Hour), "acct-sara", "Sara T.", me) + `]}`,
	})
	got, err := c.Mentions(context.Background(), "CD", me, "Me Name")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "CD-1" || got[0].LastCommentBy != "Sara T." {
		t.Fatalf("mentions: %+v", got)
	}
	if !strings.HasPrefix(got[0].ID, "CD-1:") || got[0].ID == "CD-1:" {
		t.Fatalf("mention id must be key + mention time: %q", got[0].ID)
	}
}

func TestRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "e", "t")
	_, err := c.Versions(context.Background(), "CD")
	var rl *RateLimitedError
	if !asRateLimited(err, &rl) || rl.RetryAfter != 30*time.Second {
		t.Fatalf("want RateLimitedError 30s, got %v", err)
	}
}
