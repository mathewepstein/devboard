package config

// Template is the commented config written by `devboard init --template`.
func Template() string {
	return `# devboard configuration. Tokens live in the OS keychain, not here.

[jira]
base_url = "https://example.atlassian.net"
email = "you@example.com"
# Project key whose versions and tickets are tracked.
project = "PROJ"
# Account whose mentions and assignments count as "me". Resolved by init.
account_id = ""
display_name = ""
# Status that means a ticket was picked up for development.
picked_up_status = "Selected for Development"

# Set one of labels / board_id+jql / jql, or none.
[jira.scope]
labels = []
board_id = 0
jql = ""

[github]
login = "octocat"
org = "example-org"

# One block per watched repo. prefix is the fixVersion naming prefix
# ("MW" matches "MW 1.11.0"); "*" matches bare numbers like 10.2.0;
# leave "" if the repo has no versions.
[[repos]]
name = "Example Site"
repo = "example-site"
prefix = "EX"

[poll]
interval = "60s"
port = 8765
`
}
