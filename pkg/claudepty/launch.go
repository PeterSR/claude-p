// Package claudepty drives an interactive `claude` (the Claude Code TUI)
// from Go: spawn it in a pty, wait for the input prompt, send keystrokes,
// observe the rendered screen, classify failure modes, and shut it down
// cleanly. Nothing in this package is specific to any particular use
// case — it is the substrate both the `claude-p` CLI drop-in and the
// per-project orchestrator harnesses (e.g. bloodhound) build on top of.
package claudepty

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// ClaudeLaunch is the set of knobs we hand to interactive `claude` when
// driving it from Go. Empty strings / zero values mean "don't pass that
// flag" rather than "pass an empty value." Higher-level packages can
// translate richer options into these.
type ClaudeLaunch struct {
	// Binary is the resolved path to `claude`. Empty defaults to
	// "claude" on PATH (claude.exe on Windows).
	Binary string

	// ExtraArgs are forwarded verbatim. Prefer the named fields below
	// when one exists; ExtraArgs is the escape hatch for flags the
	// caller wants to pass unmolested.
	ExtraArgs []string

	// MCPConfig is the path passed to --mcp-config. Empty = no flag.
	MCPConfig string

	// StrictMCPConfig adds --strict-mcp-config so the launched session
	// loads only the servers in MCPConfig (not the user's global ones).
	StrictMCPConfig bool

	// AllowedTools is the comma-joined list for --allowedTools. Auto-
	// approves listed tools without prompting in the TUI.
	AllowedTools string

	// AppendSystemPrompt is forwarded to --append-system-prompt.
	AppendSystemPrompt string

	// SystemPrompt replaces (not appends) the system prompt via
	// --system-prompt. Mutually exclusive with AppendSystemPrompt in
	// claude's CLI; the caller is responsible for not setting both.
	SystemPrompt string

	// PermissionMode is forwarded to --permission-mode (default,
	// acceptEdits, bypassPermissions, plan). Empty = no flag.
	PermissionMode string

	// SessionID, if non-empty, is forwarded to --session-id. Lets the
	// caller correlate the run with the JSONL claude persists at
	// ~/.claude/projects/**/<id>.jsonl.
	SessionID string

	// Model is forwarded to --model (e.g. "sonnet", "opus", or a full
	// model id). Empty = let claude pick.
	Model string

	// AddDirs are forwarded one-per --add-dir flag.
	AddDirs []string

	// Cwd, if non-empty, becomes the child's working directory.
	Cwd string

	// Env, if non-nil, fully replaces the child env. Use this when you
	// want full control; otherwise leave nil and the spawn will use
	// SubscriptionEnv() (which strips ANTHROPIC_* provider keys).
	Env []string

	// Stderr, if non-nil, receives a copy of everything claude writes
	// to its tty. Useful for diagnostics; doesn't change drive behaviour.
	// (The pty merges stdout+stderr — this just tees the merged stream.)
	Stderr io.Writer
}

// ClaudeSession is one running interactive claude under Go control.
// Methods are safe to call concurrently with the internal pty reader,
// which drains output into a buffer the snapshot methods read from.
type ClaudeSession struct {
	cmd *exec.Cmd
	pty *os.File

	// Read state.
	mu          sync.Mutex
	buf         bytes.Buffer
	lastChunkAt time.Time
	closed      bool

	// done signals the read goroutine to stop.
	done chan struct{}

	// exited closes when the child has been reaped.
	exited chan struct{}
	waitMu sync.Mutex
	wait   error
}

// LaunchClaude spawns interactive claude with the configured flags and
// (by default) a subscription-only env. Caller must Close() the returned
// session when done.
func LaunchClaude(ctx context.Context, l ClaudeLaunch) (*ClaudeSession, error) {
	bin := l.Binary
	if bin == "" {
		bin = "claude"
	}
	args := BuildClaudeArgs(l)
	cmd := exec.CommandContext(ctx, bin, args...)
	if l.Env != nil {
		cmd.Env = l.Env
	} else {
		cmd.Env = SubscriptionEnv()
	}
	if l.Cwd != "" {
		cmd.Dir = l.Cwd
	}

	master, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("spawn claude pty: %w", err)
	}
	cs := &ClaudeSession{
		cmd:         cmd,
		pty:         master,
		lastChunkAt: time.Now(),
		done:        make(chan struct{}),
		exited:      make(chan struct{}),
	}
	go cs.readLoop(l.Stderr)
	go func() {
		err := cs.cmd.Wait()
		cs.waitMu.Lock()
		cs.wait = err
		cs.waitMu.Unlock()
		close(cs.exited)
	}()
	return cs, nil
}

// BuildClaudeArgs assembles the argv for the launch. Exported so
// callers can preview / log the command without actually starting it.
func BuildClaudeArgs(l ClaudeLaunch) []string {
	var args []string
	if l.MCPConfig != "" {
		args = append(args, "--mcp-config", l.MCPConfig)
	}
	if l.StrictMCPConfig {
		args = append(args, "--strict-mcp-config")
	}
	if l.AllowedTools != "" {
		args = append(args, "--allowedTools", l.AllowedTools)
	}
	if l.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", l.AppendSystemPrompt)
	}
	if l.SystemPrompt != "" {
		args = append(args, "--system-prompt", l.SystemPrompt)
	}
	if l.PermissionMode != "" {
		args = append(args, "--permission-mode", l.PermissionMode)
	}
	if l.SessionID != "" {
		args = append(args, "--session-id", l.SessionID)
	}
	if l.Model != "" {
		args = append(args, "--model", l.Model)
	}
	for _, d := range l.AddDirs {
		args = append(args, "--add-dir", d)
	}
	args = append(args, l.ExtraArgs...)
	return args
}

func (cs *ClaudeSession) readLoop(stderr io.Writer) {
	chunk := make([]byte, 65536)
	for {
		select {
		case <-cs.done:
			return
		default:
		}
		n, err := cs.pty.Read(chunk)
		if n > 0 {
			cs.mu.Lock()
			cs.buf.Write(chunk[:n])
			cs.lastChunkAt = time.Now()
			cs.mu.Unlock()
			if stderr != nil {
				_, _ = stderr.Write(chunk[:n])
			}
		}
		if err != nil {
			return
		}
	}
}

// Snapshot returns a copy of the raw pty bytes accumulated so far and
// the duration since the last byte arrived. Safe to call repeatedly.
func (cs *ClaudeSession) Snapshot() ([]byte, time.Duration) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := append([]byte(nil), cs.buf.Bytes()...)
	return out, time.Since(cs.lastChunkAt)
}

// RenderGrid returns the current screen as rendered by the virtual
// terminal (cell positions preserved, stale redraws discarded).
func (cs *ClaudeSession) RenderGrid() string {
	raw, _ := cs.Snapshot()
	return RenderVT(raw)
}

// Write sends raw bytes into the pty. Most callers want SendKeys (which
// honours Go-style escape sequences) instead.
func (cs *ClaudeSession) Write(p []byte) (int, error) {
	return cs.pty.Write(p)
}

// SendKeys writes text into the pty, interpreting Go-style escape
// sequences (\r, \n, \t, \xNN, \uNNNN) on the way. Useful for callers
// that receive keystrokes as JSON-shaped strings.
func (cs *ClaudeSession) SendKeys(text string) (int, error) {
	return cs.pty.Write([]byte(UnescapeKeys(text)))
}

// ErrProcessExited is returned when claude exited before the event the
// waiter was looking for.
var ErrProcessExited = errors.New("claude process exited")

// ErrTimeout is returned by WaitForDone if the budget elapses without
// the done signal firing.
var ErrTimeout = errors.New("interactive orchestrator timeout")

// WaitForReady waits up to budget for the input prompt to appear,
// answering the "Is this a project you trust?" modal claude shows the
// first time a folder is opened. Returns nil once the main input row
// is visible.
func (cs *ClaudeSession) WaitForReady(ctx context.Context, budget time.Duration) error {
	const settle = 400 * time.Millisecond
	deadline := time.Now().Add(budget)
	trustHandled := false

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-cs.exited:
			return ErrProcessExited
		default:
		}
		raw, sinceLast := cs.Snapshot()
		if len(raw) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		screen := RenderVT(raw)
		lower := strings.ToLower(screen)

		if !trustHandled {
			if strings.Contains(lower, "trust this folder") || strings.Contains(lower, "do you trust") {
				if sinceLast >= settle {
					// Option 1 == Yes; \r confirms.
					if _, err := cs.pty.Write([]byte("\r")); err != nil {
						return fmt.Errorf("answer trust modal: %w", err)
					}
					trustHandled = true
					time.Sleep(750 * time.Millisecond)
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}
			trustHandled = true
		}

		if HasInputPrompt(screen) && sinceLast >= settle {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return ErrTimeout
}

// SendPrompt types the prompt + Enter into the live pty. Splits the
// text from the Enter so claude has a moment to ingest before submit
// (paste-detection heuristics inside the TUI have historically been
// twitchy about huge single writes).
func (cs *ClaudeSession) SendPrompt(prompt string) error {
	if _, err := cs.pty.Write([]byte(prompt)); err != nil {
		return fmt.Errorf("write prompt: %w", err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := cs.pty.Write([]byte("\r")); err != nil {
		return fmt.Errorf("submit prompt: %w", err)
	}
	return nil
}

// WaitForDone blocks until one of: done closes (nil return), the
// process exits (ErrProcessExited), ctx ends (ctx.Err()), or budget
// elapses (ErrTimeout). budget=0 means "ctx is the only ceiling."
func (cs *ClaudeSession) WaitForDone(ctx context.Context, done <-chan struct{}, budget time.Duration) error {
	var deadline <-chan time.Time
	if budget > 0 {
		t := time.NewTimer(budget)
		defer t.Stop()
		deadline = t.C
	}
	for {
		select {
		case <-done:
			return nil
		case <-cs.exited:
			return ErrProcessExited
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return ErrTimeout
		}
	}
}

// Exited returns a channel that closes when claude has exited and been
// reaped. After it closes, WaitErr returns the exit status.
func (cs *ClaudeSession) Exited() <-chan struct{} { return cs.exited }

// WaitErr returns the result of the internal cmd.Wait. Safe to call
// before Exited() closes — returns nil until the process actually exits.
func (cs *ClaudeSession) WaitErr() error {
	cs.waitMu.Lock()
	defer cs.waitMu.Unlock()
	return cs.wait
}

// Exit asks claude to teardown cleanly via two Ctrl-C's, then SIGKILLs
// after a short grace if it didn't. Safe to call multiple times.
func (cs *ClaudeSession) Exit() {
	_, _ = cs.pty.Write([]byte{0x03})
	time.Sleep(150 * time.Millisecond)
	_, _ = cs.pty.Write([]byte{0x03})

	select {
	case <-cs.exited:
		return
	case <-time.After(750 * time.Millisecond):
	}
	if cs.cmd.Process != nil {
		_ = cs.cmd.Process.Kill()
	}
}

// Close releases the pty and ensures the process is reaped. After
// Close the session is unusable.
func (cs *ClaudeSession) Close() error {
	cs.mu.Lock()
	if cs.closed {
		cs.mu.Unlock()
		return nil
	}
	cs.closed = true
	close(cs.done)
	cs.mu.Unlock()

	_ = cs.pty.Close()
	if cs.cmd.Process != nil {
		_ = cs.cmd.Process.Kill()
	}
	// Wait for the read goroutine + reaper to finish so callers don't
	// race with the buffer.
	<-cs.exited
	return nil
}

// PtyFD returns the underlying pty master file. Useful when a caller
// wants to do something custom with it (e.g. resize). Don't close it —
// the ClaudeSession owns the lifecycle.
func (cs *ClaudeSession) PtyFD() *os.File { return cs.pty }
