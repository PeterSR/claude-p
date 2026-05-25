package claudep

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/PeterSR/claude-p/pkg/claudepty"
)

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

// Query runs one user prompt against an interactive claude session and
// emits the result in the chosen format to Options.Stdout. Blocks until
// claude produces a terminal assistant message, the timeout fires, or
// ctx is cancelled.
func Query(ctx context.Context, opts Options) (*Result, error) {
	if opts.Prompt == "" {
		return nil, fmt.Errorf("claudep: Prompt is required")
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.OutputFormat == "" {
		opts.OutputFormat = FormatText
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
	sessionID := opts.SessionID
	if sessionID == "" {
		sessionID = claudepty.NewSessionID()
	}

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	launch := claudepty.ClaudeLaunch{
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
		Cwd:                opts.Cwd,
		ExtraArgs:          remainingPassthroughArgs(opts),
	}
	sess, err := claudepty.LaunchClaude(runCtx, launch)
	if err != nil {
		return nil, fmt.Errorf("claudep: %w", err)
	}
	defer sess.Close()

	if err := sess.WaitForReady(runCtx, 20*time.Second); err != nil {
		failure := claudepty.ClassifyInteractiveFailure(sess.RenderGrid())
		if failure != "" {
			return nil, fmt.Errorf("claudep: %s (%w)", failure, err)
		}
		return nil, fmt.Errorf("claudep: claude never reached input prompt: %w", err)
	}

	if err := sess.SendPrompt(opts.Prompt); err != nil {
		return nil, fmt.Errorf("claudep: send prompt: %w", err)
	}

	cwd := opts.Cwd
	if cwd == "" {
		if wd, werr := os.Getwd(); werr == nil {
			cwd = wd
		}
	}
	em := newEmitter(opts.Stdout, opts.OutputFormat, sessionID, cwd)
	em.init()

	jsonlPath := claudepty.WaitForJSONL(sessionID, 10*time.Second)
	if jsonlPath == "" {
		return nil, fmt.Errorf("claudep: persisted JSONL never appeared for session %s — is claude actually running?", sessionID)
	}
	em.setJSONLPath(jsonlPath)

	err = tailJSONL(runCtx, jsonlPath, func(ev tailEvent) (bool, error) {
		em.handle(ev)
		// Stop on any terminal event — either a terminal assistant text
		// message OR the system turn_duration marker (which catches
		// tool-only turns where the model is satisfied without emitting
		// a final text response).
		if ev.Terminal {
			return true, nil
		}
		return false, nil
	})
	// ctx-cancelled is OK here if we got our terminal message; we'll
	// surface ctx errors only if no terminal text was captured.
	if err != nil && em.finalText == "" {
		return nil, fmt.Errorf("claudep: %w", err)
	}

	// Cleanly exit the interactive session before the final envelope —
	// gives us a real exit code to report.
	sess.Exit()
	if waitErr := sess.WaitErr(); waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			em.setExitCode(&code)
		}
	} else {
		zero := 0
		em.setExitCode(&zero)
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
