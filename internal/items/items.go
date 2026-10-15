package items

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/mathewepstein/devboard/internal/state"
)

var ruleOrder = map[string]int{"merge": 0, "fix": 1, "review": 2}

func Derive(login string, repos []state.Repo) []state.Item {
	out := []state.Item{}
	for _, r := range repos {
		for _, pr := range r.PRs {
			if it, ok := classify(login, r.GHRepo, pr); ok {
				out = append(out, it)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ruleOrder[out[i].Rule] != ruleOrder[out[j].Rule] {
			return ruleOrder[out[i].Rule] < ruleOrder[out[j].Rule]
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func classify(login, repo string, pr state.PR) (state.Item, bool) {
	var rule, detail string
	switch {
	case pr.Author == login:
		if pr.Draft {
			return state.Item{}, false
		}
		var problems []string
		if pr.ReviewDecision == "CHANGES_REQUESTED" {
			problems = append(problems, "changes requested")
		}
		if pr.Checks == "fail" {
			problems = append(problems, "checks failing")
		}
		if pr.Mergeable == "CONFLICTING" {
			problems = append(problems, "conflict")
		}
		switch {
		case len(problems) > 0:
			rule, detail = "fix", strings.Join(problems, ", ")
		case pr.ReviewDecision == "APPROVED":
			rule, detail = "merge", "approved"
			if pr.Checks == "ok" {
				detail += ", checks ok"
			}
		default:
			return state.Item{}, false
		}
	case contains(pr.Requested, login):
		rule = "review"
		if pr.ReviewDecision == "CHANGES_REQUESTED" {
			detail = "changes requested"
		}
	default:
		return state.Item{}, false
	}
	return state.Item{
		ID:        id(repo, pr, rule),
		Rule:      rule,
		Detail:    detail,
		Repo:      repo,
		Number:    pr.Number,
		Title:     pr.Title,
		URL:       pr.URL,
		UpdatedAt: pr.UpdatedAt,
	}, true
}

func id(repo string, pr state.PR, rule string) string {
	sum := sha256.Sum256([]byte(pr.ReviewDecision + "|" + pr.Checks + "|" + pr.Mergeable + "|" + pr.HeadSHA))
	return fmt.Sprintf("%s#%d:%s:%s", repo, pr.Number, rule, hex.EncodeToString(sum[:4]))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
