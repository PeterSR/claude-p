package claudepty

import (
	"crypto/rand"
	"fmt"
	"time"
)

// NewSessionID returns a random v4 UUID suitable for passing to
// `claude --session-id`. It MUST be a well-formed v4 (correct version and
// variant nibbles): current Claude Code only persists the JSONL transcript at
// ~/.claude/projects/**/<id>.jsonl when the supplied id is a valid UUID, and
// claude-p locates the assistant's reply through that transcript.
func NewSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Time-based fallback. Uniqueness, not unguessability, is what
		// claude needs.
		return fmt.Sprintf("cp-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx (RFC 4122)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
