// Package resolver turns a browser Capture into a concrete Proposal
// (repo + branch + workspace name). Everything is deterministic: URL identity,
// a DOM branch hint, and a slug of the title. Nothing is guessed; when no repo
// can be read off the source, the side panel's picker decides.
package resolver

import (
	"strings"

	"github.com/nicksenap/gw-grab/internal/capture"
	"github.com/nicksenap/gw-grab/internal/grove"
	"github.com/nicksenap/gw-grab/internal/source"
)

// Proposal is the resolved plan used to prefill the side panel and `gw create`.
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
	Notes       []string `json:"notes"`      // info messages
}

// Resolve builds a Proposal from a Capture and the repos Grove knows about.
func Resolve(cap capture.Capture, repos []grove.Repo) Proposal {
	p := Proposal{Candidates: grove.Names(repos)}

	// 1. Deterministic identity from the URL (provider, owner/repo, ref).
	sc, ok := source.ParseURL(cap.URL)
	if !ok {
		// Unknown host: still usable as a generic task using the page title.
		sc = &source.SourceContext{
			Provider:   source.ProviderUnknown,
			RawURL:     cap.URL,
			BranchMode: source.BranchModeCreate,
		}
	}
	// The DOM title wins over anything parsed from the URL.
	if cap.Title != "" {
		sc.Title = cap.Title
	}
	p.Provider = string(sc.Provider)
	p.SourceURL = sc.RawURL
	p.SourceRef = sc.Ref
	p.SourceTitle = sc.Title

	// 2. Branch: a DOM hint (e.g. PR head ref) or a /tree/<branch> URL is ground
	// truth → track it.
	if cap.BranchHint != "" {
		p.Branch, p.Track = cap.BranchHint, true
	} else if sc.Branch != "" {
		p.Branch, p.Track = sc.Branch, true
	}

	// 3. Repo from an explicit slug (GitHub/GitLab pages carry owner/repo).
	if sc.RepoSlug != "" {
		p.RepoDisplay = sc.RepoSlug
		if name, ok := grove.MatchRepoSlug(repos, sc.RepoSlug); ok {
			p.RepoName = name
		} else {
			p.RepoName = cloneURL(sc) // grove clones on miss
			p.Notes = append(p.Notes, sc.RepoSlug+" not cloned locally — will clone")
		}
	}

	// 4. Otherwise name the branch from the title.
	if p.Branch == "" {
		p.Branch = slugBranch(sc)
	}
	p.Name = deriveName(p.Branch)
	return p
}

// slugBranch builds a branch name from the title, falling back to a
// provider-prefixed id.
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
		if sc.Ref != "" {
			return source.Slugify("slack "+firstField(sc.Ref, "."), 50)
		}
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
