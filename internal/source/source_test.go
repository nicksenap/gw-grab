package source

import "testing"

func TestParseURL_NonURL(t *testing.T) {
	for _, arg := range []string{"my-workspace", "feat/login", "", "git@github.com:o/r.git", "/tmp/x"} {
		if sc, ok := ParseURL(arg); ok {
			t.Errorf("ParseURL(%q) = ok with %+v, want not-a-url", arg, sc)
		}
	}
}

func TestParseURL_GitHubPR(t *testing.T) {
	sc, ok := ParseURL("https://github.com/funnel-io/conversational-analytics/pull/1172")
	if !ok {
		t.Fatal("expected ok")
	}
	if sc.Provider != ProviderGitHub {
		t.Errorf("provider = %q", sc.Provider)
	}
	if sc.RepoSlug != "funnel-io/conversational-analytics" {
		t.Errorf("repo slug = %q", sc.RepoSlug)
	}
	if sc.Ref != "1172" {
		t.Errorf("ref = %q", sc.Ref)
	}
	if sc.BranchMode != BranchModeTrack {
		t.Errorf("branch mode = %q, want track", sc.BranchMode)
	}
}

func TestParseURL_GitHubTree(t *testing.T) {
	sc, ok := ParseURL("https://github.com/o/r/tree/feat/some/deep-branch")
	if !ok {
		t.Fatal("expected ok")
	}
	if sc.Branch != "feat/some/deep-branch" {
		t.Errorf("branch = %q", sc.Branch)
	}
	if sc.BranchMode != BranchModeTrack {
		t.Errorf("branch mode = %q, want track", sc.BranchMode)
	}
}

func TestParseURL_GitHubIssueAndBare(t *testing.T) {
	sc, _ := ParseURL("https://github.com/o/r/issues/42")
	if sc.Ref != "42" || sc.BranchMode != BranchModeCreate {
		t.Errorf("issue: ref=%q mode=%q", sc.Ref, sc.BranchMode)
	}
	bare, _ := ParseURL("https://github.com/o/r")
	if bare.RepoSlug != "o/r" || bare.BranchMode != BranchModeCreate {
		t.Errorf("bare: slug=%q mode=%q", bare.RepoSlug, bare.BranchMode)
	}
}

func TestParseURL_GitLabMR(t *testing.T) {
	sc, ok := ParseURL("https://gitlab.com/group/sub/proj/-/merge_requests/7")
	if !ok {
		t.Fatal("expected ok")
	}
	if sc.RepoSlug != "group/sub/proj" {
		t.Errorf("slug = %q", sc.RepoSlug)
	}
	if sc.Ref != "7" || sc.BranchMode != BranchModeTrack {
		t.Errorf("ref=%q mode=%q", sc.Ref, sc.BranchMode)
	}
}

func TestParseURL_Slack(t *testing.T) {
	sc, ok := ParseURL("https://qwaya.slack.com/archives/C0FT37YHZ/p1781511693141549")
	if !ok {
		t.Fatal("expected ok")
	}
	if sc.Provider != ProviderSlack {
		t.Errorf("provider = %q", sc.Provider)
	}
	if sc.Ref != "1781511693.141549" {
		t.Errorf("ts = %q", sc.Ref)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		in     string
		maxLen int
		want   string
	}{
		{"The agent cannot see and surface the status of the Data Sources", 50, "the-agent-cannot-see-and-surface-the-status-of"},
		{"feat/login flow", 0, "feat-login-flow"},
		{"  Spaces  ", 0, "spaces"},
		{"---weird___chars///", 0, "weird-chars"},
		{"Ünïcödé staff", 0, "ncd-staff"},
		{"", 0, ""},
		{"!!!", 0, ""},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in, tt.maxLen); got != tt.want {
			t.Errorf("Slugify(%q, %d) = %q, want %q", tt.in, tt.maxLen, got, tt.want)
		}
	}
}
