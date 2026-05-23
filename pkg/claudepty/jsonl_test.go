package claudepty

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestJSONLPath_findsRecentMatch writes two synthetic JSONL files under
// the user's projects dir layout (in a temp HOME) and verifies the
// lookup picks the most-recently-modified one.
func TestJSONLPath_findsRecentMatch(t *testing.T) {
	// Redirect HOME to a temp dir so we don't pollute the real one.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // Windows fallback for os.UserHomeDir

	sessionID := "test-session-1234"
	root := filepath.Join(tmp, ".claude", "projects")

	// Two projects, same session id, different mtimes.
	older := filepath.Join(root, "-home-user-projectA")
	newer := filepath.Join(root, "-home-user-projectB")
	if err := os.MkdirAll(older, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newer, 0o755); err != nil {
		t.Fatal(err)
	}
	olderFile := filepath.Join(older, sessionID+".jsonl")
	newerFile := filepath.Join(newer, sessionID+".jsonl")
	if err := os.WriteFile(olderFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newerFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Backdate the older file.
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(olderFile, past, past); err != nil {
		t.Fatal(err)
	}

	got := JSONLPath(sessionID)
	if got != newerFile {
		t.Errorf("JSONLPath(%q) = %q, want %q", sessionID, got, newerFile)
	}
}

func TestJSONLPath_emptySessionID(t *testing.T) {
	if got := JSONLPath(""); got != "" {
		t.Errorf("empty session id should return empty path, got %q", got)
	}
}

func TestJSONLPath_noMatch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	if got := JSONLPath("definitely-not-here"); got != "" {
		t.Errorf("no match should return empty path, got %q", got)
	}
}

func TestWaitForJSONL_succeedsWhenFileAppears(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)

	sessionID := "wait-session"
	dir := filepath.Join(tmp, ".claude", "projects", "-home-user-proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, sessionID+".jsonl")

	// Create the file after a short delay.
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(target, []byte("{}\n"), 0o644)
	}()

	got := WaitForJSONL(sessionID, 2*time.Second)
	if got != target {
		t.Errorf("WaitForJSONL = %q, want %q", got, target)
	}
}

func TestWaitForJSONL_timesOut(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	got := WaitForJSONL("never-appears", 250*time.Millisecond)
	if got != "" {
		t.Errorf("timeout should return empty, got %q", got)
	}
}
