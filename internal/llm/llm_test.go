package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSuggest_HappyPath(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &gotBody)
		// Echo a valid structured suggestion as Ollama would.
		json.NewEncoder(w).Encode(ollamaChatResponse{
			Message: ollamaMessage{Role: "assistant", Content: `{"repo":"api","branch":"feat/data-status","confidence":0.9}`},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model", time.Second)
	s, err := c.Suggest(context.Background(), Request{
		Title: "Surface data source status",
		Repos: []string{"api", "web"},
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if s.Repo != "api" || s.Branch != "feat/data-status" || s.Confidence != 0.9 {
		t.Errorf("suggestion = %+v", s)
	}
	// The request must carry the JSON schema in `format` (structured output).
	if gotBody["format"] == nil {
		t.Error("request missing format schema")
	}
	if gotBody["model"] != "test-model" {
		t.Errorf("model = %v", gotBody["model"])
	}
}

func TestSuggest_ServerErrorIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := New(srv.URL, "m", time.Second)
	if _, err := c.Suggest(context.Background(), Request{Title: "x"}); err == nil {
		t.Error("expected error on 500 (caller falls back)")
	}
}

func TestSuggest_TimeoutIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(ollamaChatResponse{Message: ollamaMessage{Content: `{}`}})
	}))
	defer srv.Close()

	c := New(srv.URL, "m", 20*time.Millisecond) // shorter than handler
	if _, err := c.Suggest(context.Background(), Request{Title: "x"}); err == nil {
		t.Error("expected timeout error")
	}
}

func TestAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	if !New(srv.URL, "m", time.Second).Available(context.Background()) {
		t.Error("expected Available true")
	}
	// A dead endpoint must report unavailable, not hang.
	if New("http://127.0.0.1:0", "m", time.Second).Available(context.Background()) {
		t.Error("expected Available false for dead endpoint")
	}
}

func TestPromptIncludesContext(t *testing.T) {
	p := Request{
		Title:          "Fix login",
		Repos:          []string{"auth-svc"},
		BranchExamples: []string{"feat/sso"},
		PriorMappings:  []string{"C123 -> auth-svc"},
	}.prompt()
	for _, want := range []string{"Fix login", "auth-svc", "feat/sso", "C123 -> auth-svc"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n%s", want, p)
		}
	}
}
