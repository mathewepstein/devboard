package initflow

import (
	"fmt"
	"io"
	"os"
)

type fieldID string

const (
	fieldJiraEmail   fieldID = "jira.email"
	fieldJiraToken   fieldID = "jira.token"
	fieldGithubToken fieldID = "github.token"
	fieldGithubOrg   fieldID = "github.org"
)

type envSource struct {
	VarName string
	Value   string
	Secret  bool
}

type envDetector struct {
	picks map[fieldID]envSource
}

func detectEnv(disabled bool) *envDetector {
	d := &envDetector{picks: map[fieldID]envSource{}}
	if disabled {
		return d
	}
	if v := os.Getenv("ATLASSIAN_EMAIL"); v != "" {
		d.picks[fieldJiraEmail] = envSource{"ATLASSIAN_EMAIL", v, false}
	}
	if v := os.Getenv("ATLASSIAN_API_TOKEN"); v != "" {
		d.picks[fieldJiraToken] = envSource{"ATLASSIAN_API_TOKEN", v, true}
	}
	if v := os.Getenv("GH_TOKEN"); v != "" {
		d.picks[fieldGithubToken] = envSource{"GH_TOKEN", v, true}
	} else if v := os.Getenv("GITHUB_TOKEN"); v != "" {
		d.picks[fieldGithubToken] = envSource{"GITHUB_TOKEN", v, true}
	}
	if v := os.Getenv("GITHUB_ORG"); v != "" {
		d.picks[fieldGithubOrg] = envSource{"GITHUB_ORG", v, false}
	}
	return d
}

func (d *envDetector) Has(id fieldID) bool { _, ok := d.picks[id]; return ok }

func (d *envDetector) Get(id fieldID) string { return d.picks[id].Value }

func (d *envDetector) PrintBanner(w io.Writer) {
	if len(d.picks) == 0 {
		return
	}
	fmt.Fprintln(w, "Detected environment variables (used instead of prompting):")
	for _, id := range []fieldID{fieldJiraEmail, fieldJiraToken, fieldGithubToken, fieldGithubOrg} {
		p, ok := d.picks[id]
		if !ok {
			continue
		}
		display := p.Value
		if p.Secret {
			display = maskToken(p.Value)
		}
		fmt.Fprintf(w, "  $%-20s → %s\n", p.VarName, display)
	}
	fmt.Fprintln(w, "Re-run with --no-env to ignore these and prompt for everything.")
	fmt.Fprintln(w)
}

func (d *envDetector) Announce(w io.Writer, id fieldID, label string) {
	p := d.picks[id]
	display := p.Value
	if p.Secret {
		display = maskToken(p.Value)
	}
	fmt.Fprintf(w, "%s: %s  (from $%s)\n", label, display, p.VarName)
}

func maskToken(s string) string {
	if len(s) <= 4 {
		return "••••"
	}
	return "••••" + s[len(s)-4:]
}
