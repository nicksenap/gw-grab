// Command gw-grab is a Grove plugin and Chrome native-messaging host. The
// companion browser extension captures the page you're on (GitHub PR, Notion
// task, Slack thread, anything) and sends it here; gw-grab resolves a repo +
// branch + name and shells out to `gw create`. Grove core does all the
// worktree/state work.
//
// Page context comes from the already-authenticated browser, so there are no API
// tokens here. A local Ollama model (optional) names branches and infers the
// repo for task-style sources; without it, deterministic heuristics + the side
// panel's repo picker take over.
//
// Modes:
//
//	gw-grab <chrome-extension://ID/>   native-messaging host (Chrome launches this)
//	gw-grab install-host <ext-id>      register the host manifest for Chrome
//	gw-grab --from-capture             read one Capture JSON from stdin (testing)
//	gw-grab --dispatch                 read one dispatch Job JSON from stdin and run it
//	gw-grab --version | -h
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nicksenap/gw-grab/internal/capture"
	"github.com/nicksenap/gw-grab/internal/dispatch"
	"github.com/nicksenap/gw-grab/internal/grove"
	"github.com/nicksenap/gw-grab/internal/herdr"
	"github.com/nicksenap/gw-grab/internal/llm"
	"github.com/nicksenap/gw-grab/internal/memory"
	"github.com/nicksenap/gw-grab/internal/nativemsg"
	"github.com/nicksenap/gw-grab/internal/resolver"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	switch {
	case len(args) == 0 || args[0] == "-h" || args[0] == "--help":
		usage()
	case args[0] == "--version":
		fmt.Println(version)
	case args[0] == "install-host":
		if len(args) < 2 {
			fail("usage: gw grab install-host <chrome-extension-id>")
		}
		if err := installHost(args[1]); err != nil {
			fail(err.Error())
		}
	case args[0] == "--from-capture":
		if err := runFromCapture(os.Stdin); err != nil {
			fail(err.Error())
		}
	case args[0] == "--dispatch":
		ensurePath()
		var j dispatch.Job
		if err := json.NewDecoder(io.LimitReader(os.Stdin, nativemsg.MaxMessageSize)).Decode(&j); err != nil {
			fail("invalid job JSON: " + err.Error())
		}
		resp := runDispatch(j, func(m string) { fmt.Fprintln(os.Stderr, m) })
		out, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Println(string(out))
		if resp.Error != "" {
			os.Exit(1)
		}
	case strings.HasPrefix(args[0], "chrome-extension://"):
		// Chrome launches the host with the extension origin as argv[1].
		runHost()
	default:
		fail("unrecognized invocation; run `gw grab -h`")
	}
}

// deps builds the resolver dependencies from config + live grove/git/LLM.
func deps() resolver.Deps {
	cfg := loadConfig()
	repos, _ := grove.Repos()
	return resolver.Deps{
		Repos:          repos,
		Memory:         memory.Load(memory.DefaultPath()),
		LLM:            llm.New(cfg.ollamaEndpoint, cfg.model, cfg.timeout),
		MinConfidence:  cfg.minConfidence,
		BranchExamples: grove.BranchExamples,
	}
}

// --- native messaging host loop ---

// hostRequest is one message from the extension.
type hostRequest struct {
	Action  string            `json:"action"`  // "resolve" | "create" | "repos" | "options" | "dispatch"
	Capture capture.Capture   `json:"capture"` // for resolve
	Plan    resolver.Proposal `json:"plan"`    // for create (possibly edited in the panel)
	Job     dispatch.Job      `json:"job"`     // for dispatch
}

// hostResponse is one reply to the extension.
type hostResponse struct {
	OK       bool               `json:"ok"`
	Error    string             `json:"error,omitempty"`
	Proposal *resolver.Proposal `json:"proposal,omitempty"`
	Repos    []grove.Repo       `json:"repos,omitempty"`
	// options
	Presets  map[string][]string `json:"presets,omitempty"`
	AgentCmd string              `json:"agentCmd,omitempty"`
	// dispatch: zero or more {progress} messages precede the final reply.
	Progress string           `json:"progress,omitempty"`
	Result   *dispatch.Result `json:"result,omitempty"`
}

func runHost() {
	ensurePath()
	for {
		var req hostRequest
		if err := nativemsg.Read(os.Stdin, &req); err != nil {
			if err == io.EOF {
				return // browser closed the port
			}
			_ = nativemsg.Write(os.Stdout, hostResponse{Error: err.Error()})
			return
		}
		if req.Action == "dispatch" {
			_ = nativemsg.Write(os.Stdout, runDispatch(req.Job, func(msg string) {
				_ = nativemsg.Write(os.Stdout, hostResponse{Progress: msg})
			}))
			continue
		}
		_ = nativemsg.Write(os.Stdout, handle(req))
	}
}

func handle(req hostRequest) hostResponse {
	switch req.Action {
	case "repos":
		repos, err := grove.Repos()
		if err != nil {
			return hostResponse{Error: "gw repos failed: " + err.Error()}
		}
		return hostResponse{OK: true, Repos: repos}

	case "options":
		presets, err := grove.Presets()
		if err != nil {
			return hostResponse{Error: "gw preset list failed: " + err.Error()}
		}
		repos, _ := grove.Repos()
		return hostResponse{OK: true, Presets: presets, Repos: repos, AgentCmd: loadConfig().agentCmd}

	case "resolve":
		p := resolver.Resolve(context.Background(), req.Capture, deps())
		return hostResponse{OK: true, Proposal: &p}

	case "create":
		if err := create(req.Plan); err != nil {
			return hostResponse{Error: err.Error()}
		}
		// Remember the (signal, repo) choice for next time.
		recordMemory(req.Plan)
		return hostResponse{OK: true}

	default:
		return hostResponse{Error: "unknown action: " + req.Action}
	}
}

// create drives `gw create` from a (possibly panel-edited) proposal.
func create(p resolver.Proposal) error {
	if p.RepoName == "" {
		return fmt.Errorf("no repo selected")
	}
	if p.Branch == "" {
		return fmt.Errorf("no branch")
	}
	name := p.Name
	if name == "" {
		name = deriveName(p.Branch)
	}
	return grove.Create(grove.CreateOpts{
		Name:           name,
		Branch:         p.Branch,
		Repo:           p.RepoName,
		Track:          p.Track,
		SourceURL:      p.SourceURL,
		SourceProvider: p.Provider,
		SourceRef:      p.SourceRef,
		SourceTitle:    p.SourceTitle,
	})
}

func recordMemory(p resolver.Proposal) {
	// Only remember real local repos keyed by a provider signal we can recompute.
	if p.Provider == "" || strings.HasPrefix(p.RepoName, "http") {
		return
	}
	mem := memory.Load(memory.DefaultPath())
	_ = mem.Record(p.Provider, signalFor(p), p.RepoName)
}

// runDispatch creates the workspace, opens it as a background Herdr space and
// starts the agent with the brief. GROVE_HERDR_OPEN=0 stops the post_create hook
// from opening a focused space; we open it ourselves with --no-focus.
func runDispatch(j dispatch.Job, progress func(string)) hostResponse {
	if strings.TrimSpace(j.AgentCmd) == "" {
		j.AgentCmd = loadConfig().agentCmd
	}
	helper := herdr.DefaultHelper()
	res, err := dispatch.Run(j, dispatch.Deps{
		Create:        func(o grove.CreateOpts) error { return grove.Create(o, "GROVE_HERDR_OPEN=0") },
		FindWorkspace: grove.FindWorkspace,
		OpenHerdr:     func(path, label string) (string, error) { return herdr.Open(helper, path, label) },
		FirstPane:     herdr.FirstPane,
		RunInPane:     herdr.Run,
		BriefDir:      briefDir(),
		Progress:      progress,
	})
	if err != nil {
		return hostResponse{Error: err.Error(), Result: &res}
	}
	return hostResponse{OK: true, Result: &res}
}

func briefDir() string {
	dir := os.Getenv("GROVE_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".grove")
	}
	return filepath.Join(dir, "grab", "briefs")
}

// ensurePath adds the usual tool dirs: Chrome launches native hosts with a
// minimal PATH, so gw/herdr (Homebrew, ~/go/bin) would not be found.
func ensurePath() {
	home, _ := os.UserHomeDir()
	have := map[string]bool{}
	parts := filepath.SplitList(os.Getenv("PATH"))
	for _, p := range parts {
		have[p] = true
	}
	for _, p := range []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, "go", "bin"), filepath.Join(home, ".local", "bin")} {
		if !have[p] {
			parts = append(parts, p)
		}
	}
	_ = os.Setenv("PATH", strings.Join(parts, string(os.PathListSeparator)))
}

// --- --from-capture (testing / scripting without the browser) ---

func runFromCapture(r io.Reader) error {
	data, err := io.ReadAll(io.LimitReader(r, nativemsg.MaxMessageSize))
	if err != nil {
		return err
	}
	var cap capture.Capture
	if err := json.Unmarshal(data, &cap); err != nil {
		return fmt.Errorf("invalid capture JSON: %w", err)
	}
	p := resolver.Resolve(context.Background(), cap, deps())
	// Print the resolved plan so it can be inspected, then create.
	pretty, _ := json.MarshalIndent(p, "", "  ")
	fmt.Fprintln(os.Stderr, string(pretty))
	if err := create(p); err != nil {
		return err
	}
	recordMemory(p)
	return nil
}

func deriveName(branch string) string {
	name := strings.ReplaceAll(branch, "/", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return strings.Trim(name, "-")
}

func fail(msg string) {
	fmt.Fprintf(os.Stderr, "\033[1;31merror:\033[0m %s\n", msg)
	os.Exit(1)
}

func usage() {
	fmt.Print(`gw grab — create a Grove workspace from your browser

gw-grab is the native-messaging host for the gw-grab Chrome extension.
You normally trigger it from the browser (right-click → "Grab into Grove",
the toolbar icon, or the keyboard shortcut), not from the terminal.

One-time setup:
  gw grab install-host <chrome-extension-id>   register this host with Chrome

Other:
  gw grab --from-capture     read a Capture JSON from stdin and create (testing)
  gw grab --dispatch         read a dispatch Job JSON from stdin: create workspace,
                             open background Herdr space, start the agent
  gw grab --version

See the README for installing the extension and (optional) Ollama model.
`)
}
