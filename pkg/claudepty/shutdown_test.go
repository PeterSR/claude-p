package claudepty

import (
	"context"
	"sync"
	"testing"
	"time"
)

// exitFake models a claude session for Shutdown tests. It records every write
// and can be configured to mimic the three ways a two-Ctrl-C quit plays out:
// a clean quit (exitsOnCtrlC), the "background work is running" confirmation
// modal that only exits once Enter is sent (dialog + confirmable), or a wedged
// session that never exits on its own (both false) and must be Killed.
type exitFake struct {
	mu           sync.Mutex
	writes       [][]byte
	ctrlC        int
	exitsOnCtrlC bool // a plain quit: two Ctrl-C's exit, no modal
	dialog       bool // CaptureScreen shows the exit-confirm modal until confirmed
	confirmable  bool // Enter (\r) exits, as a real modal would
	exited       bool
	killed       bool
}

func (f *exitFake) WriteInput(p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, append([]byte(nil), p...))
	switch string(p) {
	case "\x03":
		f.ctrlC++
		if f.exitsOnCtrlC && f.ctrlC >= 2 {
			f.exited = true
		}
	case "\r":
		if f.confirmable {
			f.exited = true
		}
	}
	return nil
}

func (f *exitFake) CaptureScreen(settle, timeout time.Duration) (*Screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dialog && !f.exited {
		return screenOf(
			"  Background work is running",
			"  The following will stop when you exit:",
			"  shell · Incoming pigeon messages from other Claude Code s…",
			"  ❯ 1. Exit anyway",
			"    2. Stay",
			"  Enter to confirm · Esc to cancel",
		), nil
	}
	return screenOf("❯ "), nil
}

func (f *exitFake) Wait(ctx context.Context) (int, error) {
	for {
		f.mu.Lock()
		done := f.exited || f.killed
		f.mu.Unlock()
		if done {
			return 0, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func (f *exitFake) Exited() (bool, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exited || f.killed, 0
}

func (f *exitFake) Kill() error {
	f.mu.Lock()
	f.killed = true
	f.mu.Unlock()
	return nil
}

func (f *exitFake) Close() error { return nil }

func (f *exitFake) sent(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.writes {
		if string(w) == s {
			return true
		}
	}
	return false
}

func (f *exitFake) wasKilled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.killed
}

// TestShutdownConfirmsExitModal is the regression for the reason this file
// exists: when a monitor/background shell makes claude answer the two-Ctrl-C
// quit with an "Exit anyway / Stay" confirmation, Shutdown must confirm it with
// Enter so claude exits cleanly (running its own teardown), NOT time out and
// hard-Kill it (which bypasses that teardown and can orphan the background work).
func TestShutdownConfirmsExitModal(t *testing.T) {
	f := &exitFake{dialog: true, confirmable: true}
	Shutdown(f)

	if got, want := f.writeStringsOf(), []string{"\x03", "\x03", "\r"}; !equalStrings(got, want) {
		t.Errorf("Shutdown writes = %q, want two Ctrl-C's then a single Enter", got)
	}
	if f.wasKilled() {
		t.Errorf("Shutdown hard-Killed a session that could exit cleanly via the modal")
	}
	if exited, _ := f.Exited(); !exited {
		t.Errorf("session did not exit after confirming the modal")
	}
}

// TestShutdownCleanQuitNoModal: with no background work, two Ctrl-C's quit
// outright. Shutdown must not send a stray Enter or fall back to Kill.
func TestShutdownCleanQuitNoModal(t *testing.T) {
	f := &exitFake{exitsOnCtrlC: true}
	Shutdown(f)

	if f.sent("\r") {
		t.Errorf("Shutdown sent Enter when there was no exit modal to confirm")
	}
	if f.wasKilled() {
		t.Errorf("Shutdown Killed a session that already quit on Ctrl-C")
	}
}

// TestShutdownKillsWhenWedged: a session that neither quits on Ctrl-C nor shows
// a confirmable modal must still be terminated, via the Kill fallback.
func TestShutdownKillsWhenWedged(t *testing.T) {
	f := &exitFake{} // never exits on its own, no dialog
	Shutdown(f)

	if !f.wasKilled() {
		t.Errorf("Shutdown left a wedged session alive instead of Killing it")
	}
}

// TestShutdownKillsWhenModalSticks: the modal is up but Enter doesn't make
// claude exit. Shutdown must still fall back to Kill.
func TestShutdownKillsWhenModalSticks(t *testing.T) {
	f := &exitFake{dialog: true}
	Shutdown(f)

	if !f.sent("\r") {
		t.Errorf("Shutdown did not try confirming the exit modal before Killing")
	}
	if !f.wasKilled() {
		t.Errorf("Shutdown left a session stuck on the exit modal alive instead of Killing it")
	}
}

func TestHasExitConfirm(t *testing.T) {
	yes := "  Background work is running\n  ❯ 1. Exit anyway\n    2. Stay\n  Enter to confirm · Esc to cancel"
	if !HasExitConfirm(yes) {
		t.Errorf("HasExitConfirm missed the exit-confirmation modal")
	}
	for name, no := range map[string]string{
		"input prompt":               "❯ \n  ⏵⏵ auto mode on · 1 monitor · ← for agents",
		"transcript mention":         "⏺ When the modal shows, Shutdown presses Enter on \"Exit anyway\".\n\n❯ \n  ⏵⏵ auto mode on",
		"other menu under a mention": "⏺ It confirms Exit anyway, otherwise Stay.\n  Do you want to proceed?\n  ❯ 1. Yes\n    2. No",
	} {
		if HasExitConfirm(no) {
			t.Errorf("HasExitConfirm false-positived on %s", name)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// writeStringsOf renders recorded writes for failure messages.
func (f *exitFake) writeStringsOf() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.writes))
	for i, w := range f.writes {
		out[i] = string(w)
	}
	return out
}
