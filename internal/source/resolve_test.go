package source

import (
	"context"
	"testing"
)

// With no Notion token, the offline slug title is preserved and no note added.
func TestResolveNotion_OfflineKeepsSlugTitle(t *testing.T) {
	sc, _ := ParseURL("https://app.notion.com/p/funnelio/Fix-the-thing-37430a04501f8008b6e9c63e43d9a698")
	before := sc.Title
	Resolve(context.Background(), sc, Config{}) // no token
	if sc.Title != before {
		t.Errorf("title changed offline: %q -> %q", before, sc.Title)
	}
	if before != "Fix the thing" {
		t.Errorf("slug title = %q", before)
	}
}

// With no Slack token, a degradation note is added so the preview can show it.
func TestResolveSlack_OfflineNote(t *testing.T) {
	sc, _ := ParseURL("https://qwaya.slack.com/archives/C0FT37YHZ/p1781511693141549")
	Resolve(context.Background(), sc, Config{})
	if len(sc.Notes) == 0 {
		t.Error("expected a degradation note for tokenless Slack")
	}
}

func TestExtractNotionTitle(t *testing.T) {
	body := []byte(`{"properties":{"Name":{"type":"title","title":[{"plain_text":"Hello "},{"plain_text":"World"}]}}}`)
	if got := extractNotionTitle(body); got != "Hello World" {
		t.Errorf("title = %q, want 'Hello World'", got)
	}
	if got := extractNotionTitle([]byte(`{"properties":{}}`)); got != "" {
		t.Errorf("expected empty for no title prop, got %q", got)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("  hello\nworld\n"); got != "hello" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine("single"); got != "single" {
		t.Errorf("firstLine = %q", got)
	}
}

func TestOwnerOf(t *testing.T) {
	if ownerOf("funnel-io/conversational-analytics") != "funnel-io" {
		t.Error("ownerOf slug")
	}
	if ownerOf("noslash") != "noslash" {
		t.Error("ownerOf no slash")
	}
}
