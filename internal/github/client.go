package github

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mathewepstein/devboard/internal/state"
)

//go:embed query.graphql
var prsFragment string

const DefaultEndpoint = "https://api.github.com/graphql"

type Client struct {
	Endpoint     string
	RESTEndpoint string
	token        string
	http         *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		Endpoint: DefaultEndpoint,
		token:    token,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

type RateLimit struct {
	Cost      int       `json:"cost"`
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetAt   time.Time `json:"resetAt"`
}

type Result struct {
	PRs       map[string][]state.PR
	RateLimit RateLimit
}

type RateLimitedError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("github %d, retry after %s", e.Status, e.RetryAfter)
}

func asRateLimited(err error, target **RateLimitedError) bool { return errors.As(err, target) }

func BuildQuery(repos []string) string {
	var b strings.Builder
	b.WriteString("query($owner: String!) {\n")
	for i, r := range repos {
		fmt.Fprintf(&b, "  r%d: repository(owner: $owner, name: %s) { ...PRs }\n", i, strconv.Quote(r))
	}
	b.WriteString("  rateLimit { cost remaining limit resetAt }\n}\n")
	b.WriteString(prsFragment)
	return b.String()
}

func (c *Client) FetchPRs(ctx context.Context, org string, repos []string, project string) (Result, error) {
	payload, err := json.Marshal(map[string]any{
		"query":     BuildQuery(repos),
		"variables": map[string]string{"owner": org},
	})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("github graphql: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("read github response: %w", err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return Result{}, &RateLimitedError{Status: resp.StatusCode, RetryAfter: retryAfter(resp)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("github graphql %d: %s", resp.StatusCode, truncate(body, 200))
	}
	return parseResponse(body, repos, project)
}

func retryAfter(resp *http.Response) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	if s := resp.Header.Get("X-RateLimit-Reset"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			if d := time.Until(time.Unix(n, 0)); d > 0 {
				return d
			}
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
