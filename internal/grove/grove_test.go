package grove

import (
	"strings"
	"testing"
)

func sampleRepos() []Repo {
	return []Repo{
		{Name: "conversational-analytics", DisplayName: "funnel-io/conversational-analytics"},
		{Name: "api", DisplayName: "acme/api"},
		{Name: "web", DisplayName: ""},
	}
}

func TestMatchRepoSlug_ByDisplayName(t *testing.T) {
	repo, ok := MatchRepoSlug(sampleRepos(), "funnel-io/conversational-analytics")
	if !ok || repo != "conversational-analytics" {
		t.Errorf("got %q, %v", repo, ok)
	}
}

func TestMatchRepoSlug_CaseInsensitive(t *testing.T) {
	repo, ok := MatchRepoSlug(sampleRepos(), "Acme/API")
	if !ok || repo != "api" {
		t.Errorf("got %q, %v", repo, ok)
	}
}

func TestMatchRepoSlug_FallbackToFolderName(t *testing.T) {
	// No display-name match, but the repo half matches a folder name.
	repo, ok := MatchRepoSlug(sampleRepos(), "someone-else/web")
	if !ok || repo != "web" {
		t.Errorf("got %q, %v", repo, ok)
	}
}

func TestMatchRepoSlug_NoMatch(t *testing.T) {
	if _, ok := MatchRepoSlug(sampleRepos(), "x/y"); ok {
		t.Error("expected no match")
	}
}

func TestNames(t *testing.T) {
	names := Names(sampleRepos())
	want := []string{"funnel-io/conversational-analytics", "acme/api", "web"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestCreateArgs_PresetWinsOverRepo(t *testing.T) {
	got := strings.Join(CreateArgs(CreateOpts{Name: "n", Branch: "b", Repo: "api", Preset: "funnel-ai-full", SourceURL: "https://x"}), " ")
	want := "create n --branch b --preset funnel-ai-full --source-url https://x"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestCreateArgs_Repo(t *testing.T) {
	got := strings.Join(CreateArgs(CreateOpts{Name: "n", Branch: "b", Repo: "api", Track: true}), " ")
	if got != "create n --branch b --repos api --track" {
		t.Errorf("got %q", got)
	}
}

func TestParsePresets(t *testing.T) {
	p, err := ParsePresets([]byte(`{"funnel-ai-full":{"repos":["saturn","conversational-analytics"]}}`))
	if err != nil || len(p["funnel-ai-full"]) != 2 || p["funnel-ai-full"][0] != "saturn" {
		t.Fatalf("got %v, %v", p, err)
	}
}

func TestParseWorkspace(t *testing.T) {
	data := []byte(`[{"name":"a","path":"/w/a","repos":[{"repo_name":"saturn","worktree_path":"/w/a/saturn"}]},{"name":"b","path":"/w/b"}]`)
	ws, err := ParseWorkspace(data, "a")
	if err != nil || ws.Path != "/w/a" || ws.Repos[0].WorktreePath != "/w/a/saturn" {
		t.Fatalf("got %+v, %v", ws, err)
	}
	if _, err := ParseWorkspace(data, "zzz"); err == nil {
		t.Error("expected not-found error")
	}
}
