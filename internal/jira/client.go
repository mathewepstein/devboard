package jira

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	email   string
	token   string
	http    *http.Client
}

func NewClient(baseURL, email, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		email:   email,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

type RateLimitedError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("jira %d, retry after %s", e.Status, e.RetryAfter)
}

func asRateLimited(err error, target **RateLimitedError) bool { return errors.As(err, target) }

func (c *Client) BrowseURL(key string) string { return c.BaseURL + "/browse/" + key }

func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read jira %s: %w", path, err)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &RateLimitedError{Status: resp.StatusCode, RetryAfter: retryAfter(resp)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira %s returned %d: %s", path, resp.StatusCode, truncate(body, 200))
	}
	return body, nil
}

func retryAfter(resp *http.Response) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return time.Minute
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

const jiraTime = "2006-01-02T15:04:05.000-0700"

func parseTime(s string) time.Time {
	t, err := time.Parse(jiraTime, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
