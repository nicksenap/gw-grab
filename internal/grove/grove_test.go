package grove

import "testing"

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
