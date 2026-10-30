// Package herdr is gw-grab's glue to Herdr: open a Grove workspace as a
// background Herdr space (via the grove-herdr helper, so `gw delete` closes it),
// find its pane, and run a command in it. It never focuses or switches the
// user's view.
package herdr

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultHelper is where grove-herdr installs its workspace helper.
func DefaultHelper() string {
	if v := os.Getenv("GW_GRAB_HERDR_HELPER"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "grove-herdr", "hooks", "herdr-workspace.sh")
}

var openedRe = regexp.MustCompile(`herdr-workspace: \w+ (\S+) \(`)

// ParseOpened extracts the Herdr workspace id from the helper's output
// ("herdr-workspace: opened wDP (label) at /path" or "... reused wDP (label)").
func ParseOpened(out string) (string, bool) {
	m := openedRe.FindStringSubmatch(out)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Open opens (or reuses) a background Herdr space for a workspace path and
// returns its workspace id. It refuses to run if the helper lacks --no-focus,
// rather than stealing the user's focus.
func Open(helper, path, label string) (string, error) {
	usage, _ := exec.Command(helper).CombinedOutput()
	if !strings.Contains(string(usage), "--no-focus") {
		return "", fmt.Errorf("%s has no --no-focus option; update grove-herdr", helper)
	}
	out, err := exec.Command(helper, "open", "--no-focus", path, label).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("herdr open: %w: %s", err, strings.TrimSpace(string(out)))
	}
	id, ok := ParseOpened(string(out))
	if !ok {
		return "", fmt.Errorf("herdr open: no workspace id in output: %s", strings.TrimSpace(string(out)))
	}
	return id, nil
}

// ParseFirstPane extracts the first pane id from `herdr pane list` JSON.
func ParseFirstPane(data []byte) (string, error) {
	var resp struct {
		Result struct {
			Panes []struct {
				PaneID string `json:"pane_id"`
			} `json:"panes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("herdr pane list: %w", err)
	}
	if len(resp.Result.Panes) == 0 {
		return "", fmt.Errorf("herdr pane list: no panes")
	}
	return resp.Result.Panes[0].PaneID, nil
}

// FirstPane returns the first pane of a Herdr workspace.
func FirstPane(workspaceID string) (string, error) {
	out, err := exec.Command("herdr", "pane", "list", "--workspace", workspaceID).Output()
	if err != nil {
		return "", fmt.Errorf("herdr pane list: %w", err)
	}
	return ParseFirstPane(out)
}

// Run types a shell command into a pane and presses Enter.
func Run(pane, command string) error {
	out, err := exec.Command("herdr", "pane", "run", pane, command).CombinedOutput()
	if err != nil {
		return fmt.Errorf("herdr pane run: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
