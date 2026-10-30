# gw-grab

> Collect a few links, dispatch an agent. A [Grove](https://github.com/nicksenap/grove) plugin + Chrome extension.

You're reading a Slack thread and two Grafana panels and want a coding agent to
look into it. Right-click each one → **Add to Grove dispatch**, type "try this
out…" in the side panel, hit **Dispatch**. gw-grab then:

1. runs `gw create` with the preset or repo you picked,
2. opens the workspace as a **background** [Herdr](https://herdr.dev) space (your focus doesn't move),
3. starts the agent there (default `pi --no-skills --skill-profile pocock`) with a brief:
   your instruction plus every captured link, title and selection.

```text
try this out

Context captured from the browser:

1. Data chat is slow on stage
   https://acme.slack.com/archives/C1/p1781511693141549
   > p99 is 12s since 10:00

2. Latency dashboard
   https://grafana.acme.dev/d/abc
```

Page content comes from your already-signed-in browser, so there are no API
tokens to set up.

## Install

Requires `gw` ≥ v1.1.8, `herdr`, and the [grove-herdr](https://github.com/nicksenap/grove-herdr)
helper with `--no-focus` support. macOS or Linux, Chrome.

```sh
# 1. The native host (also gives you `gw grab`)
go build -o ~/.grove/plugins/gw-grab .

# 2. The extension
cd extension && pnpm install && pnpm build
#    chrome://extensions → Developer mode → Load unpacked → extension/dist

# 3. Let Chrome talk to the host (the id is fixed by the manifest key)
gw grab install-host "$(cat extension/EXTENSION_ID)"
```

## Use

| Action | How |
|---|---|
| Add a page | Right-click the page → **Add to Grove dispatch**, ⌘⇧G, or **+ Add this tab** in the panel |
| Add a link (e.g. a Grafana URL in Slack) | Right-click the link → **Add to Grove dispatch** (the link itself is captured, not the page) |
| Add a quote | Select text, right-click → **Add to Grove dispatch** |
| Dispatch | Write the instruction, pick preset/repo and where the agent starts, **Dispatch** (⌘↵) |

The panel remembers your last preset/repo, start directory, and agent command.
A GitHub PR as the first capture preselects its repo and checks out the PR's
existing branch. The list survives closing the panel and is cleared after a
successful dispatch.

Briefs are written to `~/.grove/grab/briefs/` (outside the worktree). If
anything fails after the workspace is created, the workspace is kept and the
panel says so; gw-grab never deletes anything.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `GW_GRAB_AGENT_CMD` | `pi --no-skills --skill-profile pocock` | Agent started when the panel doesn't set one |
| `GW_GRAB_HERDR_HELPER` | `~/.local/share/grove-herdr/hooks/herdr-workspace.sh` | grove-herdr helper path |

Chrome starts the host with a minimal `PATH`; the host adds `/opt/homebrew/bin`,
`/usr/local/bin`, `~/go/bin` and `~/.local/bin` so `gw` and `herdr` resolve.

## Without the browser

```sh
echo '{"name":"try-it","branch":"try-it","preset":"funnel-ai-full",
       "startIn":"conversational-analytics","prompt":"try this out",
       "items":[{"url":"https://grafana.acme.dev/d/abc","title":"Latency"}]}' \
  | gw grab --dispatch
```

## Development

```sh
go test ./... && go vet ./...
cd extension && pnpm typecheck && pnpm test && pnpm e2e
```

## License

MIT
