package initflow

import (
	"regexp"
	"sort"
	"strings"

	"github.com/mathewepstein/devboard/internal/jira"
)

type PrefixCount struct {
	Prefix string
	Count  int
}

var prefixRe = regexp.MustCompile(`^([A-Za-z]+)[ -]v?\d`)

// Prefixes discovered in version names, most common first then alphabetical.
func Prefixes(vs []jira.Version) []PrefixCount {
	counts := map[string]int{}
	for _, v := range vs {
		if jira.IsBare(v.Name) {
			counts[jira.BarePrefix]++
		} else if m := prefixRe.FindStringSubmatch(v.Name); m != nil {
			counts[m[1]]++
		}
	}
	out := make([]PrefixCount, 0, len(counts))
	for p, n := range counts {
		out = append(out, PrefixCount{p, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Prefix < out[j].Prefix
	})
	return out
}

// Longest known prefix anchored on the first letter, in order.
func GuessPrefix(repo string, known []string) string {
	joined := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(repo), "-", ""), "_", ""), ".", "")
	if joined == "" {
		return ""
	}
	best, tie := "", false
	for _, p := range known {
		lp := strings.ToLower(p)
		if lp == "" || lp == jira.BarePrefix || lp[0] != joined[0] || !subsequence(lp, joined) {
			continue
		}
		switch {
		case len(lp) > len(best):
			best, tie = p, false
		case len(lp) == len(best):
			tie = true
		}
	}
	if tie {
		return ""
	}
	return best
}

func subsequence(lp, joined string) bool {
	i := 0
	for j := 0; j < len(joined) && i < len(lp); j++ {
		if lp[i] == joined[j] {
			i++
		}
	}
	return i == len(lp)
}

// "member-websites" → "Member Websites"; a trailing "website" word is dropped.
func DisplayName(repo string) string {
	words := strings.FieldsFunc(repo, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	if n := len(words); n > 1 && strings.EqualFold(words[n-1], "website") {
		words = words[:n-1]
	}
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

var (
	orderByRe  = regexp.MustCompile(`(?is)\s+ORDER\s+BY\s+.*$`)
	identityRe = regexp.MustCompile(`(?is)\b(assignee|reporter)\s+(=|!=|in|not\s+in|is|is\s+not)\s*(\((?:[^()]|\([^()]*\))*\)|[^\s()]+(?:\(\))?)`)
)

// Drops ORDER BY, assignee/reporter terms, project = KEY.
func StripBoardJQL(jql, project string) string {
	s := orderByRe.ReplaceAllString(jql, "")
	s = identityRe.ReplaceAllString(s, "")
	projectRe := regexp.MustCompile(`(?i)\bproject\s*=\s*"?` + regexp.QuoteMeta(project) + `"?\b`)
	s = projectRe.ReplaceAllString(s, "")
	parts := regexp.MustCompile(`(?i)\s+AND\s+`).Split(strings.TrimSpace(s), -1)
	var kept []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "AND ")
		p = strings.TrimSpace(p)
		if p != "" && !strings.EqualFold(p, "AND") {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " AND ")
}

const defaultPickedUp = "Selected for Development"

// Sorted "new"-category statuses plus a default.
func PickedUpCandidates(statuses []jira.Status) ([]string, string) {
	set := map[string]bool{}
	for _, s := range statuses {
		if s.Category == "new" {
			set[s.Name] = true
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	def := ""
	if set[defaultPickedUp] {
		def = defaultPickedUp
	} else if len(names) == 1 {
		def = names[0]
	}
	return names, def
}
