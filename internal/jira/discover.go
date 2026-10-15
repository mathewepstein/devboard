package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress"`
	AccountType string `json:"accountType"`
	Active      bool   `json:"active"`
}

func (c *Client) Myself(ctx context.Context) (User, error) {
	body, err := c.get(ctx, "/rest/api/3/myself", nil)
	if err != nil {
		return User{}, err
	}
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		return User{}, fmt.Errorf("parse /myself: %w", err)
	}
	if u.AccountID == "" {
		return User{}, fmt.Errorf("/myself returned no accountId")
	}
	return u, nil
}

// Returns the project name; 404 covers both missing and not visible.
func (c *Client) VerifyProject(ctx context.Context, key string) (string, error) {
	body, err := c.get(ctx, "/rest/api/3/project/"+key, nil)
	if err != nil {
		return "", fmt.Errorf("project %q not found or not visible: %w", key, err)
	}
	var p struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(body, &p)
	return p.Name, nil
}

// Active human accounts only.
func (c *Client) UserSearch(ctx context.Context, query string) ([]User, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("maxResults", "10")
	body, err := c.get(ctx, "/rest/api/3/user/search", q)
	if err != nil {
		return nil, err
	}
	var all []User
	if err := json.Unmarshal(body, &all); err != nil {
		return nil, fmt.Errorf("parse user search: %w", err)
	}
	out := all[:0]
	for _, u := range all {
		if u.Active && u.AccountType == "atlassian" {
			out = append(out, u)
		}
	}
	return out, nil
}

func (c *Client) Labels(ctx context.Context) ([]string, error) {
	var out []string
	for startAt := 0; ; {
		q := url.Values{}
		q.Set("startAt", strconv.Itoa(startAt))
		q.Set("maxResults", "1000")
		body, err := c.get(ctx, "/rest/api/3/label", q)
		if err != nil {
			return nil, err
		}
		var page struct {
			Values []string `json:"values"`
			IsLast bool     `json:"isLast"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parse labels: %w", err)
		}
		out = append(out, page.Values...)
		startAt += len(page.Values)
		if page.IsLast || len(page.Values) == 0 {
			break
		}
	}
	return out, nil
}

type Board struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

func (c *Client) Boards(ctx context.Context, project string) ([]Board, error) {
	var out []Board
	for startAt := 0; ; {
		q := url.Values{}
		q.Set("projectKeyOrId", project)
		q.Set("maxResults", "50")
		if startAt > 0 {
			q.Set("startAt", strconv.Itoa(startAt))
		}
		body, err := c.get(ctx, "/rest/agile/1.0/board", q)
		if err != nil {
			return nil, err
		}
		var page struct {
			Values []Board `json:"values"`
			IsLast bool    `json:"isLast"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parse boards: %w", err)
		}
		out = append(out, page.Values...)
		startAt += len(page.Values)
		if page.IsLast || len(page.Values) == 0 {
			break
		}
	}
	return out, nil
}

func (c *Client) BoardFilterJQL(ctx context.Context, boardID int) (string, error) {
	body, err := c.get(ctx, "/rest/agile/1.0/board/"+strconv.Itoa(boardID)+"/configuration", nil)
	if err != nil {
		return "", err
	}
	var conf struct {
		Filter struct {
			ID string `json:"id"`
		} `json:"filter"`
	}
	if err := json.Unmarshal(body, &conf); err != nil || conf.Filter.ID == "" {
		return "", fmt.Errorf("board %d has no filter", boardID)
	}
	body, err = c.get(ctx, "/rest/api/3/filter/"+conf.Filter.ID, nil)
	if err != nil {
		return "", err
	}
	var f struct {
		JQL string `json:"jql"`
	}
	if err := json.Unmarshal(body, &f); err != nil {
		return "", fmt.Errorf("parse filter: %w", err)
	}
	return f.JQL, nil
}

type Status struct {
	Name     string
	Category string
}

// Flattened across issue types; duplicates kept for the caller to union.
func (c *Client) Statuses(ctx context.Context, project string) ([]Status, error) {
	body, err := c.get(ctx, "/rest/api/3/project/"+project+"/statuses", nil)
	if err != nil {
		return nil, err
	}
	var types []struct {
		Statuses []struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(body, &types); err != nil {
		return nil, fmt.Errorf("parse statuses: %w", err)
	}
	var out []Status
	for _, t := range types {
		for _, s := range t.Statuses {
			out = append(out, Status{Name: s.Name, Category: s.StatusCategory.Key})
		}
	}
	return out, nil
}

// Runs the JQL with maxResults=1 to prove it parses.
func (c *Client) ValidateJQL(ctx context.Context, jql string) error {
	_, err := c.JQLMatches(ctx, jql)
	return err
}

// Unknown fields/values return 200 with no issues, not 400.
func (c *Client) JQLMatches(ctx context.Context, jql string) (bool, error) {
	q := url.Values{}
	q.Set("jql", jql)
	q.Set("fields", "key")
	q.Set("maxResults", "1")
	body, err := c.get(ctx, "/rest/api/3/search/jql", q)
	if err != nil {
		return false, err
	}
	var page struct {
		Issues []json.RawMessage `json:"issues"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return false, fmt.Errorf("parse search: %w", err)
	}
	return len(page.Issues) > 0, nil
}
