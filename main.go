// Command gw-grab is a Grove plugin: `gw grab <url>` turns a GitHub PR, GitLab
// MR, Notion page, or Slack message URL into a ready-to-work workspace.
//
// It resolves intent (repo + branch + name) and shells out to `gw create` —
// Grove core does all workspace/worktree/state work. Ground-truth values (a PR's
// head branch) are extracted deterministically; the fuzzy gaps (Notion/Slack
// branch naming and repo inference) are filled by a local Ollama model, which
// degrades to a slug + repo picker when unavailable.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nicksenap/gw-grab/internal/grove"
	"github.com/nicksenap/gw-grab/internal/llm"
	"github.com/nicksenap/gw-grab/internal/memory"
	"github.com/nicksenap/gw-grab/internal/source"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		usage()
		os.Exit(0)
	}
	if args[0] == "--version" {
		fmt.Println(version)
		os.Exit(0)
	}

	if err := run(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "\033[1;31merror:\033[0m %s\n", err)
		os.Exit(1)
	}
}

var version = "dev"

func run(url string) error {
	cfg := loadConfig()

	// 1. Parse the URL into deterministic facts.
	sc, ok := source.ParseURL(url)
	if !ok {
		return fmt.Errorf("not a recognized source URL: %s\nSupported: GitHub PR, GitLab MR, Notion page, Slack message", url)
	}

	// 2. Resolve live metadata (PR head branch + title, Notion/Slack title).
	ctx := context.Background()
	source.Resolve(ctx, sc, source.Config{
		NotionToken: cfg.notionToken,
		SlackToken:  cfg.slackToken,
	})

	// 3. Determine repo + branch (deterministic where possible, else memory/LLM).
	repos, _ := grove.Repos()
	mem := memory.Load(memory.DefaultPath())

	repoName, branch := resolve(ctx, cfg, sc, repos, mem)

	// 4. Derive a workspace name from the branch (grove derives this too, but we
	//    want it visible in the preview).
	name := deriveName(branch)

	// 5. Preview + confirm (interactive only).
	repoName, branch, name = previewAndConfirm(sc, repoName, branch, name, repos)
	if branch == "" {
		return fmt.Errorf("no branch resolved; aborting")
	}
	if repoName == "" {
		return fmt.Errorf("no repo resolved; pass one or pick interactively")
	}

	// 6. Remember the signal→repo decision for next time.
	_ = mem.Record(string(sc.Provider), sc.Signal, repoName)

	// 7. Hand off to grove.
	track := sc.BranchMode == source.BranchModeTrack
	return grove.Create(grove.CreateOpts{
		Name:           name,
		Branch:         branch,
		Repo:           repoName,
		Track:          track,
		SourceURL:      sc.RawURL,
		SourceProvider: string(sc.Provider),
		SourceRef:      sc.Ref,
		SourceTitle:    sc.Title,
	})
}

// resolve decides the repo and branch, using the cheapest correct source first:
//   - branch: an already-resolved (PR/tree) branch wins; otherwise slug, with an
//     LLM upgrade when available.
//   - repo: a slug match (PR/MR) wins; else remembered signal→repo; else LLM
//     inference; else empty (the picker handles it).
func resolve(ctx context.Context, cfg config, sc *source.SourceContext, repos []grove.Repo, mem *memory.Store) (repoName, branch string) {
	// Repo from an explicit slug (GitHub PR / GitLab MR).
	if sc.RepoSlug != "" {
		if name, ok := grove.MatchRepoSlug(repos, sc.RepoSlug); ok {
			repoName = name
		} else {
			// Not cloned locally — hand grove the clone URL (it clones on miss).
			repoName = httpsCloneURL(sc)
		}
	}

	// Ground-truth branch already known (PR head / tree branch).
	if sc.Branch != "" {
		branch = sc.Branch
	}

	// If both are settled, we're done — no LLM needed (the common PR case).
	if repoName != "" && branch != "" {
		return repoName, branch
	}

	// Deterministic name fallback from the title.
	if branch == "" {
		branch = slugBranch(sc)
	}

	// Repo from memory (repeat sources skip the model entirely).
	if repoName == "" {
		if remembered, ok := mem.Lookup(string(sc.Provider), sc.Signal); ok {
			repoName = remembered
		}
	}

	// LLM only fills what's still missing, and only if it's actually up.
	needsRepo := repoName == "" && len(repos) > 0
	needsBetterName := sc.Title != "" // a model name beats a raw slug
	if !needsRepo && !needsBetterName {
		return repoName, branch
	}

	client := llm.New(cfg.ollamaEndpoint, cfg.model, cfg.timeout)
	if !client.Available(ctx) {
		if needsRepo {
			fmt.Fprintln(os.Stderr, dim("(Ollama not reachable — using URL/slug heuristics; set up a model for smarter grabs)"))
		}
		return repoName, branch
	}

	sug, err := client.Suggest(ctx, llm.Request{
		Title:          sc.Title,
		Repos:          grove.Names(repos),
		BranchExamples: grove.BranchExamples(repoPath(repos, repoName), 15),
		PriorMappings:  mem.RecentMappings(10),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, dim("(local model timed out — using slug heuristics)"))
		return repoName, branch
	}

	if needsRepo && sug.Repo != "" && sug.Confidence >= cfg.minConfidence {
		if name, ok := grove.MatchRepoSlug(repos, sug.Repo); ok {
			repoName = name
		}
	}
	if sug.Branch != "" {
		if b := source.Slugify(sug.Branch, 50); b != "" {
			branch = b
		}
	}
	return repoName, branch
}

// slugBranch builds a deterministic branch name from the source title/ref.
func slugBranch(sc *source.SourceContext) string {
	if b := source.Slugify(sc.Title, 50); b != "" {
		return b
	}
	switch sc.Provider {
	case source.ProviderGitHub, source.ProviderGitLab:
		if sc.Ref != "" {
			return "pr-" + sc.Ref
		}
	case source.ProviderSlack:
		return source.Slugify("slack "+sc.Signal+" "+strings.SplitN(sc.Ref, ".", 2)[0], 50)
	}
	return source.Slugify(string(sc.Provider)+"-"+sc.Ref, 50)
}

func repoPath(repos []grove.Repo, name string) string {
	for _, r := range repos {
		if r.Name == name {
			return r.Path
		}
	}
	return ""
}

// httpsCloneURL reconstructs a clone URL for an uncloned GitHub/GitLab repo.
func httpsCloneURL(sc *source.SourceContext) string {
	switch sc.Provider {
	case source.ProviderGitHub:
		return "https://github.com/" + sc.RepoSlug + ".git"
	case source.ProviderGitLab:
		return "https://gitlab.com/" + sc.RepoSlug + ".git"
	}
	return ""
}

func deriveName(branch string) string {
	name := strings.ReplaceAll(branch, "/", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return strings.Trim(name, "-")
}

func dim(s string) string { return "\033[2m" + s + "\033[0m" }

func usage() {
	fmt.Print(`gw grab — create a workspace from a PR, task, or thread URL

Usage:
  gw grab <url>

Supported URLs:
  GitHub PR     https://github.com/owner/repo/pull/123
  GitLab MR     https://gitlab.com/group/repo/-/merge_requests/123
  Notion page   https://www.notion.so/.../Title-<pageid>
  Slack message https://team.slack.com/archives/<channel>/p<ts>

Setup:
  Requires the Grove CLI (gw). For smart branch naming and repo inference on
  Notion/Slack URLs, install Ollama and pull a small model (see README).
  Optional: NOTION_TOKEN / SLACK_TOKEN for richer titles.
`)
}
