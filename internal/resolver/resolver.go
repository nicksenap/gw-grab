// Package resolver turns a browser Capture into a concrete Proposal
// (repo + branch + workspace name) for `gw create`. It is the testable brain of
// gw-grab: deterministic facts first (URL identity, a DOM branch hint, remembered
// signal→repo), then a local LLM fills only the genuinely fuzzy gaps.
package resolver

import (
	"context"
	"strings"

	"github.com/nicksenap/gw-grab/internal/capture"
	"github.com/nicksenap/gw-grab/internal/grove"
	"github.com/nicksenap/gw-grab/internal/llm"
	"github.com/nicksenap/gw-grab/internal/source"
)

// Suggester is the subset of the LLM client the resolver needs. Backed by
// internal/llm in production, faked in tests.
type Suggester interface {
	Available(ctx context.Context) bool
	Suggest(ctx context.Context, r llm.Request) (*llm.Suggestion, error)
}

// Memory is the subset of the signal→repo store the resolver needs.
type Memory interface {
	Lookup(provider, signal string) (string, bool)
	RecentMappings(n int) []string
}

// Deps are the resolver's collaborators (all injectable for testing).
type Deps struct {
	Repos         []grove.Repo
	Memory        Memory
	LLM           Suggester
	MinConfidence float64
	// BranchExamples returns recent branch names for a repo path (few-shot for
	// naming). Injected so tests need no git.
	BranchExamples func(repoPath string, n int) []string
}

// Proposal is the resolved plan shown in the side panel and sent to gw create.
type Proposal struct {
	Provider    string   `json:"provider"`
	RepoName    string   `json:"repo"`        // local repo name, or a clone URL on miss, or ""
	RepoDisplay string   `json:"repoDisplay"` // owner/repo for display
	Branch      string   `json:"branch"`
	Name        string   `json:"name"`  // derived workspace name
	Track       bool     `json:"track"` // check out an existing branch
	SourceURL   string   `json:"sourceUrl"`
	SourceRef   string   `json:"sourceRef"`
	SourceTitle string   `json:"sourceTitle"`
	Candidates  []string `json:"candidates"` // repo options for the panel dropdown
	Notes       []string `json:"notes"`      // degradation/info messages
}

// Resolve builds a Proposal from a Capture. It never errors on missing optional
// inputs (no LLM, no memory hit) — it returns the best plan it can, with notes.
func Resolve(ctx context.Context, cap capture.Capture, d Deps) Proposal {
	p := Proposal{
		Candidates: grove.Names(d.Repos),
	}

	// 1. Deterministic identity from the URL (provider, owner/repo, ref, signal).
	sc, ok := source.ParseURL(cap.URL)
	if !ok {
		// Unknown host: still grabbable as a generic task using the page title.
		sc = &source.SourceContext{
			Provider:   source.ProviderUnknown,
			RawURL:     cap.URL,
			BranchMode: source.BranchModeCreate,
		}
	}
	// The DOM title/body win over anything parsed from the URL slug.
	if cap.Title != "" {
		sc.Title = cap.Title
	}
	p.Provider = string(sc.Provider)
	p.SourceURL = sc.RawURL
	p.SourceRef = sc.Ref
	p.SourceTitle = sc.Title

	// 2. Branch: a DOM hint (e.g. PR head ref) is ground truth → track it.
	if cap.BranchHint != "" {
		p.Branch = cap.BranchHint
		p.Track = true
	} else if sc.Branch != "" { // /tree/<branch> style URL
		p.Branch = sc.Branch
		p.Track = true
	}

	// 3. Repo from an explicit slug (GitHub/GitLab pages carry owner/repo).
	if sc.RepoSlug != "" {
		p.RepoDisplay = sc.RepoSlug
		if name, ok := grove.MatchRepoSlug(d.Repos, sc.RepoSlug); ok {
			p.RepoName = name
		} else {
			p.RepoName = cloneURL(sc) // grove clones on miss
			p.Notes = append(p.Notes, sc.RepoSlug+" not cloned locally — will clone")
		}
	}

	// Fast path: git host with a known branch + repo needs no LLM.
	if p.RepoName != "" && p.Branch != "" {
		p.Name = deriveName(p.Branch)
		return p
	}

	// 4. Deterministic branch fallback from the title.
	if p.Branch == "" {
		p.Branch = slugBranch(sc)
	}

	// 5. Repo from memory (repeat signals skip the LLM entirely).
	if p.RepoName == "" && d.Memory != nil {
		if repo, ok := d.Memory.Lookup(string(sc.Provider), sc.Signal); ok {
			p.RepoName = repo
			p.Notes = append(p.Notes, "repo from memory of past grabs")
		}
	}

	// 6. LLM fills what's left — repo inference and/or a better branch name.
	needsRepo := p.RepoName == "" && len(d.Repos) > 0
	hasTitle := sc.Title != ""
	if (needsRepo || hasTitle) && d.LLM != nil && d.LLM.Available(ctx) {
		applyLLM(ctx, &p, sc, cap, d, needsRepo)
	} else if needsRepo {
		p.Notes = append(p.Notes, "Ollama not reachable — pick a repo in the panel")
	}

	p.Name = deriveName(p.Branch)
	return p
}

func applyLLM(ctx context.Context, p *Proposal, sc *source.SourceContext, cap capture.Capture, d Deps, needsRepo bool) {
	var examples []string
	if d.BranchExamples != nil {
		examples = d.BranchExamples(repoPath(d.Repos, p.RepoName), 15)
	}
	var prior []string
	if d.Memory != nil {
		prior = d.Memory.RecentMappings(10)
	}
	sug, err := d.LLM.Suggest(ctx, llm.Request{
		Title:          sc.Title,
		Body:           cap.Body,
		Repos:          grove.Names(d.Repos),
		BranchExamples: examples,
		PriorMappings:  prior,
	})
	if err != nil {
		p.Notes = append(p.Notes, "local model unavailable — using heuristics")
		return
	}
	if needsRepo && sug.Repo != "" && sug.Confidence >= d.MinConfidence {
		if name, ok := grove.MatchRepoSlug(d.Repos, sug.Repo); ok {
			p.RepoName = name
			p.RepoDisplay = sug.Repo
		}
	}
	if b := source.Slugify(sug.Branch, 50); b != "" {
		p.Branch = b
	}
}

// slugBranch builds a deterministic branch name from the title, falling back to
// a provider-prefixed id.
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
		return source.Slugify("slack "+sc.Signal+" "+firstField(sc.Ref, "."), 50)
	}
	if sc.Ref != "" {
		return source.Slugify(string(sc.Provider)+"-"+sc.Ref, 50)
	}
	return ""
}

func cloneURL(sc *source.SourceContext) string {
	switch sc.Provider {
	case source.ProviderGitHub:
		return "https://github.com/" + sc.RepoSlug + ".git"
	case source.ProviderGitLab:
		return "https://gitlab.com/" + sc.RepoSlug + ".git"
	}
	return ""
}

func repoPath(repos []grove.Repo, name string) string {
	for _, r := range repos {
		if r.Name == name {
			return r.Path
		}
	}
	return ""
}

func deriveName(branch string) string {
	name := strings.ReplaceAll(branch, "/", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return strings.Trim(name, "-")
}

func firstField(s, sep string) string {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i]
	}
	return s
}
