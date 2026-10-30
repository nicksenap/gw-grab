package resolver

import (
	"context"
	"testing"

	"github.com/nicksenap/gw-grab/internal/capture"
	"github.com/nicksenap/gw-grab/internal/grove"
	"github.com/nicksenap/gw-grab/internal/llm"
)

// --- fakes ---

type fakeLLM struct {
	available bool
	sug       *llm.Suggestion
	err       error
	gotReq    llm.Request
}

func (f *fakeLLM) Available(context.Context) bool { return f.available }
func (f *fakeLLM) Suggest(_ context.Context, r llm.Request) (*llm.Suggestion, error) {
	f.gotReq = r
	return f.sug, f.err
}

type fakeMem struct {
	repo string
	ok   bool
}

func (m fakeMem) Lookup(_, _ string) (string, bool) { return m.repo, m.ok }
func (m fakeMem) RecentMappings(int) []string       { return nil }

func sampleRepos() []grove.Repo {
	return []grove.Repo{
		{Name: "conversational-analytics", Path: "/r/ca", DisplayName: "funnel-io/conversational-analytics"},
		{Name: "api", Path: "/r/api", DisplayName: "acme/api"},
	}
}

// A GitHub PR with a DOM branch hint resolves fully and deterministically —
// no LLM call, tracking mode on.
func TestResolve_GitHubPR_DomBranchHint(t *testing.T) {
	llmStub := &fakeLLM{available: true} // present, but must NOT be used
	p := Resolve(context.Background(), capture.Capture{
		URL:        "https://github.com/funnel-io/conversational-analytics/pull/1172",
		Title:      "Surface data source status",
		BranchHint: "feat/data-source-status",
	}, Deps{Repos: sampleRepos(), LLM: llmStub})

	if p.RepoName != "conversational-analytics" {
		t.Errorf("repo = %q", p.RepoName)
	}
	if p.Branch != "feat/data-source-status" || !p.Track {
		t.Errorf("branch=%q track=%v", p.Branch, p.Track)
	}
	if p.Name != "feat-data-source-status" {
		t.Errorf("name = %q", p.Name)
	}
	if llmStub.gotReq.Title != "" {
		t.Error("LLM should not be called when repo+branch are known")
	}
}

// A Notion task (no repo, no branch) uses the LLM to infer repo + name.
func TestResolve_Notion_UsesLLM(t *testing.T) {
	llmStub := &fakeLLM{
		available: true,
		sug:       &llm.Suggestion{Repo: "acme/api", Branch: "feat/data status", Confidence: 0.9},
	}
	p := Resolve(context.Background(), capture.Capture{
		URL:   "https://app.notion.com/p/ws/Improve-the-data-API-37430a04501f8008b6e9c63e43d9a698",
		Title: "Improve the data API",
		Body:  "We need to add pagination to the data endpoints.",
	}, Deps{Repos: sampleRepos(), LLM: llmStub, MinConfidence: 0.55})

	if p.RepoName != "api" {
		t.Errorf("repo = %q, want api (from LLM)", p.RepoName)
	}
	if p.Branch != "feat-data-status" { // slugified
		t.Errorf("branch = %q", p.Branch)
	}
	if p.Track {
		t.Error("notion task should not be track mode")
	}
	if llmStub.gotReq.Body == "" {
		t.Error("LLM should receive the page body for inference")
	}
}

// Low confidence → do not auto-pick a repo; leave it for the panel.
func TestResolve_LowConfidenceLeavesRepoEmpty(t *testing.T) {
	llmStub := &fakeLLM{
		available: true,
		sug:       &llm.Suggestion{Repo: "acme/api", Branch: "feat/x", Confidence: 0.3},
	}
	p := Resolve(context.Background(), capture.Capture{
		URL:   "https://app.notion.com/p/ws/Thing-37430a04501f8008b6e9c63e43d9a698",
		Title: "Thing",
	}, Deps{Repos: sampleRepos(), LLM: llmStub, MinConfidence: 0.55})

	if p.RepoName != "" {
		t.Errorf("repo = %q, want empty on low confidence", p.RepoName)
	}
	if len(p.Candidates) != 2 {
		t.Errorf("candidates = %v", p.Candidates)
	}
}

// Memory short-circuits repo selection without calling the LLM for repo.
func TestResolve_MemoryHitSkipsRepoInference(t *testing.T) {
	llmStub := &fakeLLM{available: true, sug: &llm.Suggestion{Branch: "feat/from-llm"}}
	p := Resolve(context.Background(), capture.Capture{
		URL:   "https://team.slack.com/archives/C0FT37YHZ/p1781511693141549",
		Title: "Fix the thing",
	}, Deps{
		Repos:  sampleRepos(),
		Memory: fakeMem{repo: "api", ok: true},
		LLM:    llmStub,
	})
	if p.RepoName != "api" {
		t.Errorf("repo = %q, want api from memory", p.RepoName)
	}
}

// No LLM available and no repo hint → empty repo + a helpful note, no crash.
func TestResolve_NoLLMNoRepo(t *testing.T) {
	p := Resolve(context.Background(), capture.Capture{
		URL:   "https://app.notion.com/p/ws/Thing-37430a04501f8008b6e9c63e43d9a698",
		Title: "Thing to do",
	}, Deps{Repos: sampleRepos(), LLM: &fakeLLM{available: false}})

	if p.RepoName != "" {
		t.Errorf("repo = %q", p.RepoName)
	}
	if p.Branch != "thing-to-do" {
		t.Errorf("branch = %q, want slug fallback", p.Branch)
	}
	if len(p.Notes) == 0 {
		t.Error("expected a note about picking a repo")
	}
}

// Unknown host still produces a usable task workspace from the title.
func TestResolve_UnknownHost(t *testing.T) {
	p := Resolve(context.Background(), capture.Capture{
		URL:   "https://example.com/tickets/42",
		Title: "Investigate latency",
	}, Deps{Repos: sampleRepos(), LLM: &fakeLLM{available: false}})

	if p.Branch != "investigate-latency" {
		t.Errorf("branch = %q", p.Branch)
	}
	if p.SourceURL != "https://example.com/tickets/42" {
		t.Errorf("source url = %q", p.SourceURL)
	}
}
