// Package grove is gw-grab's glue to the Grove CLI. It shells out to `gw` and
// `git` — gw-grab owns no workspace/state logic, it only resolves intent and
// drives `gw create` with the right flags.
package grove

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
// the given repos — the candidate list for the side panel.
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

// CreateOpts are the inputs for `gw create`.
type CreateOpts struct {
	Name           string
	Branch         string
	Repo           string // local repo name(s, comma-separated), or a clone URL for clone-on-miss
	Preset         string // Grove preset; takes precedence over Repo
	Track          bool
	SourceURL      string
	SourceProvider string
	SourceRef      string
	SourceTitle    string
}

// CreateArgs builds the `gw create` argv (without the leading "gw").
func CreateArgs(o CreateOpts) []string {
	args := []string{"create", o.Name, "--branch", o.Branch}
	if o.Preset != "" {
		args = append(args, "--preset", o.Preset)
	} else {
		args = append(args, "--repos", o.Repo)
	}
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
	return args
}

// Create invokes `gw create`. gw owns all workspace/worktree/state work.
//
// In native-host mode stdout is Chrome's message stream and stdin is Chrome's
// request stream, so gw must never inherit either: its output goes to stderr
// (Chrome's log) and is also returned in the error on failure. extraEnv is
// appended to the environment (e.g. GROVE_HERDR_OPEN=0).
func Create(o CreateOpts, extraEnv ...string) error {
	var buf bytes.Buffer
	cmd := exec.Command("gw", CreateArgs(o)...)
	cmd.Stdin = nil
	cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	cmd.Env = append(os.Environ(), extraEnv...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gw create failed: %w: %s", err, tail(buf.String(), 600))
	}
	return nil
}

// Presets returns Grove presets as name → repo names (`gw preset list --json`).
func Presets() (map[string][]string, error) {
	out, err := exec.Command("gw", "preset", "list", "--json").Output()
	if err != nil {
		return nil, err
	}
	return ParsePresets(out)
}

// ParsePresets decodes `gw preset list --json` output.
func ParsePresets(data []byte) (map[string][]string, error) {
	var raw map[string]struct {
		Repos []string `json:"repos"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(raw))
	for name, p := range raw {
		out[name] = p.Repos
	}
	return out, nil
}

// Workspace mirrors the parts of one `gw list --json` entry gw-grab needs.
type Workspace struct {
	Name  string          `json:"name"`
	Path  string          `json:"path"`
	Repos []WorkspaceRepo `json:"repos"`
}

// WorkspaceRepo is one repo worktree inside a workspace.
type WorkspaceRepo struct {
	RepoName     string `json:"repo_name"`
	WorktreePath string `json:"worktree_path"`
}

// FindWorkspace returns the workspace with the given name from `gw list --json`.
func FindWorkspace(name string) (*Workspace, error) {
	out, err := exec.Command("gw", "list", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("gw list: %w", err)
	}
	return ParseWorkspace(out, name)
}

// ParseWorkspace picks one workspace by name out of `gw list --json` output.
func ParseWorkspace(data []byte, name string) (*Workspace, error) {
	var all []Workspace
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Name == name {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("workspace %q not found in gw list", name)
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}
