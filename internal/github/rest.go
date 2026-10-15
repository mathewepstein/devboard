package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"time"
)

const DefaultRESTEndpoint = "https://api.github.com"

type Repo struct {
	Name     string    `json:"name"`
	PushedAt time.Time `json:"pushed_at"`
	Archived bool      `json:"archived"`
}

func (c *Client) rest(ctx context.Context, path string) (*http.Response, []byte, error) {
	base := c.RESTEndpoint
	if base == "" {
		base = DefaultRESTEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("github %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("read github %s: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, body, fmt.Errorf("github %s returned %d: %s", path, resp.StatusCode, truncate(body, 200))
	}
	return resp, body, nil
}

func (c *Client) Viewer(ctx context.Context) (string, error) {
	_, body, err := c.rest(ctx, "/user")
	if err != nil {
		return "", err
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &u); err != nil || u.Login == "" {
		return "", fmt.Errorf("/user returned no login")
	}
	return u.Login, nil
}

func (c *Client) VerifyOrg(ctx context.Context, org string) error {
	_, _, err := c.rest(ctx, "/orgs/"+org)
	if err != nil {
		return fmt.Errorf("org %q not found or not visible: %w", org, err)
	}
	return nil
}

func (c *Client) RepoInfo(ctx context.Context, org, name string) (Repo, error) {
	_, body, err := c.rest(ctx, "/repos/"+org+"/"+name)
	if err != nil {
		return Repo{}, fmt.Errorf("repo %s/%s not found or not visible: %w", org, name, err)
	}
	var r Repo
	if err := json.Unmarshal(body, &r); err != nil {
		return Repo{}, fmt.Errorf("parse repo %s/%s: %w", org, name, err)
	}
	return r, nil
}

var nextPageRe = regexp.MustCompile(`[?&]page=(\d+)[^>]*>;\s*rel="next"`)

// Live repos, newest push first.
func (c *Client) OrgRepos(ctx context.Context, org string) ([]Repo, error) {
	var out []Repo
	page := "1"
	for {
		resp, body, err := c.rest(ctx, "/orgs/"+org+"/repos?per_page=100&type=all&sort=pushed&page="+page)
		if err != nil {
			return nil, err
		}
		var repos []Repo
		if err := json.Unmarshal(body, &repos); err != nil {
			return nil, fmt.Errorf("parse repos: %w", err)
		}
		for _, r := range repos {
			if !r.Archived {
				out = append(out, r)
			}
		}
		m := nextPageRe.FindStringSubmatch(resp.Header.Get("Link"))
		if m == nil || len(repos) == 0 {
			break
		}
		page = m[1]
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PushedAt.After(out[j].PushedAt) })
	return out, nil
}
