package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

type Issue struct {
	Key        string
	Summary    string
	Status     string
	Category   string
	Assignee   string
	AssigneeID string
	Type       string
	Histories  []History
}

type History struct {
	Created    time.Time
	Field      string
	FromString string
	ToString   string
	To         string
}

type rawIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  *struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Assignee *struct {
			DisplayName string `json:"displayName"`
			AccountID   string `json:"accountId"`
		} `json:"assignee"`
		IssueType *struct {
			Name string `json:"name"`
		} `json:"issuetype"`
	} `json:"fields"`
	Changelog *struct {
		Histories []struct {
			Created string `json:"created"`
			Items   []struct {
				Field      string `json:"field"`
				FromString string `json:"fromString"`
				ToString   string `json:"toString"`
				To         string `json:"to"`
			} `json:"items"`
		} `json:"histories"`
	} `json:"changelog"`
}

const searchFields = "key,summary,status,assignee,issuetype"

func (c *Client) Search(ctx context.Context, jql string, changelog bool) ([]Issue, error) {
	var out []Issue
	token := ""
	for {
		q := url.Values{}
		q.Set("jql", jql)
		q.Set("fields", searchFields)
		q.Set("maxResults", "100")
		if changelog {
			q.Set("expand", "changelog")
		}
		if token != "" {
			q.Set("nextPageToken", token)
		}
		body, err := c.get(ctx, "/rest/api/3/search/jql", q)
		if err != nil {
			return nil, err
		}
		var page struct {
			Issues        []rawIssue `json:"issues"`
			NextPageToken string     `json:"nextPageToken"`
			IsLast        bool       `json:"isLast"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parse search: %w", err)
		}
		for _, r := range page.Issues {
			out = append(out, toIssue(r))
		}
		if page.IsLast || page.NextPageToken == "" {
			break
		}
		token = page.NextPageToken
	}
	return out, nil
}

func toIssue(r rawIssue) Issue {
	is := Issue{Key: r.Key, Summary: r.Fields.Summary}
	if r.Fields.Status != nil {
		is.Status = r.Fields.Status.Name
		is.Category = r.Fields.Status.StatusCategory.Key
	}
	if r.Fields.Assignee != nil {
		is.Assignee = r.Fields.Assignee.DisplayName
		is.AssigneeID = r.Fields.Assignee.AccountID
	}
	if r.Fields.IssueType != nil {
		is.Type = r.Fields.IssueType.Name
	}
	if r.Changelog != nil {
		for _, h := range r.Changelog.Histories {
			created := parseTime(h.Created)
			for _, it := range h.Items {
				is.Histories = append(is.Histories, History{
					Created: created, Field: it.Field, FromString: it.FromString, ToString: it.ToString, To: it.To,
				})
			}
		}
	}
	return is
}

// Newest change where field moved to the given value.
func (is Issue) LastChangeTo(field, toString, toID string) time.Time {
	var best time.Time
	for _, h := range is.Histories {
		if h.Field != field {
			continue
		}
		if (toString != "" && h.ToString == toString) || (toID != "" && h.To == toID) {
			if h.Created.After(best) {
				best = h.Created
			}
		}
	}
	return best
}
