// Package source turns a pasted URL (GitHub PR, GitLab MR, Notion page, Slack
// message) into a SourceContext: the deterministic facts that can be extracted
// from the URL alone, plus a slot for the metadata a resolver fetches later.
//
// Everything here is offline and dependency-free. Provider resolvers (which may
// shell out to gh/glab or hit a token-authed API) live in sibling files.
package source

import (
	"net/url"
	"regexp"
	"strings"
)

// Provider identifies the kind of URL a grab was seeded from.
type Provider string

const (
	ProviderGitHub  Provider = "github"
	ProviderGitLab  Provider = "gitlab"
	ProviderNotion  Provider = "notion"
	ProviderSlack   Provider = "slack"
	ProviderUnknown Provider = ""
)

// BranchMode mirrors grove's notion of how a branch is provisioned. It is passed
// through to `gw create`: Track checks out an existing remote branch, Create
// makes a new one from base.
type BranchMode string

const (
	BranchModeCreate BranchMode = "create"
	BranchModeTrack  BranchMode = "track"
)

// SourceContext is the resolved view of a pasted URL. Fields are filled in two
// passes: ParseURL sets the deterministic ones (Provider, RawURL, identifiers),
// then a provider Resolver fills Title/Branch/RepoSlug/etc. where it can.
type SourceContext struct {
	Provider Provider
	RawURL   string

	// RepoSlug is "owner/repo" when the URL or resolver yields one (GitHub PR,
	// GitLab MR). Empty for Notion/Slack, where the repo is inferred later.
	RepoSlug string

	// Ref is the provider's stable identifier: PR/MR number, Notion page id, or
	// Slack message ts. Persisted as the workspace source ref.
	Ref string

	// Title is a human-readable description (PR title, Notion page title, first
	// line of a Slack message). May be empty when no metadata is available.
	Title string

	// Branch is the git branch to use. For a PR it is the existing head branch
	// (with BranchModeTrack); for task-style sources it is left empty for the
	// LLM/slug step to fill (BranchModeCreate).
	Branch     string
	BranchMode BranchMode

	// Signal is the stable grouping key used by repo-selection memory:
	// the GitHub/GitLab owner, the Notion workspace slug, or the Slack channel
	// id. Repeated grabs from the same Signal can skip repo inference.
	Signal string

	// Notes carries human-facing degradation messages for the preview
	// (e.g. "gh unavailable — could not resolve PR head branch").
	Notes []string
}

// notionPageID matches the trailing 32 hex chars of a Notion URL (the page id),
// optionally dash-separated inside a longer id.
var notionPageID = regexp.MustCompile(`([0-9a-fA-F]{32})$`)

// slackTS matches a Slack message permalink id like p1781511693141549.
var slackTS = regexp.MustCompile(`^p(\d{10})(\d{6})$`)

// ParseURL inspects arg and, if it is a recognized source URL, returns a
// SourceContext seeded with everything derivable offline. ok is false when arg
// is not a URL we handle (the caller should treat it as a plain name/error).
func ParseURL(arg string) (*SourceContext, bool) {
	arg = strings.TrimSpace(arg)
	if !strings.HasPrefix(arg, "http://") && !strings.HasPrefix(arg, "https://") {
		return nil, false
	}
	u, err := url.Parse(arg)
	if err != nil || u.Host == "" {
		return nil, false
	}

	host := strings.ToLower(u.Host)
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return parseGitHub(u)
	case strings.Contains(host, "gitlab"):
		return parseGitLab(u)
	case host == "notion.so" || strings.HasSuffix(host, ".notion.so") ||
		host == "notion.com" || strings.HasSuffix(host, ".notion.com"):
		return parseNotion(u)
	case strings.HasSuffix(host, ".slack.com") || host == "slack.com":
		return parseSlack(u)
	}
	return nil, false
}

// parseGitHub handles /owner/repo/pull/N (track an existing PR head),
// /owner/repo/tree/<branch> (track a branch directly), and /owner/repo/issues/N
// or bare /owner/repo (new branch from base).
func parseGitHub(u *url.URL) (*SourceContext, bool) {
	parts := splitPath(u.Path)
	if len(parts) < 2 {
		return nil, false
	}
	owner, repo := parts[0], parts[1]
	sc := &SourceContext{
		Provider:   ProviderGitHub,
		RawURL:     u.String(),
		RepoSlug:   owner + "/" + repo,
		Signal:     owner,
		BranchMode: BranchModeCreate,
	}

	if len(parts) >= 4 {
		switch parts[2] {
		case "pull":
			sc.Ref = parts[3]
			sc.BranchMode = BranchModeTrack // head branch resolved by gh later
		case "tree":
			// Branch is literal in the URL — track it directly, no API needed.
			sc.Branch = strings.Join(parts[3:], "/")
			sc.BranchMode = BranchModeTrack
		case "issues":
			sc.Ref = parts[3]
		}
	}
	return sc, true
}

// parseGitLab handles /owner/repo/-/merge_requests/N and /owner/repo/-/tree/<branch>.
func parseGitLab(u *url.URL) (*SourceContext, bool) {
	parts := splitPath(u.Path)
	if len(parts) < 2 {
		return nil, false
	}
	// GitLab supports nested groups; everything before "/-/" is the project path.
	dashIdx := indexOf(parts, "-")
	var projectParts []string
	if dashIdx >= 0 {
		projectParts = parts[:dashIdx]
	} else {
		projectParts = parts
	}
	if len(projectParts) < 2 {
		return nil, false
	}
	slug := strings.Join(projectParts, "/")
	sc := &SourceContext{
		Provider:   ProviderGitLab,
		RawURL:     u.String(),
		RepoSlug:   slug,
		Signal:     projectParts[0],
		BranchMode: BranchModeCreate,
	}
	if dashIdx >= 0 && len(parts) >= dashIdx+3 {
		switch parts[dashIdx+1] {
		case "merge_requests":
			sc.Ref = parts[dashIdx+2]
			sc.BranchMode = BranchModeTrack
		case "tree":
			sc.Branch = strings.Join(parts[dashIdx+2:], "/")
			sc.BranchMode = BranchModeTrack
		}
	}
	return sc, true
}

// parseNotion extracts the 32-hex page id and the human slug that precedes it.
// No API call — the slug alone is a usable title.
func parseNotion(u *url.URL) (*SourceContext, bool) {
	parts := splitPath(u.Path)
	if len(parts) == 0 {
		return nil, false
	}
	last := parts[len(parts)-1]

	sc := &SourceContext{
		Provider:   ProviderNotion,
		RawURL:     u.String(),
		BranchMode: BranchModeCreate,
	}
	// Workspace slug is the first path segment for app.notion.com/<ws>/... URLs.
	if len(parts) >= 2 {
		sc.Signal = parts[0]
	}

	if m := notionPageID.FindStringSubmatch(last); m != nil {
		sc.Ref = m[1]
		// The slug is everything before the trailing (dash-joined) id.
		slug := strings.TrimSuffix(last, m[1])
		slug = strings.TrimRight(slug, "-")
		sc.Title = humanizeSlug(slug)
	} else {
		sc.Title = humanizeSlug(last)
	}
	return sc, true
}

// parseSlack extracts the channel id and message ts. Offline there is no title.
func parseSlack(u *url.URL) (*SourceContext, bool) {
	parts := splitPath(u.Path)
	// .../archives/<channel>/<pTs>
	idx := indexOf(parts, "archives")
	if idx < 0 || len(parts) < idx+3 {
		return nil, false
	}
	channel := parts[idx+1]
	tsRaw := parts[idx+2]

	sc := &SourceContext{
		Provider:   ProviderSlack,
		RawURL:     u.String(),
		Signal:     channel,
		BranchMode: BranchModeCreate,
	}
	if m := slackTS.FindStringSubmatch(tsRaw); m != nil {
		sc.Ref = m[1] + "." + m[2] // Slack API ts form: 1781511693.141549
	}
	return sc, true
}

// --- helpers ---

func splitPath(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			if dec, err := url.PathUnescape(s); err == nil {
				s = dec
			}
			out = append(out, s)
		}
	}
	return out
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

// humanizeSlug turns "The-agent-cannot-see" into "The agent cannot see".
func humanizeSlug(slug string) string {
	slug = strings.ReplaceAll(slug, "-", " ")
	slug = strings.ReplaceAll(slug, "_", " ")
	return strings.TrimSpace(slug)
}
