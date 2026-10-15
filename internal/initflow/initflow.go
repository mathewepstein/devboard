package initflow

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/mathewepstein/devboard/internal/config"
	"github.com/mathewepstein/devboard/internal/github"
	"github.com/mathewepstein/devboard/internal/jira"
	"github.com/mathewepstein/devboard/internal/secrets"
)

type Options struct {
	Out   io.Writer
	NoEnv bool
}

// Prompt, validate everything, then commit config + keychain in one step.
func Run(ctx context.Context, opts Options) error {
	w := opts.Out
	env := detectEnv(opts.NoEnv)
	env.PrintBanner(w)

	fmt.Fprintln(w, "Atlassian")
	fmt.Fprintln(w, "---------")
	var baseURL, email, jiraToken string
	if err := askText("Atlassian URL (e.g. https://acme.atlassian.net, or just the subdomain acme)", "", &baseURL, required); err != nil {
		return err
	}
	baseURL, err := normalizeJiraURL(baseURL)
	if err != nil {
		return err
	}
	if env.Has(fieldJiraEmail) {
		email = env.Get(fieldJiraEmail)
		env.Announce(w, fieldJiraEmail, "Atlassian email")
	} else if err := askText("Atlassian email", "", &email, required); err != nil {
		return err
	}
	if env.Has(fieldJiraToken) {
		jiraToken = env.Get(fieldJiraToken)
		env.Announce(w, fieldJiraToken, "Atlassian API token")
	} else if err := askPassword("Atlassian API token", &jiraToken); err != nil {
		return err
	}
	jc := jira.NewClient(baseURL, email, jiraToken)
	me, err := timed(ctx, func(ctx context.Context) (jira.User, error) { return jc.Myself(ctx) })
	if err != nil {
		return fmt.Errorf("Atlassian auth failed: %w", err)
	}
	fmt.Fprintf(w, "  OK, authenticated as %s\n", me.DisplayName)

	var project string
	if err := askText("Jira project key (the letters before the dash in a ticket key, e.g. CD for CD-12345)", "", &project, required); err != nil {
		return err
	}
	project = strings.ToUpper(strings.TrimSpace(project))
	projectName, err := timed(ctx, func(ctx context.Context) (string, error) { return jc.VerifyProject(ctx, project) })
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  OK, %s\n", projectName)

	tracked := me
	trackOther := false
	if err := survey.AskOne(&survey.Confirm{
		Message: fmt.Sprintf("Track yourself (%s)? Mentions and assignments for this account are what the page shows", me.DisplayName),
		Default: true,
	}, &trackOther); err != nil {
		return err
	}
	if !trackOther {
		tracked, err = pickUser(ctx, jc)
		if err != nil {
			return err
		}
	}

	scope, err := pickScope(ctx, w, jc, project)
	if err != nil {
		return err
	}

	statuses, err := timed(ctx, func(ctx context.Context) ([]jira.Status, error) { return jc.Statuses(ctx, project) })
	if err != nil {
		return err
	}
	pickedUp, err := pickPickedUp(statuses)
	if err != nil {
		return err
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "GitHub")
	fmt.Fprintln(w, "------")
	var ghToken, org string
	if env.Has(fieldGithubToken) {
		ghToken = env.Get(fieldGithubToken)
		env.Announce(w, fieldGithubToken, "GitHub token")
	} else if err := askPassword("GitHub personal access token", &ghToken); err != nil {
		return err
	}
	gc := github.NewClient(ghToken)
	login, err := timed(ctx, func(ctx context.Context) (string, error) { return gc.Viewer(ctx) })
	if err != nil {
		return fmt.Errorf("GitHub auth failed: %w", err)
	}
	fmt.Fprintf(w, "  OK, authenticated as %s\n", login)
	if env.Has(fieldGithubOrg) {
		org = env.Get(fieldGithubOrg)
		env.Announce(w, fieldGithubOrg, "GitHub org")
	} else if err := askText("GitHub org (the owner in github.com/<org>/<repo>, e.g. acme)", "", &org, required); err != nil {
		return err
	}
	if _, err := timed(ctx, func(ctx context.Context) (struct{}, error) { return struct{}{}, gc.VerifyOrg(ctx, org) }); err != nil {
		return err
	}

	fmt.Fprintln(w, "Listing repos...")
	repos, err := timed(ctx, func(ctx context.Context) ([]github.Repo, error) { return gc.OrgRepos(ctx, org) })
	if err != nil {
		return err
	}
	picked, err := pickRepos(repos)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, "Reading fixVersions...")
	versions, err := timed(ctx, func(ctx context.Context) ([]jira.Version, error) { return jc.Versions(ctx, project) })
	if err != nil {
		return err
	}
	prefixes := Prefixes(versions)
	cfgRepos, err := pickPrefixes(w, picked, prefixes)
	if err != nil {
		return err
	}

	cfg := &config.Config{
		Jira: config.Jira{
			BaseURL: baseURL, Email: email, Project: project,
			AccountID: tracked.AccountID, DisplayName: tracked.DisplayName,
			PickedUpStatus: pickedUp, Scope: scope,
		},
		GitHub: config.GitHub{Login: login, Org: org},
		Repos:  cfgRepos,
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := secrets.Set("jira", jiraToken); err != nil {
		return err
	}
	if err := secrets.Set("github", ghToken); err != nil {
		return err
	}
	path, _ := config.Path()
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Wrote %s\n", path)
	fmt.Fprintln(w, "Tokens stored in the OS keychain.")
	fmt.Fprintln(w, "Next: devboard   (serves on http://127.0.0.1:8765)")
	return nil
}

func pickUser(ctx context.Context, jc *jira.Client) (jira.User, error) {
	for {
		var q string
		if err := askText("Name of the person to track (e.g. a teammate's first name)", "", &q, required); err != nil {
			return jira.User{}, err
		}
		users, err := timed(ctx, func(ctx context.Context) ([]jira.User, error) { return jc.UserSearch(ctx, q) })
		if err != nil {
			return jira.User{}, err
		}
		if len(users) == 0 {
			fmt.Println("  no active users match; try again")
			continue
		}
		opts := make([]string, len(users))
		for i, u := range users {
			opts[i] = u.DisplayName + " (" + u.Email + ")"
		}
		var idx int
		if err := survey.AskOne(&survey.Select{Message: "Track:", Options: opts, PageSize: 10}, &idx); err != nil {
			return jira.User{}, err
		}
		return users[idx], nil
	}
}

func pickScope(ctx context.Context, w io.Writer, jc *jira.Client, project string) (config.Scope, error) {
	const (
		byLabel = "Label(s): tickets carrying a label, e.g. FE"
		byBoard = "Board: whatever a Jira board's filter shows, e.g. Frontend Board"
		byJQL   = "JQL fragment: your own clause, e.g. component = web"
		byNone  = "None: every ticket in the project"
	)
	var kind string
	if err := survey.AskOne(&survey.Select{
		Message: "Ticket scope: which tickets count as your team's for version lists and the picked-up panel?",
		Options: []string{byLabel, byBoard, byJQL, byNone},
	}, &kind); err != nil {
		return config.Scope{}, err
	}
	switch kind {
	case byLabel:
		labels, err := timed(ctx, func(ctx context.Context) ([]string, error) { return jc.Labels(ctx) })
		if err != nil {
			return config.Scope{}, err
		}
		var picked []string
		if err := survey.AskOne(&survey.MultiSelect{Message: "Labels (type to filter, space to pick, enter when done):", Options: labels, PageSize: 15}, &picked, survey.WithValidator(survey.Required)); err != nil {
			return config.Scope{}, err
		}
		return config.Scope{Labels: picked}, nil
	case byBoard:
		boards, err := timed(ctx, func(ctx context.Context) ([]jira.Board, error) { return jc.Boards(ctx, project) })
		if err != nil {
			return config.Scope{}, err
		}
		if len(boards) == 0 {
			return config.Scope{}, fmt.Errorf("project %s has no boards", project)
		}
		opts := make([]string, len(boards))
		for i, b := range boards {
			opts[i] = fmt.Sprintf("%s (%s, id %d)", b.Name, b.Type, b.ID)
		}
		var idx int
		if err := survey.AskOne(&survey.Select{Message: "Board (its filter becomes the scope, minus any assignee terms):", Options: opts, PageSize: 15}, &idx); err != nil {
			return config.Scope{}, err
		}
		raw, err := timed(ctx, func(ctx context.Context) (string, error) { return jc.BoardFilterJQL(ctx, boards[idx].ID) })
		if err != nil {
			return config.Scope{}, err
		}
		stripped := StripBoardJQL(raw, project)
		fmt.Fprintf(w, "  board filter: %s\n", strings.Join(strings.Fields(raw), " "))
		if stripped == "" {
			fmt.Fprintln(w, "  nothing left after removing assignee/reporter terms; scope = whole project")
			return config.Scope{}, nil
		}
		fmt.Fprintf(w, "  stored scope: %s\n", stripped)
		if err := validateScope(ctx, jc, project, stripped); err != nil {
			return config.Scope{}, err
		}
		return config.Scope{BoardID: boards[idx].ID, JQL: stripped}, nil
	case byJQL:
		var frag string
		if err := askText("JQL fragment, ANDed under project = "+project+" (e.g. component = web AND type != Epic)", "", &frag, required); err != nil {
			return config.Scope{}, err
		}
		if err := validateScope(ctx, jc, project, frag); err != nil {
			return config.Scope{}, err
		}
		return config.Scope{JQL: frag}, nil
	}
	return config.Scope{}, nil
}

func validateScope(ctx context.Context, jc *jira.Client, project, frag string) error {
	_, err := timed(ctx, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, jc.ValidateJQL(ctx, "project = "+project+" AND ("+frag+")")
	})
	if err != nil {
		return fmt.Errorf("scope JQL rejected by Jira: %w", err)
	}
	return nil
}

func pickPickedUp(statuses []jira.Status) (string, error) {
	names, def := PickedUpCandidates(statuses)
	if len(names) == 0 {
		return "", fmt.Errorf("project has no statuses in the 'new' category")
	}
	var picked string
	if err := survey.AskOne(&survey.Select{
		Message: "Status that means a ticket was picked up for development (the page lists tickets that moved into it this week):",
		Options: names, Default: def, PageSize: 10,
	}, &picked); err != nil {
		return "", err
	}
	return picked, nil
}

func pickRepos(repos []github.Repo) ([]github.Repo, error) {
	if len(repos) == 0 {
		return nil, fmt.Errorf("org has no repos visible to this token")
	}
	opts := make([]string, len(repos))
	for i, r := range repos {
		opts[i] = fmt.Sprintf("%s  (pushed %s)", r.Name, r.PushedAt.Format("2006-01-02"))
	}
	var idx []int
	if err := survey.AskOne(&survey.MultiSelect{
		Message:  "Repos to watch (type to filter, space to pick, enter when done; most recently pushed first):",
		Options:  opts,
		PageSize: 20,
	}, &idx, survey.WithValidator(survey.Required)); err != nil {
		return nil, err
	}
	out := make([]github.Repo, len(idx))
	for i, j := range idx {
		out[i] = repos[j]
	}
	return out, nil
}

func pickPrefixes(w io.Writer, repos []github.Repo, prefixes []PrefixCount) ([]config.Repo, error) {
	known := make([]string, len(prefixes))
	opts := []string{"none"}
	for i, p := range prefixes {
		known[i] = p.Prefix
		opts = append(opts, fmt.Sprintf("%s  (%d versions)", p.Prefix, p.Count))
	}
	opts = append(opts, "other...")
	var out []config.Repo
	for _, r := range repos {
		guess := GuessPrefix(r.Name, known)
		def := "none"
		for _, o := range opts {
			if guess != "" && strings.HasPrefix(o, guess+"  ") {
				def = o
			}
		}
		var choice string
		if err := survey.AskOne(&survey.Select{
			Message:  "fixVersion prefix for " + r.Name + " (the letters before the number in its Jira versions, e.g. MW for \"MW 1.11.0\"; none if it has no versions):",
			Options:  opts,
			Default:  def,
			PageSize: 12,
		}, &choice); err != nil {
			return nil, err
		}
		prefix := ""
		switch choice {
		case "none":
		case "other...":
			if err := askText("Prefix (e.g. MW)", "", &prefix, required); err != nil {
				return nil, err
			}
		default:
			prefix = strings.Fields(choice)[0]
		}
		name := DisplayName(r.Name)
		if err := askText("Display name for "+r.Name+" (shown on the page; enter to keep)", name, &name, required); err != nil {
			return nil, err
		}
		out = append(out, config.Repo{Name: name, Repo: r.Name, Prefix: prefix})
	}
	return out, nil
}

func askText(message, def string, out *string, validators ...survey.Validator) error {
	opts := make([]survey.AskOpt, 0, len(validators))
	for _, v := range validators {
		opts = append(opts, survey.WithValidator(v))
	}
	return survey.AskOne(&survey.Input{Message: message + ":", Default: def}, out, opts...)
}

func askPassword(message string, out *string) error {
	return survey.AskOne(&survey.Password{Message: message + ":"}, out, survey.WithValidator(survey.Required))
}

func required(ans interface{}) error {
	if s, _ := ans.(string); strings.TrimSpace(s) == "" {
		return fmt.Errorf("value required")
	}
	return nil
}

func normalizeJiraURL(raw string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	if s == "" {
		return "", fmt.Errorf("Atlassian URL is empty")
	}
	if !strings.Contains(s, "://") && !strings.Contains(s, ".") && !strings.Contains(s, ":") {
		s = "https://" + s + ".atlassian.net"
	} else if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid Atlassian URL %q", raw)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func timed[T any](parent context.Context, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	return fn(ctx)
}

var _ = strconv.Itoa
