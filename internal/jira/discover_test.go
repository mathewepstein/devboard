package jira

import (
	"context"
	"testing"
)

func TestMyself(t *testing.T) {
	c, _ := newServer(t, map[string]string{"/rest/api/3/myself": `{"accountId":"acct-me","displayName":"Me Name","emailAddress":"me@acme.com"}`})
	me, err := c.Myself(context.Background())
	if err != nil || me.AccountID != "acct-me" || me.DisplayName != "Me Name" {
		t.Fatalf("%+v %v", me, err)
	}
}

func TestVerifyProject(t *testing.T) {
	c, _ := newServer(t, map[string]string{"/rest/api/3/project/CD": `{"key":"CD","name":"ConsumerDirect"}`})
	name, err := c.VerifyProject(context.Background(), "CD")
	if err != nil || name != "ConsumerDirect" {
		t.Fatalf("%q %v", name, err)
	}
}

func TestUserSearch(t *testing.T) {
	c, _ := newServer(t, map[string]string{"/rest/api/3/user/search?maxResults=10&query=mat": `[{"accountId":"a1","displayName":"Mat A","accountType":"atlassian","active":true},{"accountId":"a2","displayName":"Bot","accountType":"app","active":true},{"accountId":"a3","displayName":"Gone","accountType":"atlassian","active":false}]`})
	users, err := c.UserSearch(context.Background(), "mat")
	if err != nil || len(users) != 1 || users[0].AccountID != "a1" {
		t.Fatalf("want only active atlassian users, got %+v %v", users, err)
	}
}

func TestLabelsPaged(t *testing.T) {
	c, _ := newServer(t, map[string]string{
		"/rest/api/3/label?maxResults=1000&startAt=0": `{"values":["BE","FE"],"isLast":false}`,
		"/rest/api/3/label?maxResults=1000&startAt=2": `{"values":["QA"],"isLast":true}`,
	})
	labels, err := c.Labels(context.Background())
	if err != nil || len(labels) != 3 || labels[2] != "QA" {
		t.Fatalf("%v %v", labels, err)
	}
}

func TestBoardsAndFilter(t *testing.T) {
	c, _ := newServer(t, map[string]string{
		"/rest/agile/1.0/board?maxResults=50&projectKeyOrId=CD": `{"values":[{"id":357,"type":"kanban","name":"Frontend Board"},{"id":27,"type":"kanban","name":"CD Kanban Board"}],"isLast":true}`,
		"/rest/agile/1.0/board/357/configuration":               `{"filter":{"id":"14596"}}`,
		"/rest/api/3/filter/14596":                              `{"name":"Filter for Frontend Board","jql":"project = CD AND labels = FE ORDER BY created DESC"}`,
	})
	boards, err := c.Boards(context.Background(), "CD")
	if err != nil || len(boards) != 2 || boards[0].ID != 357 || boards[0].Name != "Frontend Board" {
		t.Fatalf("%+v %v", boards, err)
	}
	jql, err := c.BoardFilterJQL(context.Background(), 357)
	if err != nil || jql != "project = CD AND labels = FE ORDER BY created DESC" {
		t.Fatalf("%q %v", jql, err)
	}
}

func TestStatuses(t *testing.T) {
	c, _ := newServer(t, map[string]string{"/rest/api/3/project/CD/statuses": `[
		{"name":"Epic","statuses":[{"name":"Open","statusCategory":{"key":"new"}},{"name":"Done","statusCategory":{"key":"done"}}]},
		{"name":"Bug","statuses":[{"name":"Open","statusCategory":{"key":"new"}},{"name":"Selected for Development","statusCategory":{"key":"new"}}]}
	]`})
	st, err := c.Statuses(context.Background(), "CD")
	if err != nil || len(st) != 4 || st[3].Name != "Selected for Development" || st[3].Category != "new" {
		t.Fatalf("%+v %v", st, err)
	}
}
