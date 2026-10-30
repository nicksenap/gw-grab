package herdr

import "testing"

func TestParseOpened(t *testing.T) {
	for _, out := range []string{
		"herdr-workspace: opened wDP (try-out) at /w/try-out\n",
		"herdr-workspace: reused wDP (try-out)\n",
	} {
		if id, ok := ParseOpened(out); !ok || id != "wDP" {
			t.Errorf("%q → %q, %v", out, id, ok)
		}
	}
	if _, ok := ParseOpened("herdr-workspace: herdr server is not running; skipping"); ok {
		t.Error("expected no id")
	}
}

func TestParseFirstPane(t *testing.T) {
	id, err := ParseFirstPane([]byte(`{"result":{"panes":[{"pane_id":"wDP:p1"},{"pane_id":"wDP:p2"}]}}`))
	if err != nil || id != "wDP:p1" {
		t.Fatalf("got %q, %v", id, err)
	}
	if _, err := ParseFirstPane([]byte(`{"result":{"panes":[]}}`)); err == nil {
		t.Error("expected error on no panes")
	}
}
