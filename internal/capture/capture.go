// Package capture defines the payload the browser extension sends to the
// gw-grab native-messaging host. The extension reads the already-authenticated,
// already-rendered page (GitHub PR, Slack thread, Grafana, anything) and
// hands over plain context — no API tokens, no provider SDKs on the host side.
package capture

// Capture is one "grab this" request from the extension. Every field except URL
// is best-effort: the content script fills what the page exposes and leaves the
// rest empty, and the resolver copes with whatever it gets.
type Capture struct {
	// URL is the page the user grabbed. Always present; used for deterministic
	// provider + identifier parsing.
	URL string `json:"url"`

	// Title is the page/issue/PR/task title as shown in the DOM.
	Title string `json:"title"`

	// Body is extracted page text (PR description, Slack thread)
	// included in the agent's brief. May be long; the brief clips it.
	Body string `json:"body"`

	// BranchHint is a branch name read directly off the page when the site shows
	// one — e.g. a GitHub PR's head ref. When set it is treated as ground truth
	// and checked out as a tracking branch (no guessing).
	BranchHint string `json:"branchHint"`

	// Selection is any text the user had highlighted when triggering the grab,
	// useful when grabbing a specific comment or paragraph.
	Selection string `json:"selection"`
}
