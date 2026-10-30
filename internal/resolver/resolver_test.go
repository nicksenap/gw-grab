package resolver

import (
	"testing"

	"github.com/nicksenap/gw-grab/internal/capture"
	"github.com/nicksenap/gw-grab/internal/grove"
)

func sampleRepos() []grove.Repo {
	return []grove.Repo{
		{Name: "conversational-analytics", Path: "/r/ca", DisplayName: "funnel-io/conversational-analytics"},
		{Name: "api", Path: "/r/api", DisplayName: "acme/api"},
	}
}

// A GitHub PR with a DOM branch hint resolves fully: local repo, tracked branch.
func TestResolve_GitHubPR_DomBranchHint(t *testing.T) {
	p := Resolve(capture.Capture{
		URL:        "https://github.com/funnel-io/conversational-analytics/pull/1172",
		Title:      "Surface data source status",
		BranchHint: "feat/data-source-status",
	}, sampleRepos())

	if p.RepoName != "conversational-analytics" {
		t.Errorf("repo = %q", p.RepoName)
	}
	if p.Branch != "feat/data-source-status" || !p.Track {
		t.Errorf("branch=%q track=%v", p.Branch, p.Track)
	}
	if p.Name != "feat-data-source-status" {
		t.Errorf("name = %q", p.Name)
	}
}

// A PR for a repo that isn't cloned proposes cloning it.
func TestResolve_GitHubPR_NotCloned(t *testing.T) {
	p := Resolve(capture.Capture{URL: "https://github.com/someone/else/pull/3"}, sampleRepos())
	if p.RepoName != "https://github.com/someone/else.git" || len(p.Notes) != 1 {
		t.Errorf("repo=%q notes=%v", p.RepoName, p.Notes)
	}
	if p.Branch != "pr-3" {
		t.Errorf("branch = %q", p.Branch)
	}
}

// A Slack message has no repo; the branch comes from the message text, or the ts.
func TestResolve_Slack(t *testing.T) {
	p := Resolve(capture.Capture{
		URL:   "https://qwaya.slack.com/archives/C0FT37YHZ/p1781511693141549",
		Title: "Data chat is slow on stage",
	}, sampleRepos())
	if p.RepoName != "" || p.Track {
		t.Errorf("repo=%q track=%v", p.RepoName, p.Track)
	}
	if p.Branch != "data-chat-is-slow-on-stage" || p.Provider != "slack" || p.SourceRef != "1781511693.141549" {
		t.Errorf("%+v", p)
	}

	p = Resolve(capture.Capture{URL: "https://qwaya.slack.com/archives/C0FT37YHZ/p1781511693141549"}, nil)
	if p.Branch != "slack-1781511693" {
		t.Errorf("branch = %q", p.Branch)
	}
}

// Unknown host still produces a usable task workspace from the title.
func TestResolve_UnknownHost(t *testing.T) {
	p := Resolve(capture.Capture{
		URL:   "https://grafana.example/d/abc",
		Title: "Investigate latency",
	}, sampleRepos())

	if p.Branch != "investigate-latency" || p.Name != "investigate-latency" {
		t.Errorf("branch=%q name=%q", p.Branch, p.Name)
	}
	if p.SourceURL != "https://grafana.example/d/abc" || len(p.Candidates) != 2 {
		t.Errorf("%+v", p)
	}
}
