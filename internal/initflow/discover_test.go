package initflow

import (
	"reflect"
	"testing"

	"github.com/mathewepstein/devboard/internal/jira"
)

func TestPrefixes(t *testing.T) {
	vs := []jira.Version{
		{Name: "MW 1.11.0"}, {Name: "CDW 1.29.0"}, {Name: "CDW 1.30.0"}, {Name: "PH-1.9.0"},
		{Name: "10.2.0"}, {Name: "10.1.0"}, {Name: "v11.0.0"}, {Name: "RMS v2.1"}, {Name: "Sprint 42"},
	}
	got := Prefixes(vs)
	want := []PrefixCount{{jira.BarePrefix, 3}, {"CDW", 2}, {"MW", 1}, {"PH", 1}, {"RMS", 1}, {"Sprint", 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestGuessPrefix(t *testing.T) {
	known := []string{jira.BarePrefix, "MW", "CDW", "PH", "PL", "TLS", "SMW", "SCW"}
	cases := map[string]string{
		"member-websites":         "MW",
		"cd-widgets":              "CDW",
		"partnerhub-website":      "PH",
		"platformlink-website":    "PL",
		"thelendingscore-website": "TLS",
		"hogo-website":            "",
		"scoremaster":             "",
		"smartcredit-website":     "",
		"sql":                     "",
		"main":                    "",
	}
	for repo, want := range cases {
		if got := GuessPrefix(repo, known); got != want {
			t.Errorf("%s: got %q want %q", repo, got, want)
		}
	}
}

func TestDisplayName(t *testing.T) {
	cases := map[string]string{
		"member-websites":      "Member Websites",
		"platformlink-website": "Platformlink",
		"cd-widgets":           "Cd Widgets",
		"hogo-website":         "Hogo",
		"scoremaster":          "Scoremaster",
	}
	for repo, want := range cases {
		if got := DisplayName(repo); got != want {
			t.Errorf("%s: got %q want %q", repo, got, want)
		}
	}
}

func TestStripBoardJQL(t *testing.T) {
	in := `project = CD
AND assignee IN (currentUser(), 712020:0f6e4ff0-c06e-44f1-bcdb-730e03fde93d, 557058:b06cd5cb-5451-4a85-aa16-3b701b828440, empty)
AND labels = FE
AND type != Epic
ORDER BY created DESC`
	if got := StripBoardJQL(in, "CD"); got != "labels = FE AND type != Epic" {
		t.Fatalf("got %q", got)
	}
	if got := StripBoardJQL(`project = CD AND reporter = currentUser() ORDER BY rank`, "CD"); got != "" {
		t.Fatalf("all-identity filter must reduce to empty, got %q", got)
	}
	if got := StripBoardJQL(`project in (CD, X) AND component = web`, "CD"); got != "project in (CD, X) AND component = web" {
		t.Fatalf("only the exact project = KEY term is dropped, got %q", got)
	}
}

func TestPickedUpCandidates(t *testing.T) {
	statuses := []jira.Status{
		{Name: "Open", Category: "new"}, {Name: "Selected for Development", Category: "new"},
		{Name: "In Progress", Category: "indeterminate"}, {Name: "Done", Category: "done"},
		{Name: "Open", Category: "new"}, {Name: "Blocked", Category: "new"},
	}
	names, def := PickedUpCandidates(statuses)
	if !reflect.DeepEqual(names, []string{"Blocked", "Open", "Selected for Development"}) {
		t.Fatalf("names %v", names)
	}
	if def != "Selected for Development" {
		t.Fatalf("default %q", def)
	}
	_, def = PickedUpCandidates([]jira.Status{{Name: "To Do", Category: "new"}})
	if def != "To Do" {
		t.Fatalf("single new status must be the default, got %q", def)
	}
}
