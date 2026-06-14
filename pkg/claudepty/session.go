// Package claudepty drives an interactive `claude` (the Claude Code TUI)
// from Go: it no longer owns a pty itself. The pty substrate — spawning,
// reading, terminal emulation, rendered/raw capture, resize, teardown — is
// provided by pupptyeer (github.com/PeterSR/pupptyeer), used two ways behind
// the PTYSession interface: in-process (one-shot, no external binary) and over
// a pupptyeer daemon (persistent, multi-turn). Everything in this package is
// the claude-specific layer on top: building claude's argv, answering the
// trust modal, detecting the input prompt, classifying failures, and lifting
// the answer from claude's persisted JSONL transcript.
package claudepty

import (
	"context"
	"strings"
	"time"
)

// VTCols / VTRows define the virtual terminal claude is driven in. The size
// matters because claude's TUI layout depends on the dimensions it sees; both
// backends pin this same size so the trust-modal / input-prompt / failure
// detectors behave identically whether driving in-process or over the daemon.
const (
	VTCols = 200
	VTRows = 60
)

// Cursor is the cursor position in a captured Screen. Row/Col are 0-based.
type Cursor struct {
	Row, Col int
	Visible  bool
}

// Screen is a rendered snapshot of the pty's visible grid: one space-padded
// string per row, the cursor, and whether the program is on the alternate
// screen buffer. It is what the trust/prompt/failure detectors read.
type Screen struct {
	Cols, Rows int
	Lines      []string
	Cursor     Cursor
	AltScreen  bool
}

// Text joins the rendered lines with newlines. The string-based detectors
// (HasInputPrompt, ClassifyInteractiveFailure) consume this.
func (s *Screen) Text() string {
	if s == nil {
		return ""
	}
	return strings.Join(s.Lines, "\n")
}

// PTYSession is the pty substrate the claude driving logic needs, satisfied by
// both the in-process backend (pupptyeer/pkg/ptysession) and the daemon
// backend (pupptyeer clients/go). It is deliberately small: write input,
// capture the rendered screen, observe exit, and tear down.
type PTYSession interface {
	// WriteInput sends raw bytes to claude's pty input (prompt text, \r, 0x03).
	WriteInput(p []byte) error

	// CaptureScreen returns the rendered grid, having first waited up to
	// timeout for the screen to be quiet for settle (settle<=0 = no wait).
	CaptureScreen(settle, timeout time.Duration) (*Screen, error)

	// Wait blocks until claude exits (or ctx is done) and returns its exit code.
	Wait(ctx context.Context) (exitCode int, err error)

	// Exited reports without blocking whether claude has exited, plus the code.
	Exited() (exited bool, exitCode int)

	// Kill terminates claude. Idempotent.
	Kill() error

	// Close releases this handle's resources. In daemon mode it detaches but
	// leaves the session alive for continuation; in-process it kills.
	Close() error
}
