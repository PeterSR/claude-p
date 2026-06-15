package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/PeterSR/claude-p/pkg/claudep"
)

var runOpts claudep.Options

// runOutputFormat is bound to --output-format and parsed into
// claudep.OutputFormat when the run command fires.
var runOutputFormat string

// runTimeout is bound to --timeout (seconds). We expose seconds rather
// than a Duration string to match `claude -p`'s `--timeout` flavour.
var runTimeoutSec int

// Optional-value flag plumbing — cobra doesn't have a great built-in
// for "optional value" so we shadow each with a string and a bool.
// User passes `--debug=foo` or bare `--debug`; we set DebugSet=true in
// both cases.
type optStr struct {
	val string
	set bool
}

func (o *optStr) String() string { return o.val }
func (o *optStr) Set(v string) error {
	o.val = v
	o.set = true
	return nil
}
func (o *optStr) Type() string { return "string" }

var (
	flagDebug         optStr
	flagFromPR        optStr
	flagRemoteControl optStr
	flagResume        optStr
	flagTmux          optStr
	flagWorktree      optStr
)

func runE(cmd *cobra.Command, args []string) error {
	// Idle-start boots a warm daemon session and detaches without a prompt, so
	// it skips prompt resolution entirely. Handle it before everything else.
	if runOpts.PupptyeerStartIdle {
		res, err := claudep.StartIdle(context.Background(), runOpts)
		if err != nil {
			return err
		}
		fmt.Println(res.SessionID)
		return nil
	}

	prompt, err := resolvePrompt(args)
	if err != nil {
		return err
	}
	runOpts.Prompt = prompt

	switch strings.ToLower(runOutputFormat) {
	case "", "text":
		runOpts.OutputFormat = claudep.FormatText
	case "json":
		runOpts.OutputFormat = claudep.FormatJSON
	case "stream-json":
		runOpts.OutputFormat = claudep.FormatStreamJSON
	default:
		return fmt.Errorf("unknown --output-format %q (want text|json|stream-json)", runOutputFormat)
	}

	if runTimeoutSec > 0 {
		runOpts.Timeout = time.Duration(runTimeoutSec) * time.Second
	}

	// Splice optional-value flags onto Options.
	runOpts.Debug, runOpts.DebugSet = flagDebug.val, flagDebug.set
	runOpts.FromPR, runOpts.FromPRSet = flagFromPR.val, flagFromPR.set
	runOpts.RemoteControl, runOpts.RemoteControlSet = flagRemoteControl.val, flagRemoteControl.set
	runOpts.Resume, runOpts.ResumeSet = flagResume.val, flagResume.set
	runOpts.Tmux, runOpts.TmuxSet = flagTmux.val, flagTmux.set
	runOpts.Worktree, runOpts.WorktreeSet = flagWorktree.val, flagWorktree.set

	_, err = claudep.Query(context.Background(), runOpts)
	return err
}

// resolvePrompt returns the prompt from argv if present, otherwise from
// stdin if it's piped. If neither, returns an error directing the user
// to provide one.
func resolvePrompt(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	if stdinIsPiped() {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read prompt from stdin: %w", err)
		}
		p := strings.TrimSpace(string(b))
		if p == "" {
			return "", fmt.Errorf("no prompt provided (stdin was empty)")
		}
		return p, nil
	}
	return "", fmt.Errorf("no prompt provided — pass it as the last argument or pipe it on stdin")
}

func stdinIsPiped() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

func init() {
	c := &cobra.Command{
		Use:   "run [flags] <prompt>",
		Short: "Run one prompt and emit the result (default command)",
		RunE:  runE,
		Args:  cobra.ArbitraryArgs,
	}

	f := c.Flags()
	// -p / --print is `claude`'s switch into headless mode. claude-p is
	// always headless, so the flag is a no-op here — accepted purely so
	// existing `claude -p ...` invocations work verbatim when the binary
	// is swapped to claude-p.
	var noopPrint bool
	f.BoolVarP(&noopPrint, "print", "p", false, "no-op; claude-p is always headless")
	_ = f.MarkHidden("print")
	f.StringVar(&runOpts.Binary, "binary", "", "path to the claude executable (default: claude on PATH)")
	f.StringVar(&runOpts.SessionID, "session-id", "", "session id passed to claude --session-id (default: random)")
	f.StringVar(&runOpts.Cwd, "cwd", "", "working directory for the claude session")
	f.StringVar(&runOutputFormat, "output-format", "text", "output format: text|json|stream-json")
	f.IntVar(&runTimeoutSec, "timeout", 300, "overall timeout in seconds")

	// pupptyeer daemon backend (persistent, multi-turn). Default is an
	// in-process pty (one-shot, no external binary).
	f.BoolVar(&runOpts.PupptyeerDaemon, "pupptyeer-daemon", false, "drive claude through a running pupptyeer daemon (persistent; same --session-id continues the conversation) instead of an in-process pty")
	f.StringVar(&runOpts.PupptyeerSocket, "pupptyeer-socket", "", "pupptyeer daemon socket path (default: $PUPPTYEER_SOCK or the standard per-user location)")
	f.StringVar(&runOpts.PupptyeerBin, "pupptyeer-bin", "", "pupptyeer binary used to auto-start a daemon if none is running (default: $PUPPTYEER_BIN or pupptyeer on PATH)")
	f.BoolVar(&runOpts.PupptyeerStartIdle, "pupptyeer-start-idle", false, "boot a daemon session, wait until claude is at the prompt, print the session id, and detach without sending a prompt (implies --pupptyeer-daemon; continue later with run --session-id <id>)")

	// Passthrough flags — alphabetical to match flags.go.
	f.StringSliceVar(&runOpts.AddDirs, "add-dir", nil, "passed to claude --add-dir (repeatable)")
	f.StringVar(&runOpts.Agent, "agent", "", "passed to claude --agent")
	f.StringVar(&runOpts.Agents, "agents", "", "passed to claude --agents")
	f.BoolVar(&runOpts.AllowDangerouslySkipPermissions, "allow-dangerously-skip-permissions", false, "passed to claude")
	f.StringSliceVar(&runOpts.AllowedTools, "allowedTools", nil, "passed to claude --allowedTools (repeatable)")
	f.StringVar(&runOpts.AppendSystemPrompt, "append-system-prompt", "", "passed to claude")
	f.StringSliceVar(&runOpts.Betas, "betas", nil, "passed to claude --betas (repeatable)")
	f.BoolVar(&runOpts.Brief, "brief", false, "passed to claude")
	f.BoolVar(&runOpts.Chrome, "chrome", false, "passed to claude")
	f.BoolVar(&runOpts.NoChrome, "no-chrome", false, "passed to claude")
	f.BoolVar(&runOpts.ContinueSession, "continue", false, "passed to claude")
	f.BoolVar(&runOpts.DangerouslySkipPermissions, "dangerously-skip-permissions", false, "passed to claude")
	f.Var(&flagDebug, "debug", "passed to claude --debug (optional value)")
	f.Lookup("debug").NoOptDefVal = ""
	f.StringVar(&runOpts.DebugFile, "debug-file", "", "passed to claude")
	f.BoolVar(&runOpts.DisableSlashCommands, "disable-slash-commands", false, "passed to claude")
	f.StringSliceVar(&runOpts.DisallowedTools, "disallowedTools", nil, "passed to claude --disallowedTools (repeatable)")
	f.StringVar(&runOpts.Effort, "effort", "", "passed to claude")
	f.BoolVar(&runOpts.ExcludeDynamicSystemPromptSections, "exclude-dynamic-system-prompt-sections", false, "passed to claude")
	f.StringSliceVar(&runOpts.Files, "file", nil, "passed to claude --file (repeatable)")
	f.BoolVar(&runOpts.ForkSession, "fork-session", false, "passed to claude")
	f.Var(&flagFromPR, "from-pr", "passed to claude --from-pr (optional value)")
	f.Lookup("from-pr").NoOptDefVal = ""
	f.BoolVar(&runOpts.IDE, "ide", false, "passed to claude")
	f.StringVar(&runOpts.JSONSchema, "json-schema", "", "passed to claude")
	f.StringSliceVar(&runOpts.MCPConfig, "mcp-config", nil, "passed to claude --mcp-config (repeatable)")
	f.BoolVar(&runOpts.MCPDebug, "mcp-debug", false, "passed to claude")
	f.StringSliceVar(&runOpts.Tools, "tools", nil, "passed to claude --tools (repeatable)")
	f.StringVar(&runOpts.Model, "model", "", "passed to claude")
	f.StringVar(&runOpts.Name, "name", "", "passed to claude")
	f.StringVar(&runOpts.PermissionMode, "permission-mode", "", "passed to claude")
	f.StringSliceVar(&runOpts.PluginDirs, "plugin-dir", nil, "passed to claude --plugin-dir (repeatable)")
	f.StringSliceVar(&runOpts.PluginURLs, "plugin-url", nil, "passed to claude --plugin-url (repeatable)")
	f.Var(&flagRemoteControl, "remote-control", "passed to claude --remote-control (optional value)")
	f.Lookup("remote-control").NoOptDefVal = ""
	f.StringVar(&runOpts.RemoteControlSessionNamePrefix, "remote-control-session-name-prefix", "", "passed to claude")
	f.Var(&flagResume, "resume", "passed to claude --resume (optional value)")
	f.Lookup("resume").NoOptDefVal = ""
	f.StringVar(&runOpts.SettingSources, "setting-sources", "", "passed to claude")
	f.StringVar(&runOpts.Settings, "settings", "", "passed to claude")
	f.BoolVar(&runOpts.StrictMCPConfig, "strict-mcp-config", false, "passed to claude")
	f.StringVar(&runOpts.SystemPrompt, "system-prompt", "", "passed to claude")
	f.Var(&flagTmux, "tmux", "passed to claude --tmux (optional value)")
	f.Lookup("tmux").NoOptDefVal = ""
	f.Var(&flagWorktree, "worktree", "passed to claude --worktree (optional value)")
	f.Lookup("worktree").NoOptDefVal = ""

	rootCmd.AddCommand(c)

	// Also make the run command the default — running `claude-p
	// "hello"` should not require typing the literal `run` subcommand.
	// cobra has no built-in for this; we override RunE on rootCmd to
	// fall through.
	rootCmd.RunE = runE
	rootCmd.Args = cobra.ArbitraryArgs
	rootCmd.Flags().AddFlagSet(f)
}
