package claudepty

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrProcessExited is returned when claude exited before the awaited event.
var ErrProcessExited = errors.New("claude process exited")

// ErrTimeout is returned when an interactive wait exhausts its budget.
var ErrTimeout = errors.New("interactive orchestrator timeout")

// readySettle is how long the screen must be quiet before we trust a snapshot
// for readiness/trust decisions.
const readySettle = 400 * time.Millisecond

// WaitForReady drives the "trust this folder" modal and waits for claude's
// input prompt to appear, using rendered-screen settle (server/lib-side) rather
// than raw-byte polling. Returns nil once the main input row is visible.
func WaitForReady(ctx context.Context, s PTYSession, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	trustHandled := false

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if exited, _ := s.Exited(); exited {
			return ErrProcessExited
		}

		// Capturing with a settle window blocks until the grid is quiet (or the
		// short per-capture timeout elapses), which paces this loop without a
		// manual sleep.
		scr, err := s.CaptureScreen(readySettle, 1500*time.Millisecond)
		if err != nil || scr == nil || len(scr.Lines) == 0 {
			continue
		}
		screen := scr.Text()
		lower := strings.ToLower(screen)

		// The trust modal can appear a beat after boot, so keep watching for
		// it until the prompt shows — don't mark it "handled" just because an
		// early screen didn't have it yet (that would skip a late modal).
		if !trustHandled && (strings.Contains(lower, "trust this folder") || strings.Contains(lower, "do you trust")) {
			// Option 1 (Yes) is preselected; Enter confirms.
			if err := s.WriteInput([]byte("\r")); err != nil {
				return err
			}
			trustHandled = true
			continue
		}

		// Primary signal: claude has parked its visible cursor in the "❯"
		// input row. Fall back to the text match if the cursor can't be
		// trusted (e.g. a backend that doesn't report cursor position).
		if ReadyForInput(scr) || HasInputPrompt(screen) {
			return nil
		}
	}
	return ErrTimeout
}

// SendPrompt types the prompt + Enter into the pty. The text and the Enter are
// split so claude has a moment to ingest before submit (its paste-detection
// heuristics have historically been twitchy about huge single writes).
func SendPrompt(s PTYSession, prompt string) error {
	if err := s.WriteInput([]byte(prompt)); err != nil {
		return err
	}
	time.Sleep(150 * time.Millisecond)
	return s.WriteInput([]byte("\r"))
}

// Shutdown asks claude to tear down cleanly via two Ctrl-C's, then Kills it
// after a short grace. Used for one-shot runs that own the session; daemon
// continuation detaches via Close instead.
func Shutdown(s PTYSession) {
	_ = s.WriteInput([]byte{0x03})
	time.Sleep(150 * time.Millisecond)
	_ = s.WriteInput([]byte{0x03})

	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	if _, err := s.Wait(ctx); err != nil {
		_ = s.Kill()
	}
}

// SendKeys writes text into the pty, interpreting Go-style escape sequences
// (\r, \n, \t, \xNN, \uNNNN). Returns the number of bytes written.
func SendKeys(s PTYSession, text string) (int, error) {
	b := []byte(UnescapeKeys(text))
	if err := s.WriteInput(b); err != nil {
		return 0, err
	}
	return len(b), nil
}

// SettleSnapshot waits up to budget for the screen to be quiet for at least
// settle, then returns the rendered screen text. quiet reports whether a
// capture succeeded (false means the budget elapsed without a readable grid).
func SettleSnapshot(s PTYSession, settle, budget time.Duration) (screen string, quiet bool) {
	scr, err := s.CaptureScreen(settle, budget)
	if err != nil || scr == nil {
		return "", false
	}
	return scr.Text(), true
}
