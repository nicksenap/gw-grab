package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Config holds optional tokens/settings a resolver may use. All fields are
// optional; resolvers degrade to offline behavior when they are empty.
type Config struct {
	NotionToken string
	SlackToken  string
}

// Resolve enriches sc in place with live metadata where available (PR head
// branch + title, Notion/Slack title). It never returns an error for missing
// credentials or unreachable services — it appends a human-facing note instead,
// so a grab always proceeds with the best available information.
func Resolve(ctx context.Context, sc *SourceContext, cfg Config) {
	switch sc.Provider {
	case ProviderGitHub:
		resolveGitHubPR(ctx, sc, "")
	case ProviderGitLab:
		resolveGitLabMR(ctx, sc)
	case ProviderNotion:
		resolveNotion(ctx, sc, cfg.NotionToken)
	case ProviderSlack:
		resolveSlack(ctx, sc, cfg.SlackToken)
	}
}

// resolveGitHubPR fills the PR head branch + title via the gh CLI. host is the
// GitHub host ("" → github.com; gh honors GH_HOST for enterprise).
func resolveGitHubPR(ctx context.Context, sc *SourceContext, host string) {
	if sc.Ref == "" || sc.BranchMode != BranchModeTrack {
		return // not a PR (bare repo, issue, or tree URL already has its branch)
	}
	if _, err := exec.LookPath("gh"); err != nil {
		sc.Notes = append(sc.Notes, "gh CLI not found — cannot resolve PR head branch; will create a new branch instead")
		sc.BranchMode = BranchModeCreate
		return
	}
	args := []string{"pr", "view", sc.Ref,
		"--json", "number,headRefName,title,headRepositoryOwner",
		"--repo", sc.RepoSlug}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdin = nil
	if host != "" {
		cmd.Env = append(os.Environ(), "GH_HOST="+host)
	}
	out, err := cmd.Output()
	if err != nil {
		sc.Notes = append(sc.Notes, fmt.Sprintf("gh could not read PR #%s — will create a new branch instead", sc.Ref))
		sc.BranchMode = BranchModeCreate
		return
	}
	var pr struct {
		HeadRefName         string `json:"headRefName"`
		Title               string `json:"title"`
		HeadRepositoryOwner struct {
			Login string `json:"login"`
		} `json:"headRepositoryOwner"`
	}
	if err := json.Unmarshal(out, &pr); err != nil {
		sc.Notes = append(sc.Notes, "could not parse gh output — will create a new branch instead")
		sc.BranchMode = BranchModeCreate
		return
	}
	sc.Branch = pr.HeadRefName
	sc.Title = pr.Title
	// Fork PR: the head branch lives on a different owner, so origin/<head>
	// won't exist. grove's --track already falls back, but flag it for the user.
	if owner := ownerOf(sc.RepoSlug); pr.HeadRepositoryOwner.Login != "" && !strings.EqualFold(pr.HeadRepositoryOwner.Login, owner) {
		sc.Notes = append(sc.Notes, fmt.Sprintf("PR head is on fork %q — branch may not exist on origin", pr.HeadRepositoryOwner.Login))
	}
}

// resolveGitLabMR fills the MR source branch + title via the glab CLI.
func resolveGitLabMR(ctx context.Context, sc *SourceContext) {
	if sc.Ref == "" || sc.BranchMode != BranchModeTrack {
		return
	}
	if _, err := exec.LookPath("glab"); err != nil {
		sc.Notes = append(sc.Notes, "glab CLI not found — cannot resolve MR source branch; will create a new branch instead")
		sc.BranchMode = BranchModeCreate
		return
	}
	cmd := exec.CommandContext(ctx, "glab", "mr", "view", sc.Ref, "-R", sc.RepoSlug, "--output", "json")
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		sc.Notes = append(sc.Notes, fmt.Sprintf("glab could not read MR !%s — will create a new branch instead", sc.Ref))
		sc.BranchMode = BranchModeCreate
		return
	}
	var mr struct {
		SourceBranch string `json:"source_branch"`
		Title        string `json:"title"`
	}
	if err := json.Unmarshal(out, &mr); err != nil {
		sc.BranchMode = BranchModeCreate
		return
	}
	sc.Branch = mr.SourceBranch
	sc.Title = mr.Title
}

// resolveNotion upgrades the offline slug title to the canonical page title when
// a token is present. The slug title set by parseNotion is the fallback.
func resolveNotion(ctx context.Context, sc *SourceContext, token string) {
	if token == "" || sc.Ref == "" {
		return // offline slug title already set
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.notion.com/v1/pages/"+sc.Ref, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", "2022-06-28")

	body, err := doJSON(req)
	if err != nil {
		sc.Notes = append(sc.Notes, "Notion API unavailable — using title from URL slug")
		return
	}
	if title := extractNotionTitle(body); title != "" {
		sc.Title = title
	}
}

// resolveSlack fetches the thread's root message text when a token is present,
// giving a descriptive title where the URL alone has none.
func resolveSlack(ctx context.Context, sc *SourceContext, token string) {
	if token == "" || sc.Ref == "" {
		sc.Notes = append(sc.Notes, "Slack message text unavailable — set SLACK_TOKEN for a descriptive name")
		return
	}
	url := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&latest=%s&limit=1&inclusive=true",
		sc.Signal, sc.Ref)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	body, err := doJSON(req)
	if err != nil {
		sc.Notes = append(sc.Notes, "Slack API unavailable — using a generic name")
		return
	}
	var resp struct {
		OK       bool `json:"ok"`
		Messages []struct {
			Text string `json:"text"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &resp) == nil && resp.OK && len(resp.Messages) > 0 {
		sc.Title = firstLine(resp.Messages[0].Text)
	}
}

// --- helpers ---

func doJSON(req *http.Request) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// extractNotionTitle pulls the plain text out of a Notion page's title property,
// which is nested as properties.<name>.title[].plain_text.
func extractNotionTitle(body []byte) string {
	var page struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Title []struct {
				PlainText string `json:"plain_text"`
			} `json:"title"`
		} `json:"properties"`
	}
	if json.Unmarshal(body, &page) != nil {
		return ""
	}
	for _, p := range page.Properties {
		if p.Type == "title" {
			var b strings.Builder
			for _, t := range p.Title {
				b.WriteString(t.PlainText)
			}
			return strings.TrimSpace(b.String())
		}
	}
	return ""
}

func ownerOf(slug string) string {
	if i := strings.Index(slug, "/"); i >= 0 {
		return slug[:i]
	}
	return slug
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
