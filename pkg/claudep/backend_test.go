package claudep

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PeterSR/claude-p/pkg/claudepty"
)

// TestNewBackendDaemonUnreachableFailsLoud asserts the camp-A contract: with
// daemon mode requested against an unreachable socket, newBackend fails with the
// pupptyeer client's canonical "no pupptyeer daemon at <sock>" error and never
// spawns anything. claude-p no longer manages the daemon lifecycle.
func TestNewBackendDaemonUnreachableFailsLoud(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "nope.sock")
	opts := Options{PupptyeerDaemon: true, PupptyeerSocket: sock}

	start := time.Now()
	sess, reused, err := newBackend(opts, "test-session", claudepty.ClaudeLaunch{})
	elapsed := time.Since(start)

	if err == nil {
		_ = sess.Close()
		t.Fatal("newBackend returned nil error for an unreachable daemon; want a loud failure")
	}
	if sess != nil {
		t.Errorf("newBackend returned a non-nil session on failure: %v", sess)
	}
	if reused {
		t.Error("reused = true on failure, want false")
	}
	// The client's canonical error names the socket and the remedy. We assert the
	// stable, actionable fragments rather than the full string.
	for _, want := range []string{"no pupptyeer daemon at", sock} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q (expected the client's canonical connect-or-scream message)", err.Error(), want)
		}
	}
	// No spawn, no poll loop: a failed connect must return promptly rather than
	// waiting on a former "start the daemon and wait for the socket" deadline.
	if elapsed > 2*time.Second {
		t.Errorf("newBackend took %s to fail; expected an immediate connect failure with no spawn/poll", elapsed)
	}
}
