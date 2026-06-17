package memory

import (
	"path/filepath"
	"testing"
)

func TestRecordAndLookup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grab-memory.json")
	s := Load(path)

	if _, ok := s.Lookup("slack", "C123"); ok {
		t.Error("expected miss on empty store")
	}

	if err := s.Record("slack", "C123", "data-svc"); err != nil {
		t.Fatalf("record: %v", err)
	}

	// Reload from disk to prove persistence.
	s2 := Load(path)
	repo, ok := s2.Lookup("slack", "C123")
	if !ok || repo != "data-svc" {
		t.Errorf("lookup = %q, %v", repo, ok)
	}
}

func TestRecordIncrementsCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.json")
	s := Load(path)
	s.Record("notion", "ws1", "api")
	s.Record("notion", "ws1", "api")
	s.Record("notion", "ws1", "api")
	if s.entries[0].Count != 3 {
		t.Errorf("count = %d, want 3", s.entries[0].Count)
	}
}

func TestRecordOverwriteResetsCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.json")
	s := Load(path)
	s.Record("github", "acme", "old-repo")
	s.Record("github", "acme", "old-repo")
	s.Record("github", "acme", "new-repo") // different choice
	repo, _ := s.Lookup("github", "acme")
	if repo != "new-repo" {
		t.Errorf("repo = %q, want new-repo", repo)
	}
	if s.entries[0].Count != 1 {
		t.Errorf("count = %d, want reset to 1", s.entries[0].Count)
	}
}

func TestRecordIgnoresEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.json")
	s := Load(path)
	s.Record("slack", "", "repo") // no signal
	s.Record("slack", "C1", "")   // no repo
	if len(s.entries) != 0 {
		t.Errorf("expected no entries, got %d", len(s.entries))
	}
}

func TestRecentMappingsOrderedByCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.json")
	s := Load(path)
	s.Record("slack", "C-low", "r1")
	s.Record("slack", "C-high", "r2")
	s.Record("slack", "C-high", "r2") // count 2
	got := s.RecentMappings(10)
	if len(got) != 2 || got[0] != "C-high -> r2" {
		t.Errorf("recent = %v, want most-used first", got)
	}
}
