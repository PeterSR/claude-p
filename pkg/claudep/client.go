package claudep

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/PeterSR/claude-p/pkg/claudepty"
)

// compactScreen drops blank lines and trailing spaces from a rendered screen so
// it can be shown in a diagnostic without the 200x60 padding.
func compactScreen(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(strings.TrimRight(line, " "))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// Result summarises one Query invocation. The output has already been
// written to Options.Stdout in the chosen format; Result exists for
// library callers that want to react to the run programmatically.
type Result struct {
	SessionID    string
	FinalText    string
	JSONLPath    string
	DurationMs   int64
	TerminalSeen bool
}

// applyDefaults fills in the option fields shared by every entry point.
func applyDefaults(opts *Options) {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
}

// resolveSessionID returns the caller's session id or a fresh random one.
func resolveSessionID(opts Options) string {
	if opts.SessionID != "" {
		return opts.SessionID
	}
	return claudepty.NewSessionID()
}

// resolveCwd resolves the working directory to an absolute path up front. The
// daemon backend spawns claude in a separate process whose default cwd is the
// daemon's, not ours — so we must pass an explicit cwd or the session lands in
// the wrong project (wrong transcript location, wrong --resume scope).
func resolveCwd(opts Options) string {
	if opts.Cwd != "" {
		return opts.Cwd
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return opts.Cwd
}

// prepareSession launches (or reattaches) the pty backend and waits until
// claude is sitting at the input prompt. On success it returns a live session
// the caller owns (Close it); on failure it closes the session itself. reused
// reports a continued daemon session that's already past the trust modal and at
// the prompt, so it skips the wait.
func prepareSession(ctx context.Context, opts Options, sessionID, cwd string) (claudepty.PTYSession, bool, error) {
	launch := buildLaunch(opts, sessionID, cwd)

	sess, reused, err := newBackend(opts, sessionID, launch)
	if err != nil {
		return nil, false, fmt.Errorf("claudep: %w", err)
	}

	// A continued (reused) daemon session is already past the trust modal and
	// sitting at the input prompt — only fresh launches need to wait.
	if !reused {
		// 45s, not 20s: first boot can be slow (loading MCP servers, plugins,
		// large project context) before the prompt appears. Bounded by ctx.
		if err := claudepty.WaitForReady(ctx, sess, 45*time.Second); err != nil {
			scr, _ := sess.CaptureScreen(0, 500*time.Millisecond)
			screen := scr.Text()
			// Surface what claude was actually showing so a "never reached the
			// prompt" failure is diagnosable (login screen, an unrecognised
			// modal, slow boot, a nested-session oddity, ...) instead of opaque.
			if compact := compactScreen(screen); compact != "" {
				fmt.Fprintf(opts.Stderr, "claudep: claude never reached the input prompt within 45s; last rendered screen:\n%s\n", compact)
			}
			_ = sess.Close()
			if failure := claudepty.ClassifyInteractiveFailure(screen); failure != "" {
				return nil, false, fmt.Errorf("claudep: %s (%w)", failure, err)
			}
			return nil, false, fmt.Errorf("claudep: claude never reached input prompt: %w", err)
		}
	}

	return sess, reused, nil
}

// buildLaunch maps the claude-p Options onto the claudepty.ClaudeLaunch knobs
// both backends consume. Shared by the in-process and daemon launch paths.
func buildLaunch(opts Options, sessionID, cwd string) claudepty.ClaudeLaunch {
	return claudepty.ClaudeLaunch{
		Binary:             opts.Binary,
		MCPConfig:          firstNonEmpty(opts.MCPConfig),
		StrictMCPConfig:    opts.StrictMCPConfig,
		AllowedTools:       joinComma(opts.AllowedTools),
		AppendSystemPrompt: opts.AppendSystemPrompt,
		SystemPrompt:       opts.SystemPrompt,
		PermissionMode:     opts.PermissionMode,
		SessionID:          sessionID,
		Model:              opts.Model,
		AddDirs:            opts.AddDirs,
		Cwd:                cwd,
		ExtraArgs:          remainingPassthroughArgs(opts),
	}
}

// StartIdle boots a pupptyeer daemon session, waits until claude is sitting at
// the input prompt, then detaches without sending a prompt — leaving the TUI
// warm so a later Query with the returned SessionID continues the conversation
// without paying the startup cost. It forces daemon mode (an in-process pty
// dies with the call, so there is nothing to leave idle). No Prompt is needed.
func StartIdle(ctx context.Context, opts Options) (*Result, error) {
	applyDefaults(&opts)
	// Idle-start only makes sense against the daemon; force it on regardless of
	// what the caller set so the session outlives this call.
	opts.PupptyeerDaemon = true
	sessionID := resolveSessionID(opts)
	cwd := resolveCwd(opts)

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	sess, _, err := prepareSession(runCtx, opts, sessionID, cwd)
	if err != nil {
		return nil, err
	}
	// Detach only — leave the live claude warm at the prompt for continuation.
	_ = sess.Close()

	return &Result{SessionID: sessionID, JSONLPath: claudepty.JSONLPath(sessionID)}, nil
}

// Query runs one user prompt against an interactive claude session and
// emits the result in the chosen format to Options.Stdout. Blocks until
// claude produces a terminal assistant message, the timeout fires, or
// ctx is cancelled.
func Query(ctx context.Context, opts Options) (*Result, error) {
	if opts.Prompt == "" {
		return nil, fmt.Errorf("claudep: Prompt is required")
	}
	if opts.OutputFormat == "" {
		opts.OutputFormat = FormatText
	}
	applyDefaults(&opts)
	sessionID := resolveSessionID(opts)
	cwd := resolveCwd(opts)

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	sess, _, err := prepareSession(runCtx, opts, sessionID, cwd)
	if err != nil {
		return nil, err
	}
	// One-shot owns the session and tears it down; daemon mode detaches and
	// leaves the live claude alive for the next invocation to continue.
	defer sess.Close()

	// Record where the transcript currently ends BEFORE sending the prompt, so
	// the tail only sees the new turn (a continued conversation already has all
	// prior turns on disk).
	startOffset := claudepty.JSONLOffset(sessionID)

	if err := claudepty.SendPrompt(sess, opts.Prompt); err != nil {
		return nil, fmt.Errorf("claudep: send prompt: %w", err)
	}

	em := newEmitter(opts.Stdout, opts.OutputFormat, sessionID, cwd)
	em.init()

	jsonlPath := claudepty.WaitForJSONL(sessionID, 10*time.Second)
	if jsonlPath == "" {
		return nil, fmt.Errorf("claudep: persisted JSONL never appeared for session %s — is claude actually running?", sessionID)
	}
	em.setJSONLPath(jsonlPath)

	// Each turn opens with claude logging our submitted prompt as a `user`
	// event, then the assistant reply, then a trailing `system turn_duration`.
	// When continuing a live session the PRIOR turn's turn_duration can still be
	// flushing to disk just past startOffset; without this gate the tail would
	// mistake that stale marker for the new turn's completion and emit nothing.
	// Only accept a terminal event once we've seen this turn's user echo.
	sawUserTurn := false
	err = tailJSONL(runCtx, jsonlPath, startOffset, func(ev tailEvent) (bool, error) {
		em.handle(ev)
		if ev.Type == "user" {
			sawUserTurn = true
		}
		// Stop on any terminal event — either a terminal assistant text
		// message OR the system turn_duration marker (which catches
		// tool-only turns where the model is satisfied without emitting
		// a final text response) — but only after this turn's user echo.
		if ev.Terminal && sawUserTurn {
			return true, nil
		}
		return false, nil
	})
	// ctx-cancelled is OK here if we got our terminal message; we'll
	// surface ctx errors only if no terminal text was captured.
	if err != nil && em.finalText == "" {
		return nil, fmt.Errorf("claudep: %w", err)
	}

	if opts.PupptyeerDaemon {
		// Leave the live session running for continuation; detach only. A
		// still-alive session has no meaningful exit code, so leave it nil
		// (the envelope tolerates that).
		_ = sess.Close()
	} else {
		// One-shot: cleanly exit claude so we can report a real exit code.
		claudepty.Shutdown(sess)
		if exited, code := sess.Exited(); exited {
			em.setExitCode(&code)
		}
	}

	em.finish()

	return &Result{
		SessionID:    sessionID,
		FinalText:    em.finalText,
		JSONLPath:    jsonlPath,
		DurationMs:   time.Since(em.startedAt).Milliseconds(),
		TerminalSeen: em.terminalSeen,
	}, nil
}

// remainingPassthroughArgs handles the flags that aren't first-class on
// claudepty.ClaudeLaunch — everything Options carries that isn't
// already shaped onto launch fields.
func remainingPassthroughArgs(o Options) []string {
	// Build a "scratch" Options identical to o but with the fields
	// already placed on ClaudeLaunch cleared, then run BuildArgs over
	// it. This keeps the passthrough list in one place (flags.go).
	scratch := o
	scratch.AllowedTools = nil
	scratch.AppendSystemPrompt = ""
	scratch.SystemPrompt = ""
	scratch.PermissionMode = ""
	scratch.Model = ""
	scratch.AddDirs = nil
	scratch.MCPConfig = nil
	scratch.StrictMCPConfig = false
	return BuildArgs(scratch)
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func joinComma(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += "," + s
	}
	return out
}

// ensure io.Writer is referenced (keeps the import in case future
// helpers need it).
var _ io.Writer = (io.Writer)(nil)
