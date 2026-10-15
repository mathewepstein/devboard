package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type Version struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Released    bool   `json:"released"`
	ReleaseDate string `json:"releaseDate"`
}

func (c *Client) Versions(ctx context.Context, project string) ([]Version, error) {
	var all []Version
	for _, status := range []string{"released", "unreleased"} {
		for startAt := 0; ; {
			q := url.Values{}
			q.Set("status", status)
			q.Set("startAt", strconv.Itoa(startAt))
			q.Set("maxResults", "50")
			body, err := c.get(ctx, "/rest/api/3/project/"+project+"/version", q)
			if err != nil {
				return nil, err
			}
			var page struct {
				Values []Version `json:"values"`
				IsLast bool      `json:"isLast"`
			}
			if err := json.Unmarshal(body, &page); err != nil {
				return nil, fmt.Errorf("parse versions: %w", err)
			}
			all = append(all, page.Values...)
			startAt += len(page.Values)
			if page.IsLast || len(page.Values) == 0 {
				break
			}
		}
	}
	return all, nil
}

// live: released, latest releaseDate. next: unreleased, lowest semver. queued: the rest.
func MatchVersions(vs []Version, prefix string) (live, next *Version, queued []Version) {
	if prefix == "" {
		return nil, nil, nil
	}
	var released, unreleased []Version
	for _, v := range vs {
		if !hasPrefix(v.Name, prefix) {
			continue
		}
		if v.Released {
			released = append(released, v)
		} else {
			unreleased = append(unreleased, v)
		}
	}
	if len(released) > 0 {
		sort.SliceStable(released, func(i, j int) bool {
			if released[i].ReleaseDate != released[j].ReleaseDate {
				return released[i].ReleaseDate > released[j].ReleaseDate
			}
			return semverLess(numbers(released[j].Name, prefix), numbers(released[i].Name, prefix))
		})
		live = &released[0]
	}
	if len(unreleased) > 0 {
		sort.SliceStable(unreleased, func(i, j int) bool {
			return semverLess(numbers(unreleased[i].Name, prefix), numbers(unreleased[j].Name, prefix))
		})
		next = &unreleased[0]
		queued = unreleased[1:]
	}
	return live, next, queued
}

func hasPrefix(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) {
		return false
	}
	sep := name[len(prefix)]
	return sep == ' ' || sep == '-' || sep == '_'
}

func numbers(name, prefix string) []int {
	rest := strings.TrimLeft(name[len(prefix):], " -_v")
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '.' || r == ' ' || r == '-' })
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

func semverLess(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}
