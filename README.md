# devboard

Local status page for your repos and tickets. Polls GitHub (GraphQL) and Jira, serves a page on `127.0.0.1`.

The page shows:

- Per repo: live and next fixVersion with their tickets, queued versions, open PRs with review/check/merge state
- PRs needing you: `merge` (yours, approved), `fix` (yours, changes requested / checks failing / conflict), `review` (review requested from you)
- Jira: mentions, new assignments, tickets newly moved to the picked-up status

Items can be dismissed; dismissals persist in `seen.json` and reappear when the PR's state changes.

## Requirements

- Go 1.26
- macOS for `agent` (launchd); `serve` runs anywhere
- GitHub token, Jira API token

## Install

```sh
make install          # go install with version from git describe
devboard init         # interactive setup: org, repos, Jira project/scope, tokens
devboard              # serve (default command)
```

`init` prefills from `ATLASSIAN_EMAIL`, `ATLASSIAN_API_TOKEN`, `GH_TOKEN`, `GITHUB_ORG`; pass `--no-env` to prompt for everything. `init --template` writes a commented config instead. `--force` overwrites an existing config.

## Commands

| Command | Does |
|---|---|
| `devboard` / `devboard serve` | Serve the page. Flags: `--port` (default 8765), `--interval` (default 60s) |
| `devboard init` | First-run setup |
| `devboard doctor` | Check config, tokens, Jira, GitHub, agent, running page |
| `devboard agent install` | Write and load the LaunchAgent `local.devboard` |
| `devboard agent uninstall` | Unload and remove it |
| `devboard agent status [--wait 20s]` | Agent state and last poll |

## Config

`~/.config/devboard/config.toml` (or `$XDG_CONFIG_HOME/devboard/`, or `$DEVBOARD_CONFIG`).

```toml
[jira]
base_url = "https://example.atlassian.net"
email = "you@example.com"
project = "PROJ"
account_id = ""
display_name = ""
picked_up_status = "Selected for Development"

[jira.scope]          # one of labels / board_id / jql, or none
labels = []
board_id = 0
jql = ""

[github]
login = "octocat"
org = "example-org"

[[repos]]
name = "Example Site"
repo = "example-site"
prefix = "EX"         # "EX 1.2.0"; "*" = bare numbers; "" = no versions

[poll]
interval = "60s"
port = 8765
```

## Tokens

Resolved in order, first match wins:

- Jira: `DEVBOARD_JIRA_TOKEN`, `ATLASSIAN_API_TOKEN`, keychain `devboard:jira`
- GitHub: `DEVBOARD_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, keychain `devboard:github`

The LaunchAgent has no shell env, so it needs keychain tokens; `agent install` warns if they're missing.

GitHub polling is budgeted to 60% of the GraphQL hourly limit; the interval stretches if a poll costs more.

## Files

| Path | Holds |
|---|---|
| `~/.config/devboard/config.toml` | Config |
| `~/.local/share/devboard/seen.json` | Dismissed items |
| `~/Library/LaunchAgents/local.devboard.plist` | Agent plist |
| `~/Library/Logs/devboard/devboard.log` | Agent log |

## Development

```sh
make run ARGS="serve --port 9000"
make check            # vet + test
make reload           # install, restart agent, wait for first poll
```

Frontend is plain HTML/CSS/JS in `web/`, embedded into the binary.
