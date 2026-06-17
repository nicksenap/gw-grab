// Package llm asks a local Ollama model to choose a repo and branch name from
// task context. It is used only for the fuzzy gaps — Notion/Slack naming and
// repo inference — never for ground-truth values like a PR's head branch.
//
// Ollama's `format` field constrains decoding to a JSON schema, so the response
// is always schema-valid; model choice only affects the quality of the values.
// Every call is bounded by a hard timeout and degrades to a deterministic
// fallback, so a missing/slow/cold model never blocks a grab.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DefaultEndpoint is Ollama's local API. Override via config.
const DefaultEndpoint = "http://localhost:11434"

// DefaultModel is chosen for strong structured-output + instruction following
// at small size; see README for lighter alternatives (e.g. gemma3:1b).
const DefaultModel = "qwen2.5:3b"

// DefaultTimeout bounds a single inference. Past this we fall back to slugify.
const DefaultTimeout = 4 * time.Second

// Client talks to a local Ollama server.
type Client struct {
	Endpoint string
	Model    string
	Timeout  time.Duration
	HTTP     *http.Client
}

// New returns a Client with defaults filled in for any zero fields.
func New(endpoint, model string, timeout time.Duration) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if model == "" {
		model = DefaultModel
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	return &Client{
		Endpoint: endpoint,
		Model:    model,
		Timeout:  timeout,
		HTTP:     &http.Client{},
	}
}

// Request is the task context handed to the model.
type Request struct {
	// Title/Body describe the task (PR/Notion/Slack text).
	Title string
	Body  string
	// Repos are candidate "owner/repo" or local names the model may pick from.
	Repos []string
	// BranchExamples are existing branch names from the target repo, used as
	// few-shot so the model matches the team's naming convention.
	BranchExamples []string
	// PriorMappings are recent "signal → repo" decisions for few-shot repo hints.
	PriorMappings []string
}

// Suggestion is the model's structured answer.
type Suggestion struct {
	Repo       string  `json:"repo"`       // chosen repo (from Repos), or "" if unsure
	Branch     string  `json:"branch"`     // proposed branch name
	Confidence float64 `json:"confidence"` // 0..1 self-rated confidence in repo
}

// schema is the JSON schema Ollama enforces on the response.
var schema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "repo":       {"type": "string"},
    "branch":     {"type": "string"},
    "confidence": {"type": "number"}
  },
  "required": ["repo", "branch", "confidence"]
}`)

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   json.RawMessage `json:"format"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
}

// Available reports whether the Ollama server responds within a short probe.
// Used to decide whether to attempt LLM resolution or go straight to fallback.
func (c *Client) Available(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", c.Endpoint+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// Suggest asks the model for a repo+branch suggestion. The returned error is
// non-nil only on a real failure (server down, timeout, bad response); callers
// treat any error as "use the deterministic fallback".
func (c *Client) Suggest(ctx context.Context, r Request) (*Suggestion, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	body, err := json.Marshal(ollamaChatRequest{
		Model:  c.Model,
		Stream: false,
		Format: schema,
		Options: map[string]any{
			"temperature": 0.2, // low: we want stable, conventional names
		},
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: r.prompt()},
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.Endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama status %d", resp.StatusCode)
	}

	var chat ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chat); err != nil {
		return nil, err
	}
	var s Suggestion
	if err := json.Unmarshal([]byte(chat.Message.Content), &s); err != nil {
		return nil, fmt.Errorf("decoding suggestion: %w", err)
	}
	return &s, nil
}

const systemPrompt = `You name git branches and pick the target repository for a developer task.
Rules:
- branch: short, lowercase, hyphenated, prefixed by type when clear (feat/, fix/, chore/). Match the style of the provided existing branch examples. No spaces. Under 50 chars.
- repo: choose EXACTLY one value from the provided candidate list that the task most likely belongs to. If none clearly fits, return an empty string.
- confidence: 0..1, your confidence in the repo choice. Use a low value when guessing.
Respond only with the structured JSON.`

func (r Request) prompt() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Task title: %s\n", r.Title)
	if r.Body != "" {
		fmt.Fprintf(&b, "Task details: %s\n", clip(r.Body, 1500))
	}
	if len(r.Repos) > 0 {
		fmt.Fprintf(&b, "\nCandidate repos (pick one, exact value):\n")
		for _, repo := range r.Repos {
			fmt.Fprintf(&b, "- %s\n", repo)
		}
	}
	if len(r.BranchExamples) > 0 {
		fmt.Fprintf(&b, "\nExisting branch names in the likely repo (match this style):\n")
		for _, ex := range r.BranchExamples {
			fmt.Fprintf(&b, "- %s\n", ex)
		}
	}
	if len(r.PriorMappings) > 0 {
		fmt.Fprintf(&b, "\nPast routing of similar sources (signal -> repo):\n")
		for _, m := range r.PriorMappings {
			fmt.Fprintf(&b, "- %s\n", m)
		}
	}
	return b.String()
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
