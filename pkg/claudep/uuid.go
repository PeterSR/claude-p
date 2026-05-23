package claudep

import (
	"crypto/rand"
	"fmt"
)

// newEventUUID returns a UUID-shaped (8-4-4-4-12 hex) token. Used as a
// per-event identifier on the synthesized stream-json events. Matches
// the shape Python claude-p emits.
func newEventUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Should not happen on any sane platform; fall back to a tag
		// that's at least unique within a process.
		return fmt.Sprintf("noise-%x", b[:])
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
