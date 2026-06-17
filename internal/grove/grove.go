// Package grove is gw-grab's glue to the Grove CLI. It shells out to `gw` and
// `git` — gw-grab owns no workspace/state logic, it only resolves intent and
// drives `gw create` with the right flags.
package grove

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

// Repo mirrors one entry of `gw repos --json`.
type Repo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Remote      string `json:"remote"`
	DisplayName string `json:"display_name"` // "owner/repo" from the remote
}

// Repos returns the repos Grove has discovered (name + remote identity), used to
// map a PR's owner/repo to a local clone and to offer candidates to the model.
func Repos() ([]Repo, error) {
	out, err := exec.Command("gw", "repos", "--json").Output()
	if err != nil {
		return nil, err
	}
	var repos []Repo
	if err := json.Unmarshal(out, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// MatchRepoSlug finds the local repo whose remote display name equals
// owner/repo (case-insensitive), falling back to a folder-name match on the
// repo half. Returns the local repo name and whether a match was found.
func MatchRepoSlug(repos []Repo, slug string) (string, bool) {
	slug = strings.ToLower(slug)
	tail := slug
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		tail = slug[i+1:]
	}
	for _, r := range repos {
		if strings.ToLower(r.DisplayName) == slug {
			return r.Name, true
		}
	}
	for _, r := range repos {
		if strings.ToLower(r.Name) == tail {
			return r.Name, true
		}
	}
	return "", false
}

// Names returns the display names (owner/repo where known, else folder name) of
// the given repos — the candidate list handed to the model.
func Names(repos []Repo) []string {
	out := make([]string, len(repos))
	for i, r := range repos {
		if r.DisplayName != "" {
			out[i] = r.DisplayName
		} else {
			out[i] = r.Name
		}
	}
	return out
}

// BranchExamples returns up to n recent branch names from a repo's local path,
// used as few-shot so generated names match the team's convention.
func BranchExamples(repoPath string, n int) []string {
	if repoPath == "" {
		return nil
	}
	cmd := exec.Command("git", "-C", repoPath, "for-each-ref",
		"--sort=-committerdate", "--format=%(refname:short)", "refs/heads", "refs/remotes/origin")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var branches []string
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		b := strings.TrimPrefix(strings.TrimSpace(line), "origin/")
		if b == "" || b == "HEAD" || seen[b] {
			continue
		}
		seen[b] = true
		branches = append(branches, b)
		if len(branches) >= n {
			break
		}
	}
	return branches
}

// CreateOpts are the inputs for `gw create`.
type CreateOpts struct {
	Name           string
	Branch         string
	Repo           string // local repo name, or a clone URL for clone-on-miss
	Track          bool
	SourceURL      string
	SourceProvider string
	SourceRef      string
	SourceTitle    string
}

// Create invokes `gw create` with the resolved options, streaming its output to
// the user's terminal. gw owns all workspace/worktree/state work.
func Create(o CreateOpts) error {
	args := []string{"create", o.Name, "--repos", o.Repo, "--branch", o.Branch}
	if o.Track {
		args = append(args, "--track")
	}
	if o.SourceURL != "" {
		args = append(args, "--source-url", o.SourceURL)
	}
	if o.SourceProvider != "" {
		args = append(args, "--source-provider", o.SourceProvider)
	}
	if o.SourceRef != "" {
		args = append(args, "--source-ref", o.SourceRef)
	}
	if o.SourceTitle != "" {
		args = append(args, "--source-title", o.SourceTitle)
	}
	cmd := exec.Command("gw", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
