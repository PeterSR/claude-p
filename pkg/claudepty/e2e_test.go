package claudepty

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestReadyDetectionE2E drives a real `claude` in-process and asserts the
// signals WaitForReady relies on actually hold against the live TUI. It is the
// drift detector for the readiness logic: claude's cursor placement, prompt
// glyph, and trust-modal wording can only change in real claude, so only an
// e2e run can catch that drift.
//
// Gated behind CLAUDE_P_E2E because it spawns claude and needs login:
//
//	CLAUDE_P_E2E=1 go test ./pkg/claudepty -run ReadyDetectionE2E -v
//
// The two readiness signals are asserted independently so a -v failure says
// WHICH one drifted:
//   - cursor: claude parks the visible cursor on the "❯" row (the primary
//     signal, immune to the variable "Try …" placeholder).
//   - text: a "❯" prompt row exists (the fallback for backends with no cursor).
func TestReadyDetectionE2E(t *testing.T) {
	if os.Getenv("CLAUDE_P_E2E") == "" {
		t.Skip("set CLAUDE_P_E2E=1 to run (spawns claude, needs login)")
	}

	// A fresh temp dir as cwd exercises the trust modal too: if claude prompts
	// to trust the folder and WaitForReady's wording match has drifted, the
	// wait times out and this fails. (If the dir's parent is already trusted no
	// modal shows — we can't force one without an isolated HOME, which would
	// drop the login.)
	sess, err := StartInproc(ClaudeLaunch{Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("StartInproc: %v", err)
	}
	defer Shutdown(sess)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := WaitForReady(ctx, sess, 45*time.Second); err != nil {
		scr, _ := sess.CaptureScreen(0, time.Second)
		t.Fatalf("WaitForReady: %v\nlast screen:\n%s", err, trimScreen(scr))
	}

	// Re-capture a settled screen to assert on the steady ready state.
	scr, err := sess.CaptureScreen(readySettle, 2*time.Second)
	if err != nil || scr == nil {
		t.Fatalf("CaptureScreen after ready: %v", err)
	}
	dump := trimScreen(scr)

	// Primary signal: the cursor is the thing we now depend on.
	if !scr.Cursor.Visible {
		t.Errorf("DRIFT: cursor not reported visible after ready; readiness fell back to the text match. screen:\n%s", dump)
	} else {
		row := ""
		if scr.Cursor.Row >= 0 && scr.Cursor.Row < len(scr.Lines) {
			row = scr.Lines[scr.Cursor.Row]
		}
		t.Logf("cursor at row=%d col=%d on %q", scr.Cursor.Row, scr.Cursor.Col, strings.TrimRight(row, " "))
		if !ReadyForInput(scr) {
			t.Errorf("DRIFT: cursor visible but ReadyForInput=false (cursor not on a \"❯\" row). screen:\n%s", dump)
		}
	}

	// Secondary signal: the text fallback should still find a prompt row.
	if !HasInputPrompt(scr.Text()) {
		t.Errorf("DRIFT: HasInputPrompt text fallback found no \"❯\" prompt row. screen:\n%s", dump)
	}
}

// trimScreen renders a Screen for failure diagnostics: drop blank lines and
// trailing padding so a 200x60 grid stays readable in test output.
func trimScreen(scr *Screen) string {
	if scr == nil {
		return "<nil>"
	}
	var b strings.Builder
	for _, line := range scr.Lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(strings.TrimRight(line, " "))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
