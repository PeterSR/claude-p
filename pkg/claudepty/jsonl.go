package claudepty

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// JSONLPath returns the most-recently-modified persisted JSONL file
// claude saves for the given session ID under ~/.claude/projects/**.
// Returns "" if no such file exists. Cross-platform: uses os.UserHomeDir
// which respects %USERPROFILE% on Windows.
func JSONLPath(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := filepath.Join(home, ".claude", "projects")
	suffix := sessionID + ".jsonl"

	var best string
	var bestT time.Time
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), suffix) {
			return nil
		}
		if info.ModTime().After(bestT) {
			best = path
			bestT = info.ModTime()
		}
		return nil
	})
	return best
}

// JSONLOffset returns the current size in bytes of the session's persisted
// transcript, or 0 if it does not exist yet. Capture it BEFORE sending a new
// prompt so a tail can skip everything already written (prior turns of a
// continued conversation) and only see the new turn.
func JSONLOffset(sessionID string) int64 {
	p := JSONLPath(sessionID)
	if p == "" {
		return 0
	}
	fi, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// WaitForJSONL polls JSONLPath until the file appears or the deadline
// passes. Useful right after a `claude --session-id <id>` invocation
// when you want to start streaming events as soon as the file exists.
func WaitForJSONL(sessionID string, budget time.Duration) string {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if p := JSONLPath(sessionID); p != "" {
			return p
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}
