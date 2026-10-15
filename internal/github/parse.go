package github

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mathewepstein/devboard/internal/state"
)

type gqlResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Path    []any  `json:"path"`
	} `json:"errors"`
}

type repoNode struct {
	PullRequests struct {
		Nodes []prNode `json:"nodes"`
	} `json:"pullRequests"`
}

type prNode struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	ReviewDecision *string   `json:"reviewDecision"`
	Mergeable      string    `json:"mergeable"`
	UpdatedAt      time.Time `json:"updatedAt"`
	HeadRefName    string    `json:"headRefName"`
	Author         *struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login"`
				Name  string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestReviews struct {
		Nodes []struct {
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
			State string `json:"state"`
		} `json:"nodes"`
	} `json:"latestReviews"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				OID               string `json:"oid"`
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

func parseResponse(body []byte, repos []string, project string) (Result, error) {
	var resp gqlResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return Result{}, fmt.Errorf("parse github response: %w", err)
	}
	if len(resp.Errors) > 0 {
		var order []string
		counts := map[string]int{}
		for _, e := range resp.Errors {
			msg := e.Message
			if len(e.Path) > 1 {
				alias, _ := e.Path[0].(string)
				if i, err := strconv.Atoi(strings.TrimPrefix(alias, "r")); err == nil && i < len(repos) {
					alias = repos[i]
				}
				msg = fmt.Sprintf("%s %v: %s", alias, e.Path[len(e.Path)-1], msg)
			}
			if counts[msg] == 0 {
				order = append(order, msg)
			}
			counts[msg]++
		}
		for i, msg := range order {
			if n := counts[msg]; n > 1 {
				order[i] = fmt.Sprintf("%s (x%d)", msg, n)
			}
		}
		return Result{}, fmt.Errorf("github graphql: %s", strings.Join(order, "; "))
	}

	res := Result{PRs: make(map[string][]state.PR, len(repos))}
	if raw, ok := resp.Data["rateLimit"]; ok {
		if err := json.Unmarshal(raw, &res.RateLimit); err != nil {
			return Result{}, fmt.Errorf("parse rateLimit: %w", err)
		}
	}
	for i, repo := range repos {
		prs := []state.PR{}
		if raw, ok := resp.Data["r"+strconv.Itoa(i)]; ok && string(raw) != "null" {
			var node repoNode
			if err := json.Unmarshal(raw, &node); err != nil {
				return Result{}, fmt.Errorf("parse %s: %w", repo, err)
			}
			for _, n := range node.PullRequests.Nodes {
				prs = append(prs, toPR(n, project))
			}
		}
		res.PRs[repo] = prs
	}
	return res, nil
}

func toPR(n prNode, project string) state.PR {
	p := state.PR{
		Number:      n.Number,
		Title:       n.Title,
		URL:         n.URL,
		Draft:       n.IsDraft,
		Mergeable:   n.Mergeable,
		UpdatedAt:   n.UpdatedAt,
		HeadRefName: n.HeadRefName,
		Requested:   []string{},
		Reviewers:   []string{},
		TicketKeys:  TicketKeys(project, n.Title, n.HeadRefName),
	}
	if n.Author != nil {
		p.Author = n.Author.Login
	}
	if n.ReviewDecision != nil {
		p.ReviewDecision = *n.ReviewDecision
	}
	for _, r := range n.ReviewRequests.Nodes {
		if r.RequestedReviewer.Login != "" {
			p.Requested = append(p.Requested, r.RequestedReviewer.Login)
		} else if r.RequestedReviewer.Name != "" {
			p.Requested = append(p.Requested, r.RequestedReviewer.Name)
		}
	}
	for _, r := range n.LatestReviews.Nodes {
		if r.Author != nil {
			p.Reviewers = append(p.Reviewers, r.Author.Login+":"+r.State)
		}
	}
	p.Checks = "none"
	if len(n.Commits.Nodes) > 0 {
		c := n.Commits.Nodes[0].Commit
		p.HeadSHA = c.OID
		if c.StatusCheckRollup != nil {
			switch c.StatusCheckRollup.State {
			case "SUCCESS":
				p.Checks = "ok"
			case "FAILURE", "ERROR":
				p.Checks = "fail"
			case "PENDING", "EXPECTED":
				p.Checks = "pending"
			}
		}
	}
	return p
}

var keyRe = map[string]*regexp.Regexp{}

func TicketKeys(project string, texts ...string) []string {
	re, ok := keyRe[project]
	if !ok {
		re = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(project) + `-(\d+)\b`)
		keyRe[project] = re
	}
	seen := map[string]bool{}
	var keys []string
	for _, t := range texts {
		for _, m := range re.FindAllStringSubmatch(t, -1) {
			k := strings.ToUpper(project) + "-" + m[1]
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, _ := strconv.Atoi(keys[i][len(project)+1:])
		b, _ := strconv.Atoi(keys[j][len(project)+1:])
		return a < b
	})
	return keys
}
