// Package memory persists which repo a given source "signal" (Slack channel,
// Notion workspace, GitHub owner) was grabbed into. Repeated grabs from the same
// signal can then pre-select the repo deterministically and skip LLM inference —
// so grab gets faster and more accurate the more it is used.
package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Entry records one signal→repo decision.
type Entry struct {
	Provider  string `json:"provider"` // e.g. "slack", "notion", "github"
	Signal    string `json:"signal"`   // channel id / workspace slug / owner
	Repo      string `json:"repo"`     // chosen local repo name
	Count     int    `json:"count"`    // how many times this mapping was used
	UpdatedAt string `json:"updated_at"`
}

// Store is an on-disk map of signal→repo decisions.
type Store struct {
	path    string
	entries []Entry
	now     func() time.Time
}

// key uniquely identifies a signal within a provider.
func key(provider, signal string) string { return provider + "\x00" + signal }

// DefaultPath returns the memory file path under GROVE_DIR (falling back to
// ~/.grove), matching where grove keeps its own state.
func DefaultPath() string {
	dir := os.Getenv("GROVE_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".grove")
	}
	return filepath.Join(dir, "grab-memory.json")
}

// Load reads the store from path. A missing file yields an empty store.
func Load(path string) *Store {
	s := &Store{path: path, now: time.Now}
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, &s.entries)
	}
	return s
}

// Lookup returns the remembered repo for a signal and whether one exists.
func (s *Store) Lookup(provider, signal string) (string, bool) {
	if signal == "" {
		return "", false
	}
	for _, e := range s.entries {
		if e.Provider == provider && e.Signal == signal {
			return e.Repo, true
		}
	}
	return "", false
}

// Record upserts a signal→repo decision and persists the store. Recording a
// different repo for an existing signal overwrites it (the latest choice wins)
// and resets the count.
func (s *Store) Record(provider, signal, repo string) error {
	if signal == "" || repo == "" {
		return nil // nothing stable to remember
	}
	stamp := s.now().UTC().Format(time.RFC3339)
	for i := range s.entries {
		if key(s.entries[i].Provider, s.entries[i].Signal) == key(provider, signal) {
			if s.entries[i].Repo == repo {
				s.entries[i].Count++
			} else {
				s.entries[i].Repo = repo
				s.entries[i].Count = 1
			}
			s.entries[i].UpdatedAt = stamp
			return s.save()
		}
	}
	s.entries = append(s.entries, Entry{
		Provider: provider, Signal: signal, Repo: repo, Count: 1, UpdatedAt: stamp,
	})
	return s.save()
}

// RecentMappings returns up to n "signal -> repo" strings, most-used first, for
// use as LLM few-shot hints.
func (s *Store) RecentMappings(n int) []string {
	sorted := make([]Entry, len(s.entries))
	copy(sorted, s.entries)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	out := make([]string, 0, len(sorted))
	for _, e := range sorted {
		out = append(out, e.Signal+" -> "+e.Repo)
	}
	return out
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
