package claudepty

import (
	"strconv"
	"strings"
	"time"
)

// UnescapeKeys turns Go-style escape sequences in s into their
// corresponding bytes. Useful when keystrokes arrive as JSON-shaped
// strings (e.g. {"text": "\\r"} where the JSON literally contains
// backslash-r and the caller means "carriage return").
//
// Anything that doesn't parse as a Go literal is returned unchanged
// (assume the caller meant the literal text).
func UnescapeKeys(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	// strconv.Unquote requires surrounding quotes and handles all the
	// usual escapes (\r, \n, \t, \xNN, \uNNNN, \\, \"). Wrap and try.
	if unq, err := strconv.Unquote("\"" + escapeForUnquote(s) + "\""); err == nil {
		return unq
	}
	return s
}

func escapeForUnquote(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			b.WriteString(`\"`)
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// SettleSnapshot waits up to budget for the pty buffer to be quiet for
// at least settle (no new bytes in that window), then returns the
// rendered screen + whether quiet was actually observed (false = the
// budget elapsed without a quiet window).
func (cs *ClaudeSession) SettleSnapshot(settle, budget time.Duration) (screen string, quiet bool) {
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		_, since := cs.Snapshot()
		if since >= settle {
			quiet = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cs.RenderGrid(), quiet
}
