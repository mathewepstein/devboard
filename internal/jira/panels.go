package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mathewepstein/devboard/internal/state"
)

const (
	selectedWindow = "-7d"
	assignedWindow = "-14d"
	mentionWindow  = 14 * 24 * time.Hour
)

func stamp(key string, t time.Time) string {
	return key + ":" + strconv.FormatInt(t.Unix(), 10)
}

func and(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " AND ")
}

func (c *Client) VersionTickets(ctx context.Context, version, scope string) ([]state.Ticket, error) {
	issues, err := c.Search(ctx, and(`fixVersion = "`+version+`"`, scope), false)
	if err != nil {
		return nil, err
	}
	out := make([]state.Ticket, 0, len(issues))
	for _, is := range issues {
		out = append(out, state.Ticket{
			Key: is.Key, Summary: is.Summary, URL: c.BrowseURL(is.Key),
			Status: is.Status, Category: is.Category, Assignee: is.Assignee,
		})
	}
	return out, nil
}

func (c *Client) Selected(ctx context.Context, project, scope, status string) ([]state.Selected, error) {
	jql := and("project = "+project, scope, `status CHANGED TO "`+status+`" AFTER `+selectedWindow)
	issues, err := c.Search(ctx, jql, true)
	if err != nil {
		return nil, err
	}
	out := make([]state.Selected, 0, len(issues))
	for _, is := range issues {
		moved := is.LastChangeTo("status", status, "")
		out = append(out, state.Selected{
			ID: stamp(is.Key, moved), Key: is.Key, Summary: is.Summary, URL: c.BrowseURL(is.Key),
			Assignee: is.Assignee, MovedAt: moved,
		})
	}
	return out, nil
}

func (c *Client) Assigned(ctx context.Context, project, accountID string) ([]state.Assigned, error) {
	jql := and("project = "+project, `assignee CHANGED TO "`+accountID+`" AFTER `+assignedWindow, "resolution = Unresolved")
	issues, err := c.Search(ctx, jql, true)
	if err != nil {
		return nil, err
	}
	out := make([]state.Assigned, 0, len(issues))
	for _, is := range issues {
		at := is.LastChangeTo("assignee", "", accountID)
		out = append(out, state.Assigned{
			ID: stamp(is.Key, at), Key: is.Key, Summary: is.Summary, URL: c.BrowseURL(is.Key),
			Status: is.Status, Type: is.Type, AssignedAt: at,
		})
	}
	return out, nil
}

type comment struct {
	Created time.Time
	Author  string
	Name    string
	Mention bool
}

func (c *Client) Mentions(ctx context.Context, project, accountID, displayName string) ([]state.Mention, error) {
	jql := and("project = "+project, "resolution = Unresolved", `comment ~ "`+displayName+`"`, "updated >= "+assignedWindow)
	issues, err := c.Search(ctx, jql, false)
	if err != nil {
		return nil, err
	}
	out := []state.Mention{}
	cutoff := time.Now().Add(-mentionWindow)
	for _, is := range issues {
		comments, err := c.comments(ctx, is.Key, accountID)
		if err != nil {
			return nil, err
		}
		var hit *comment
		var replied bool
		for i := range comments {
			cm := &comments[i]
			if cm.Author == accountID {
				if hit == nil {
					replied = true
				}
				continue
			}
			if cm.Mention && cm.Created.After(cutoff) && hit == nil {
				hit = cm
			}
		}
		if hit == nil || replied {
			continue
		}
		m := state.Mention{
			ID: stamp(is.Key, hit.Created), Key: is.Key, Summary: is.Summary, URL: c.BrowseURL(is.Key),
			Status: is.Status, Updated: hit.Created,
		}
		if len(comments) > 0 {
			m.LastCommentBy = comments[0].Name
		}
		out = append(out, m)
	}
	return out, nil
}

// Newest first, as the API returns with orderBy=-created.
func (c *Client) comments(ctx context.Context, key, accountID string) ([]comment, error) {
	q := url.Values{}
	q.Set("orderBy", "-created")
	q.Set("maxResults", strconv.Itoa(50))
	body, err := c.get(ctx, "/rest/api/3/issue/"+key+"/comment", q)
	if err != nil {
		return nil, err
	}
	var page struct {
		Comments []struct {
			Created string `json:"created"`
			Author  struct {
				AccountID   string `json:"accountId"`
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Body json.RawMessage `json:"body"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("parse comments %s: %w", key, err)
	}
	out := make([]comment, 0, len(page.Comments))
	for _, cm := range page.Comments {
		out = append(out, comment{
			Created: parseTime(cm.Created),
			Author:  cm.Author.AccountID,
			Name:    cm.Author.DisplayName,
			Mention: mentions(cm.Body, accountID),
		})
	}
	return out, nil
}

func mentions(body json.RawMessage, accountID string) bool {
	var node any
	if err := json.Unmarshal(body, &node); err != nil {
		return false
	}
	return walk(node, accountID)
}

func walk(n any, accountID string) bool {
	switch v := n.(type) {
	case map[string]any:
		if v["type"] == "mention" {
			if attrs, ok := v["attrs"].(map[string]any); ok && attrs["id"] == accountID {
				return true
			}
		}
		for _, child := range v {
			if walk(child, accountID) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if walk(child, accountID) {
				return true
			}
		}
	}
	return false
}
