package claudepty

import (
	"crypto/rand"
	"fmt"
	"time"
)

// NewSessionID returns a UUID-shaped (8-4-4-4-12 hex) token suitable
// for passing to `claude --session-id`. Not strictly a v4 UUID — claude
// only cares about uniqueness here. Used so callers can locate the
// persisted JSONL at ~/.claude/projects/**/<id>.jsonl afterwards.
func NewSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Time-based fallback. Uniqueness, not unguessability, is what
		// claude needs.
		return fmt.Sprintf("cp-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
