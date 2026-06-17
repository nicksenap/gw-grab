package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nicksenap/gw-grab/internal/grove"
	"github.com/nicksenap/gw-grab/internal/source"
)

// previewAndConfirm shows the resolved plan and lets the user accept or edit it.
// In a non-interactive context it prints the plan and proceeds with the resolved
// values unchanged (so scripts/CI never block). Returns the final values.
func previewAndConfirm(sc *source.SourceContext, repoName, branch, name string, repos []grove.Repo) (string, string, string) {
	printPlan(sc, repoName, branch, name)

	if !isTTY() {
		return repoName, branch, name
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, "\nCreate this workspace? [Y]es / [e]dit / [n]o: ")
		line, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "y", "yes":
			return repoName, branch, name
		case "n", "no":
			fmt.Fprintln(os.Stderr, "Aborted.")
			os.Exit(0)
		case "e", "edit":
			repoName = promptDefault(reader, "Repo", repoName)
			branch = promptDefault(reader, "Branch", branch)
			name = promptDefault(reader, "Name", deriveName(branch))
			printPlan(sc, repoName, branch, name)
		}
	}
}

func printPlan(sc *source.SourceContext, repoName, branch, name string) {
	bold := func(s string) string { return "\033[1m" + s + "\033[0m" }
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%s\n", bold("gw grab"))
	fmt.Fprintf(os.Stderr, "  Source : %s\n", sourceLabel(sc))
	repoShown := repoName
	if strings.HasPrefix(repoName, "http") {
		repoShown = repoName + "  (will clone)"
	} else if repoName == "" {
		repoShown = dim("(none — will prompt)")
	}
	fmt.Fprintf(os.Stderr, "  Repo   : %s\n", repoShown)
	mode := ""
	if sc.BranchMode == source.BranchModeTrack {
		mode = dim("  [check out existing branch]")
	}
	fmt.Fprintf(os.Stderr, "  Branch : %s%s\n", branch, mode)
	fmt.Fprintf(os.Stderr, "  Name   : %s\n", name)
	for _, note := range sc.Notes {
		fmt.Fprintf(os.Stderr, "  %s %s\n", dim("note:"), dim(note))
	}
}

func sourceLabel(sc *source.SourceContext) string {
	parts := []string{string(sc.Provider)}
	if sc.Ref != "" {
		parts = append(parts, "#"+sc.Ref)
	}
	label := strings.Join(parts, " ")
	if sc.Title != "" {
		label += " — " + sc.Title
	}
	return label
}

func promptDefault(r *bufio.Reader, label, def string) string {
	fmt.Fprintf(os.Stderr, "  %s [%s]: ", label, def)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// isTTY reports whether stdin and stderr are both terminals (interactive),
// using a stdlib stat check on the character-device mode to avoid a dependency.
func isTTY() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stderr)
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
