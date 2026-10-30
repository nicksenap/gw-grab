package dispatch

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nicksenap/gw-grab/internal/capture"
	"github.com/nicksenap/gw-grab/internal/grove"
)

func TestBrief(t *testing.T) {
	got := Brief("  try this out  ", []capture.Capture{
		{URL: "https://x.slack.com/archives/C1/p1", Title: "Data chat is slow", Body: "Data chat is slow"},
		{URL: "https://grafana/d/abc", Title: "Latency\n dashboard", Selection: "p99 12s\nspike at 10:00"},
		{URL: "https://grafana/explore", Body: "logs here"},
	})
	want := `try this out

Context captured from the browser:

1. Data chat is slow
   https://x.slack.com/archives/C1/p1

2. Latency dashboard
   https://grafana/d/abc
   > p99 12s
   > spike at 10:00

3. https://grafana/explore
   https://grafana/explore
   > logs here`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBrief_ClipsBody(t *testing.T) {
	got := Brief("", []capture.Capture{{URL: "u", Title: "t", Body: strings.Repeat("a", 3000)}})
	if !strings.Contains(got, strings.Repeat("a", maxBody)+"…") || strings.Contains(got, strings.Repeat("a", maxBody+1)) {
		t.Error("body not clipped to maxBody")
	}
}

func TestAgentCommand_Quotes(t *testing.T) {
	got := AgentCommand("/w/it's", "pi --no-skills", "/b/x.md")
	want := `cd '/w/it'\''s' && pi --no-skills "$(cat '/b/x.md')"`
	if got != want {
		t.Errorf("got %s", got)
	}
}

func TestValidate(t *testing.T) {
	ok := Job{Name: "a-b", Branch: "a-b", Preset: "p", Prompt: "x"}
	if err := Validate(ok); err != nil {
		t.Fatal(err)
	}
	for _, j := range []Job{
		{Name: "", Branch: "b", Preset: "p", Prompt: "x"},
		{Name: "a; rm -rf", Branch: "b", Preset: "p", Prompt: "x"},
		{Name: "a", Branch: "", Preset: "p", Prompt: "x"},
		{Name: "a", Branch: "b", Prompt: "x"},
		{Name: "a", Branch: "b", Preset: "p"},
	} {
		if Validate(j) == nil {
			t.Errorf("expected error for %+v", j)
		}
	}
}

type fake struct {
	created grove.CreateOpts
	ran     string
	openErr error
}

func (f *fake) deps(t *testing.T) Deps {
	return Deps{
		Create: func(o grove.CreateOpts) error { f.created = o; return nil },
		FindWorkspace: func(name string) (*grove.Workspace, error) {
			return &grove.Workspace{Name: name, Path: "/w/" + name,
				Repos: []grove.WorkspaceRepo{{RepoName: "saturn", WorktreePath: "/w/" + name + "/saturn"}}}, nil
		},
		OpenHerdr: func(path, label string) (string, error) { return "wX", f.openErr },
		FirstPane: func(string) (string, error) { return "wX:p1", nil },
		RunInPane: func(_ string, c string) error { f.ran = c; return nil },
		BriefDir:  t.TempDir(),
		Now:       func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
}

func TestRun_HappyPath(t *testing.T) {
	f := &fake{}
	j := Job{Name: "try-x", Branch: "try-x", Preset: "funnel-ai-full", StartIn: "saturn", Prompt: "try this out",
		Items: []capture.Capture{{URL: "https://slack/1", Title: "msg"}}}
	res, err := Run(j, f.deps(t))
	if err != nil {
		t.Fatal(err)
	}
	if f.created.Preset != "funnel-ai-full" || f.created.SourceURL != "https://slack/1" {
		t.Errorf("create opts %+v", f.created)
	}
	if res.Pane != "wX:p1" || !strings.HasSuffix(res.Brief, "try-x-20260102-030405.md") {
		t.Errorf("result %+v", res)
	}
	if !strings.HasPrefix(f.ran, "cd '/w/try-x/saturn' && "+DefaultAgentCmd+` "$(cat `) {
		t.Errorf("ran %s", f.ran)
	}
	brief, _ := os.ReadFile(res.Brief)
	if !strings.HasPrefix(string(brief), "try this out\n") {
		t.Errorf("brief %q", brief)
	}
}

func TestRun_FailureAfterCreateKeepsWorkspace(t *testing.T) {
	f := &fake{openErr: errors.New("boom")}
	_, err := Run(Job{Name: "a", Branch: "a", Preset: "p", Prompt: "x"}, f.deps(t))
	if err == nil || !strings.Contains(err.Error(), "created and kept") {
		t.Errorf("got %v", err)
	}
}

func TestRun_UnknownStartIn(t *testing.T) {
	f := &fake{}
	_, err := Run(Job{Name: "a", Branch: "a", Preset: "p", Prompt: "x", StartIn: "nope"}, f.deps(t))
	if err == nil || !strings.Contains(err.Error(), "not in workspace") {
		t.Errorf("got %v", err)
	}
}
