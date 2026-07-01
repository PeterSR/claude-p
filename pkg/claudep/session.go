package claudep

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/PeterSR/claude-p/pkg/claudepty"
)

// Session is a live, multi-turn handle to one interactive claude conversation.
// Unlike Query — which launches, sends one prompt, lifts the answer, and tears
// the session down in a single call — a Session stays open across many Prompt
// calls so a caller can hold a conversation, peek at the TUI, answer permission
// prompts, and continue. It is the building block behind the high-level driving
// MCP (pkg/drivemcp); library callers can use it directly too.
//
// A Session is safe for concurrent use: every method that touches the pty takes
// an internal lock, and Prompt serializes turns so two overlapping prompts can
// never interleave on the same conversation.
type Session struct {
	mu        sync.Mutex
	sess      claudepty.PTYSession
	sessionID string
	cwd       string
	daemon    bool
	socket    string
	reused    bool

	// jsonlPath is cached once claude's transcript file exists so each turn
	// doesn't re-walk ~/.claude/projects looking for it.
	jsonlPath string
}

// Open launches (or, in daemon mode, continues) an interactive claude session
// and waits until it is sitting at the input prompt, returning a live Session
// the caller owns. Close (or Kill) it when done. ctx bounds the launch +
// ready-wait; pass a context with a deadline so a stuck boot can't hang forever.
//
// Open honours the same Options as Query — model, permission-mode, cwd, backend
// selection (PupptyeerDaemon), allowed tools, system prompt, and so on — but
// ignores Prompt (there is no prompt to send at open time) and the output-format
// fields (a Session returns text from Prompt rather than emitting an envelope).
func Open(ctx context.Context, opts Options) (*Session, error) {
	applyDefaults(&opts)
	sessionID := resolveSessionID(opts)
	cwd := resolveCwd(opts)

	sess, reused, err := prepareSession(ctx, opts, sessionID, cwd)
	if err != nil {
		return nil, err
	}
	return &Session{
		sess:      sess,
		sessionID: sessionID,
		cwd:       cwd,
		daemon:    opts.PupptyeerDaemon,
		socket:    opts.PupptyeerSocket,
		reused:    reused,
		jsonlPath: claudepty.JSONLPath(sessionID),
	}, nil
}

// ID returns the claude --session-id correlating this conversation with its
// persisted JSONL transcript. Pass it to a later Open (daemon mode) or Query to
// continue the same conversation.
func (s *Session) ID() string { return s.sessionID }

// Cwd returns the working directory claude is running in.
func (s *Session) Cwd() string { return s.cwd }

// Reused reports whether Open continued an already-alive daemon session (true)
// rather than booting a fresh one (false). Always false for the in-process
// backend, which can't outlive the call that created it.
func (s *Session) Reused() bool { return s.reused }

// Prompt sends one user message and blocks until claude finishes the turn,
// returning the assistant's final text. "Finishes" means the turn's terminal
// event has landed in the transcript — either a terminal assistant text message
// or the trailing system turn_duration marker (which also covers tool-only
// turns that end without a text reply, in which case the returned text is
// empty). ctx bounds the wait.
//
// This is the capability a raw send-keys/read-screen MCP cannot offer: one call
// in, the actual answer out, with no screen-scraping or polling by the caller.
func (s *Session) Prompt(ctx context.Context, prompt string) (string, error) {
	if prompt == "" {
		return "", errors.New("claudep: prompt is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if exited, _ := s.sess.Exited(); exited {
		return "", claudepty.ErrProcessExited
	}

	// Record where the transcript currently ends BEFORE sending, so the tail
	// only sees this turn (a continued conversation already has prior turns on
	// disk just before this offset).
	startOffset := claudepty.JSONLOffset(s.sessionID)

	if err := claudepty.SendPrompt(s.sess, prompt); err != nil {
		return "", fmt.Errorf("claudep: send prompt: %w", err)
	}

	jsonlPath := s.jsonlPath
	if jsonlPath == "" {
		jsonlPath = claudepty.WaitForJSONL(s.sessionID, 10*time.Second)
		if jsonlPath == "" {
			return "", fmt.Errorf("claudep: persisted JSONL never appeared for session %s — is claude actually running?", s.sessionID)
		}
		s.jsonlPath = jsonlPath
	}

	// Mirror Query's terminal-detection: take the last assistant text block as
	// the answer, and stop on the first terminal event AFTER this turn's user
	// echo (so a prior turn's still-flushing turn_duration can't end us early).
	var finalText string
	sawUserTurn := false
	err := tailJSONL(ctx, jsonlPath, startOffset, func(ev tailEvent) (bool, error) {
		if ev.Type == "user" {
			sawUserTurn = true
		}
		if ev.Type == "assistant" && ev.Text != "" {
			finalText = ev.Text
		}
		if ev.Terminal && sawUserTurn {
			return true, nil
		}
		return false, nil
	})
	if err != nil && finalText == "" {
		return "", fmt.Errorf("claudep: %w", err)
	}
	return finalText, nil
}

// Screen waits up to budget for claude's TUI to be quiet for settle, then
// returns the rendered grid as text (one line per row). Use it to inspect what
// claude is showing — a permission prompt, a modal, a menu — when a turn needs a
// keystroke rather than a prompt. quiet is false if no readable grid appeared
// within budget.
func (s *Session) Screen(settle, budget time.Duration) (screen string, quiet bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return claudepty.SettleSnapshot(s.sess, settle, budget)
}

// SendKeys writes raw text into the pty, interpreting Go-style escape sequences
// (\r Enter, \n newline, \t tab, \x03 Ctrl-C, \uNNNN). Use it to answer an
// interactive prompt claude is blocking on (e.g. "1\r" to pick option 1, or
// "\x1b" to dismiss a panel) — not for normal conversation, which is Prompt's
// job. Returns the number of bytes written.
func (s *Session) SendKeys(text string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return claudepty.SendKeys(s.sess, text)
}

// WaitReady blocks until claude is back at its main input prompt (or budget
// elapses), having dismissed any first-run style picker or trust modal along
// the way. Open already does this once at launch; call it again after a
// SendKeys sequence to confirm claude has settled before the next Prompt.
func (s *Session) WaitReady(ctx context.Context, budget time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return claudepty.WaitForReady(ctx, s.sess, budget)
}

// Interrupt sends Esc to stop claude mid-turn without ending the session, the
// same key a human presses to cancel a running response. It does not wait for
// claude to settle; follow with WaitReady if the next step needs the prompt.
func (s *Session) Interrupt() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sess.WriteInput([]byte{0x1b})
}

// Exited reports without blocking whether claude has exited, plus its code.
func (s *Session) Exited() (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sess.Exited()
}

// Close releases this handle. In daemon mode it detaches but leaves the live
// claude alive for a later Open (same SessionID) to continue; in-process it
// terminates claude. Use Kill to end a daemon session for good.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sess.Close()
}

// Kill terminates claude regardless of backend (a daemon session is ended, not
// just detached). Idempotent.
func (s *Session) Kill() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sess.Kill()
}

// Shutdown stops the inner claude cleanly: two Ctrl-C's (the keystrokes a human
// uses to quit the TUI) with a short grace period, then a Kill if it hasn't
// exited. Unlike Close — which in daemon mode only detaches and leaves claude
// warm for continuation — Shutdown ends the conversation regardless of backend.
func (s *Session) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	claudepty.Shutdown(s.sess)
}

// IsDaemon reports whether this session runs inside the pupptyeer daemon
// (persistent, and visible to a pupptyeer client for out-of-band monitoring)
// rather than in-process. Only daemon sessions can be watched from outside this
// process.
func (s *Session) IsDaemon() bool { return s.daemon }

// PupptyeerSocket returns the daemon socket override this session connects
// through (empty means the default resolution). Meaningful only when IsDaemon.
func (s *Session) PupptyeerSocket() string { return s.socket }

// StartTurn sends a prompt WITHOUT waiting for the reply, returning the
// transcript byte offset captured just before the send (the "since" cursor for
// PollTurn / CollectTurn — so a later read sees exactly this turn) and the
// transcript path. Use it for fire-and-continue flows where the caller arms its
// own out-of-band wakeup (e.g. a pupptyeer idle monitor on a daemon session)
// instead of blocking on the turn. Pair it with PollTurn/CollectTurn to read the
// answer once the turn finishes.
func (s *Session) StartTurn(prompt string) (sinceOffset int64, transcriptPath string, err error) {
	if prompt == "" {
		return 0, "", errors.New("claudep: prompt is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if exited, _ := s.sess.Exited(); exited {
		return 0, "", claudepty.ErrProcessExited
	}
	sinceOffset = claudepty.JSONLOffset(s.sessionID)
	if err := claudepty.SendPrompt(s.sess, prompt); err != nil {
		return 0, "", fmt.Errorf("claudep: send prompt: %w", err)
	}
	path := s.jsonlPath
	if path == "" {
		// First turn of a brand-new session: the transcript may take a moment to
		// appear. A continued session already has a cached path, so this waits
		// only on the very first turn.
		path = claudepty.WaitForJSONL(s.sessionID, 5*time.Second)
		if path != "" {
			s.jsonlPath = path
		}
	}
	return sinceOffset, path, nil
}

// PollTurn scans the transcript from sinceOffset once, without blocking, and
// reports whether the turn has finished. done is true with the assistant's
// final text once this turn's terminal event has landed; otherwise done is
// false (the turn is still running, or no transcript exists yet). Use it after
// an out-of-band wakeup to authoritatively confirm completion.
func (s *Session) PollTurn(sinceOffset int64) (text string, done bool, err error) {
	path := s.transcriptPath()
	if path == "" {
		return "", false, nil
	}
	f, ferr := os.Open(path)
	if ferr != nil {
		if errors.Is(ferr, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, ferr
	}
	defer f.Close()
	if sinceOffset > 0 {
		if fi, e := f.Stat(); e == nil && fi.Size() >= sinceOffset {
			if _, e := f.Seek(sinceOffset, io.SeekStart); e != nil {
				return "", false, e
			}
		}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var finalText string
	sawUserTurn := false
	for sc.Scan() {
		ev, derr := decodeJSONLLine(bytesTrimNewline(append([]byte(nil), sc.Bytes()...)))
		if derr != nil {
			continue
		}
		if ev.Type == "user" {
			sawUserTurn = true
		}
		if ev.Type == "assistant" && ev.Text != "" {
			finalText = ev.Text
		}
		if ev.Terminal && sawUserTurn {
			return finalText, true, nil
		}
	}
	return "", false, sc.Err()
}

// CollectTurn blocks up to ctx for the turn started at sinceOffset to finish,
// returning the assistant's final text. done is false (no error) if ctx expires
// first — the turn is still running, so try again later. It reads claude's own
// end-of-turn marker, so it is the authoritative completion check.
func (s *Session) CollectTurn(ctx context.Context, sinceOffset int64) (text string, done bool, err error) {
	path := s.transcriptPath()
	if path == "" {
		return "", false, fmt.Errorf("claudep: no transcript for session %s yet", s.sessionID)
	}
	return awaitTurnAt(ctx, path, sinceOffset)
}

// AwaitTurn blocks up to ctx for the turn started at sinceOffset in sessionID's
// transcript to finish, returning the assistant's final text. done is false (no
// error) if ctx expires first. It needs only the session id (no live session)
// because it reads claude's persisted transcript off disk, so it works for
// either backend — it is the primitive behind the `claude-p await-turn` helper
// that a monitor can arm to be notified the moment a turn completes.
func AwaitTurn(ctx context.Context, sessionID string, sinceOffset int64) (text string, done bool, err error) {
	path := claudepty.JSONLPath(sessionID)
	if path == "" {
		return "", false, fmt.Errorf("claudep: no transcript on disk for session %s", sessionID)
	}
	return awaitTurnAt(ctx, path, sinceOffset)
}

// awaitTurnAt is the shared wait: tail the transcript from sinceOffset until the
// terminal event that follows this turn's user echo. Requiring the user echo
// first is what makes it robust to a prior turn's turn_duration marker still
// flushing just past the offset.
func awaitTurnAt(ctx context.Context, path string, sinceOffset int64) (text string, done bool, err error) {
	var finalText string
	sawUserTurn := false
	terr := tailJSONL(ctx, path, sinceOffset, func(ev tailEvent) (bool, error) {
		if ev.Type == "user" {
			sawUserTurn = true
		}
		if ev.Type == "assistant" && ev.Text != "" {
			finalText = ev.Text
		}
		if ev.Terminal && sawUserTurn {
			return true, nil
		}
		return false, nil
	})
	if terr != nil {
		if errors.Is(terr, context.DeadlineExceeded) || errors.Is(terr, context.Canceled) {
			return "", false, nil // still running
		}
		return "", false, fmt.Errorf("claudep: %w", terr)
	}
	return finalText, true, nil
}

// transcriptPath returns the cached transcript path, resolving and caching it
// from disk on first use.
func (s *Session) transcriptPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jsonlPath != "" {
		return s.jsonlPath
	}
	if p := claudepty.JSONLPath(s.sessionID); p != "" {
		s.jsonlPath = p
		return p
	}
	return ""
}
