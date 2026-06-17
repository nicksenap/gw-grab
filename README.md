# gw-grab

> Paste a link, get a workspace. A [Grove](https://github.com/nicksenap/grove) plugin.

`gw grab <url>` turns a **GitHub PR**, **GitLab MR**, **Notion page**, or **Slack
message** URL into a ready-to-work Grove workspace — figuring out the repo, the
branch, and a sensible name for you.

```console
$ gw grab https://github.com/funnel-io/conversational-analytics/pull/1172

gw grab
  Source : github #1172 — Surface data source status
  Repo   : conversational-analytics
  Branch : feat/data-source-status   [check out existing branch]
  Name   : feat-data-source-status

Create this workspace? [Y]es / [e]dit / [n]o: y
```

For a **GitHub PR** it checks out the PR's *existing* head branch (so you land
right on the code under review). For a **Notion task** or **Slack thread** —
which carry no branch and often no repo hint — it derives a good branch name and
infers the target repo.

## How it works

`gw-grab` only resolves *intent*; [Grove](https://github.com/nicksenap/grove)
core does all the git/worktree/state work. The pipeline:

1. **Parse** the URL into deterministic facts (owner/repo, PR number, page id,
   channel + ts).
2. **Resolve** live metadata: a PR's head branch + title via `gh` (GitLab via
   `glab`); a Notion/Slack title via API token if you set one.
3. **Fill the gaps** that have no single right answer — branch *naming* and repo
   *inference* for Notion/Slack — with a small **local** model (see below).
4. **Preview** the resolved `{repo, branch, name}` and let you accept or edit.
5. Hand off to `gw create … --track --source-url …`.

Ground-truth values (like a PR's real head branch) are **never** guessed by the
model — only the genuinely fuzzy parts are.

### It learns your routing

Each grab remembers which repo a given **signal** (Slack channel, Notion
workspace, GitHub owner) went to, in `$GROVE_DIR/grab-memory.json`. The next grab
from the same channel/workspace pre-selects that repo **without** calling the
model — so grab gets faster and more accurate the more you use it.

## Install

```sh
gw plugin install nicksenap/gw-grab
```

Requires the [Grove CLI](https://github.com/nicksenap/grove) (`gw`) ≥ v1.1.8.

## Setup (this plugin needs a little more than most)

Unlike most Grove plugins, `gw-grab` works best with a couple of extra pieces.
**All of them are optional** — without them, grab still works, just with simpler
heuristics (URL-slug branch names + the repo picker).

### 1. A local model (recommended) — for smart naming & repo inference

`gw-grab` talks to a local [Ollama](https://ollama.com) server. Everything stays
on your machine — your Notion/Slack content never leaves it.

```sh
# install Ollama (https://ollama.com/download), then pull a small model:
ollama pull qwen2.5:3b      # default — best structured-output quality (~1.9 GB)
# or, lighter / faster:
ollama pull gemma3:1b       # ~815 MB
```

If Ollama isn't running or the model isn't pulled, `gw-grab` prints a one-line
note and falls back to deterministic heuristics — it never blocks.

| Model | Size | Notes |
|-------|------|-------|
| `qwen2.5:3b` *(default)* | 1.9 GB | Best instruction-following / repo inference at small size |
| `gemma3:1b` | 815 MB | Lightest; great at naming, lighter on inference |
| `llama3.2:3b` | 2.0 GB | Solid all-rounder, 128K context |

### 2. Tokens (optional) — for richer titles

- **`NOTION_TOKEN`** — fetch a Notion page's real title (otherwise the URL slug
  is used, which is usually fine).
- **`SLACK_TOKEN`** — fetch a Slack message's text for a descriptive branch name
  (otherwise the name is generic, e.g. `slack-c0ft37yhz-1781511693`).

### 3. CLIs (optional) — for PR/MR resolution

- **`gh`** — resolve a GitHub PR's head branch + title. Without it, a PR URL
  falls back to creating a new branch named `pr-<n>`.
- **`glab`** — same, for GitLab MRs.

## Configuration

All via environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `GW_GRAB_MODEL` | `qwen2.5:3b` | Ollama model to use |
| `GW_GRAB_OLLAMA` | `http://localhost:11434` | Ollama endpoint |
| `GW_GRAB_TIMEOUT_MS` | `4000` | Hard cap on a single inference before fallback |
| `GW_GRAB_MIN_CONFIDENCE` | `0.55` | Min model confidence to auto-pick a repo |
| `NOTION_TOKEN` | — | Notion integration token |
| `SLACK_TOKEN` | — | Slack token (needs `channels:history`) |

## Supported URLs

| Source | Example | Branch behavior |
|--------|---------|-----------------|
| GitHub PR | `…/owner/repo/pull/123` | check out existing head |
| GitHub branch | `…/owner/repo/tree/my-branch` | check out existing branch |
| GitHub issue / repo | `…/owner/repo/issues/9` | new branch |
| GitLab MR | `…/group/repo/-/merge_requests/12` | check out existing source branch |
| Notion page | `…/Title-<32hexid>` | new branch (named from title) |
| Slack message | `…/archives/<chan>/p<ts>` | new branch (named from thread) |

## Non-interactive use

In a non-TTY (scripts/CI) `gw-grab` skips the prompt and proceeds with the
resolved values, printing the plan to stderr. If no repo can be determined it
exits non-zero rather than hanging.

## License

MIT
