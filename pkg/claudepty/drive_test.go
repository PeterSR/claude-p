package claudepty

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeSession is an in-memory PTYSession: it serves a scripted sequence of
// screens to CaptureScreen and records everything written. It lets us test the
// claude driving logic (trust modal, prompt detection, key sequencing) without
// a real pty or a real claude.
type fakeSession struct {
	mu       sync.Mutex
	screens  []*Screen
	captures int
	writes   [][]byte
	exited   bool
	code     int
}

func screenOf(lines ...string) *Screen {
	return &Screen{Cols: VTCols, Rows: len(lines), Lines: lines}
}

func screenWithCursor(cur Cursor, lines ...string) *Screen {
	s := screenOf(lines...)
	s.Cursor = cur
	return s
}

func (f *fakeSession) WriteInput(p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, append([]byte(nil), p...))
	return nil
}

func (f *fakeSession) CaptureScreen(settle, timeout time.Duration) (*Screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.captures
	if i >= len(f.screens) {
		i = len(f.screens) - 1
	}
	f.captures++
	return f.screens[i], nil
}

func (f *fakeSession) Wait(ctx context.Context) (int, error) { return f.code, nil }
func (f *fakeSession) Exited() (bool, int)                   { return f.exited, f.code }
func (f *fakeSession) Kill() error                           { f.exited = true; return nil }
func (f *fakeSession) Close() error                          { return nil }

func (f *fakeSession) writeStrings() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.writes))
	for i, w := range f.writes {
		out[i] = string(w)
	}
	return out
}

// TestWaitForReadyAnswersTrustThenSeesPrompt is the load-bearing driving test:
// a trust modal must be answered with Enter, then the input prompt detected.
func TestWaitForReadyAnswersTrustThenSeesPrompt(t *testing.T) {
	f := &fakeSession{screens: []*Screen{
		screenOf(
			"Do you trust the files in this folder?",
			"❯ 1. Yes, I trust this folder",
			"  2. No, take me back",
		),
		screenOf("", "❯", `  Try "fix the bug"`),
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitForReady(ctx, f, 2*time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}

	writes := f.writeStrings()
	if len(writes) != 1 || writes[0] != "\r" {
		t.Errorf("expected exactly one trust-confirming \\r write, got %q", writes)
	}
	if f.captures < 2 {
		t.Errorf("expected at least 2 captures (trust + prompt), got %d", f.captures)
	}
}

// TestWaitForReadyViaCursorWhenTextWouldMiss is the regression guard for the
// ghost-placeholder bug: claude renders the input row with a persistent, oddly
// worded placeholder that the text fallback (HasInputPrompt) deliberately does
// NOT match, but the visible cursor is parked on that "❯" row. WaitForReady
// must still detect ready via the cursor signal. If the cursor path is ever
// removed, this fails while the text-only test keeps passing — pinpointing the
// regression.
func TestWaitForReadyViaCursorWhenTextWouldMiss(t *testing.T) {
	ready := screenWithCursor(
		Cursor{Row: 1, Col: 4, Visible: true},
		"────────────────────",
		`❯  ask me anything (paste to add files)`,
		"────────────────────",
		"? for shortcuts · ← for agents",
	)
	// Precondition: the text fallback genuinely can't see this prompt, so a
	// pass below proves the cursor path is doing the work.
	if HasInputPrompt(ready.Text()) {
		t.Fatal("test setup invalid: text fallback matched; pick a placeholder it can't")
	}
	if !ReadyForInput(ready) {
		t.Fatal("test setup invalid: cursor signal should match this screen")
	}

	f := &fakeSession{screens: []*Screen{ready}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitForReady(ctx, f, 2*time.Second); err != nil {
		t.Fatalf("WaitForReady via cursor: %v", err)
	}
}

// TestWaitForReadyViaTextWhenCursorNotVisible is the complementary guard to
// TestWaitForReadyViaCursorWhenTextWouldMiss: when a backend reports the cursor
// as not-visible (pupptyeer 0.8.0 defaults an absent rendered cursor to
// not-visible), the cursor signal abstains and readiness must fall back to the
// HasInputPrompt text match. If that fallback is ever dropped, this fails while
// the cursor test keeps passing, pinpointing the regression.
func TestWaitForReadyViaTextWhenCursorNotVisible(t *testing.T) {
	ready := screenWithCursor(
		Cursor{Row: 1, Col: 2, Visible: false},
		"────────────────────",
		`❯ Try "fix the bug"`,
		"────────────────────",
	)
	// Precondition: the cursor signal abstains (not visible), so a pass below
	// proves the text fallback is doing the work.
	if ReadyForInput(ready) {
		t.Fatal("test setup invalid: cursor signal should abstain when not visible")
	}
	if !HasInputPrompt(ready.Text()) {
		t.Fatal("test setup invalid: text fallback should match this prompt row")
	}

	f := &fakeSession{screens: []*Screen{ready}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitForReady(ctx, f, 2*time.Second); err != nil {
		t.Fatalf("WaitForReady via text fallback: %v", err)
	}
}

func TestWaitForReadyExitedProcess(t *testing.T) {
	f := &fakeSession{screens: []*Screen{screenOf("")}, exited: true}
	if err := WaitForReady(context.Background(), f, time.Second); err != ErrProcessExited {
		t.Errorf("err = %v, want ErrProcessExited", err)
	}
}

// TestSendPromptSplitsTextAndEnter confirms the prompt text and the submitting
// Enter are written separately (claude's paste heuristics need the gap).
func TestSendPromptSplitsTextAndEnter(t *testing.T) {
	f := &fakeSession{screens: []*Screen{screenOf("")}}
	if err := SendPrompt(f, "hello world"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	writes := f.writeStrings()
	if len(writes) != 2 || writes[0] != "hello world" || writes[1] != "\r" {
		t.Errorf("writes = %q, want [\"hello world\", \"\\r\"]", writes)
	}
}

// TestSendKeysUnescapes confirms Go-style escapes become real control bytes.
func TestSendKeysUnescapes(t *testing.T) {
	f := &fakeSession{screens: []*Screen{screenOf("")}}
	n, err := SendKeys(f, `/usage\r`)
	if err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	writes := f.writeStrings()
	if len(writes) != 1 || writes[0] != "/usage\r" {
		t.Errorf("writes = %q, want [\"/usage\\r\"]", writes)
	}
	if n != len("/usage\r") {
		t.Errorf("n = %d, want %d", n, len("/usage\r"))
	}
}
