// Package dispatch turns a set of browser captures plus a short instruction
// into a running agent: `gw create` a workspace, open it as a background Herdr
// space, and start the agent there with a brief built from the captures.
package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nicksenap/gw-grab/internal/capture"
	"github.com/nicksenap/gw-grab/internal/grove"
)

// DefaultAgentCmd is the agent launched when the job doesn't name one.
const DefaultAgentCmd = "pi --no-skills --skill-profile pocock"

// maxBody caps how much page text per item goes into the brief.
const maxBody = 1500

// Job is one dispatch request from the side panel.
type Job struct {
	Name     string            `json:"name"`
	Branch   string            `json:"branch"`
	Track    bool              `json:"track"`
	Preset   string            `json:"preset"`  // Grove preset; wins over Repos
	Repos    []string          `json:"repos"`   // used when Preset is empty
	StartIn  string            `json:"startIn"` // repo to start the agent in; "" = workspace root
	AgentCmd string            `json:"agentCmd"`
	Prompt   string            `json:"prompt"`
	Items    []capture.Capture `json:"items"`
}

// Result describes what was created.
type Result struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Herdr     string `json:"herdr"`
	Pane      string `json:"pane"`
	Brief     string `json:"brief"`
}

// Deps are the side-effecting collaborators, injectable for tests.
type Deps struct {
	Create        func(grove.CreateOpts) error
	FindWorkspace func(name string) (*grove.Workspace, error)
	OpenHerdr     func(path, label string) (string, error)
	FirstPane     func(workspaceID string) (string, error)
	RunInPane     func(pane, command string) error
	BriefDir      string
	Progress      func(string)
	Now           func() time.Time
}

// Brief renders the agent's first message: the instruction, then each capture
// as a numbered context item (title, URL, selection or a clipped page body).
func Brief(prompt string, items []capture.Capture) string {
	var b strings.Builder
	prompt = strings.TrimSpace(prompt)
	if prompt != "" {
		b.WriteString(prompt)
		b.WriteString("\n\n")
	}
	if len(items) == 0 {
		return strings.TrimSpace(b.String())
	}
	b.WriteString("Context captured from the browser:\n")
	for i, it := range items {
		title := strings.TrimSpace(it.Title)
		if title == "" {
			title = it.URL
		}
		fmt.Fprintf(&b, "\n%d. %s\n   %s\n", i+1, oneLine(title), it.URL)
		text := strings.TrimSpace(it.Selection)
		if text == "" {
			text = strings.TrimSpace(it.Body)
			if text == oneLine(title) { // Slack-style captures repeat the message as title
				text = ""
			}
		}
		if text != "" {
			if len(text) > maxBody {
				text = text[:maxBody] + "…"
			}
			for _, line := range strings.Split(text, "\n") {
				b.WriteString("   > ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// AgentCommand is the shell line typed into the pane: cd into dir and start the
// agent with the brief file's contents as its initial message.
func AgentCommand(dir, agentCmd, briefPath string) string {
	return fmt.Sprintf("cd %s && %s \"$(cat %s)\"", shellQuote(dir), agentCmd, shellQuote(briefPath))
}

// Validate checks a job before anything is created.
func Validate(j Job) error {
	if j.Name == "" || !validName.MatchString(j.Name) {
		return fmt.Errorf("invalid workspace name %q", j.Name)
	}
	if j.Branch == "" {
		return fmt.Errorf("no branch")
	}
	if j.Preset == "" && len(j.Repos) == 0 {
		return fmt.Errorf("pick a preset or a repo")
	}
	if strings.TrimSpace(j.Prompt) == "" && len(j.Items) == 0 {
		return fmt.Errorf("nothing to dispatch: add a prompt or a capture")
	}
	return nil
}

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Run executes a job end to end. A failure after the workspace exists leaves it
// in place (and says so); nothing is ever deleted.
func Run(j Job, d Deps) (Result, error) {
	progress := d.Progress
	if progress == nil {
		progress = func(string) {}
	}
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	if err := Validate(j); err != nil {
		return Result{}, err
	}
	agent := strings.TrimSpace(j.AgentCmd)
	if agent == "" {
		agent = DefaultAgentCmd
	}

	opts := grove.CreateOpts{
		Name:   j.Name,
		Branch: j.Branch,
		Preset: j.Preset,
		Repo:   strings.Join(j.Repos, ","),
		Track:  j.Track,
	}
	if len(j.Items) > 0 {
		opts.SourceURL = j.Items[0].URL
		opts.SourceTitle = oneLine(j.Items[0].Title)
	}
	progress("Creating workspace " + j.Name + "…")
	if err := d.Create(opts); err != nil {
		return Result{}, err
	}

	res := Result{Workspace: j.Name}
	kept := func(err error) error {
		return fmt.Errorf("%w (workspace %s was created and kept)", err, j.Name)
	}
	ws, err := d.FindWorkspace(j.Name)
	if err != nil {
		return res, kept(err)
	}
	res.Path = ws.Path
	dir := ws.Path
	if j.StartIn != "" {
		found := false
		for _, r := range ws.Repos {
			if r.RepoName == j.StartIn {
				dir, found = r.WorktreePath, true
			}
		}
		if !found {
			return res, kept(fmt.Errorf("repo %q not in workspace", j.StartIn))
		}
	}

	if err := os.MkdirAll(d.BriefDir, 0o755); err != nil {
		return res, kept(err)
	}
	res.Brief = filepath.Join(d.BriefDir, fmt.Sprintf("%s-%s.md", j.Name, now().Format("20060102-150405")))
	if err := os.WriteFile(res.Brief, []byte(Brief(j.Prompt, j.Items)+"\n"), 0o644); err != nil {
		return res, kept(err)
	}

	progress("Opening Herdr space…")
	if res.Herdr, err = d.OpenHerdr(ws.Path, j.Name); err != nil {
		return res, kept(err)
	}
	if res.Pane, err = d.FirstPane(res.Herdr); err != nil {
		return res, kept(err)
	}
	progress("Starting agent in " + res.Pane + "…")
	if err := d.RunInPane(res.Pane, AgentCommand(dir, agent, res.Brief)); err != nil {
		return res, kept(err)
	}
	return res, nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
