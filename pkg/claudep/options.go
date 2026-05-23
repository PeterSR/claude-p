// Package claudep is the Go API behind the `claude-p` CLI: drive an
// interactive `claude` session in a pty, capture the assistant's reply,
// and emit it in a `claude -p`-shaped envelope (text / json / stream-json).
//
// Use this when you want `claude -p` behaviour from a Go program but
// you only have an interactive Claude Code subscription available
// (i.e. you don't want your tokens going through the Agent SDK credit
// path).
//
// For the lower-level "drive a pty" primitives — without any of the
// `claude -p` output-format reshaping — see pkg/claudepty.
package claudep

import (
	"io"
	"time"
)

// OutputFormat selects how Query writes results to Options.Stdout.
type OutputFormat string

const (
	// FormatText writes only the final assistant text, no metadata.
	// Matches `claude -p --output-format text`.
	FormatText OutputFormat = "text"

	// FormatJSON writes one JSON object summarizing the run on
	// completion. Matches `claude -p --output-format json` in shape;
	// usage/cost fields are nil (the interactive TUI does not expose
	// per-turn token counts).
	FormatJSON OutputFormat = "json"

	// FormatStreamJSON writes one JSON event per line as the session
	// progresses. Shape is `claude -p --output-format stream-json`:
	// a synthetic system init, then claude's own assistant/user
	// events forwarded from the persisted JSONL, then a synthetic
	// result event.
	FormatStreamJSON OutputFormat = "stream-json"
)

// Options describes one Query invocation. Most fields map 1:1 onto
// `claude` CLI flags. Empty / nil = "don't pass that flag".
type Options struct {
	// Prompt is the user message to send. Required.
	Prompt string

	// Binary is the resolved path to `claude`. Empty = "claude" on
	// PATH.
	Binary string

	// SessionID, if non-empty, is passed as --session-id. claude-p
	// always sets one even if empty: it needs to be able to find the
	// persisted JSONL for the assistant text. An empty SessionID
	// triggers a fresh one to be generated automatically.
	SessionID string

	// OutputFormat picks the output shape. Empty = FormatText.
	OutputFormat OutputFormat

	// Cwd, if non-empty, becomes the spawned claude's working dir.
	Cwd string

	// Timeout caps the entire run. Default 5 minutes.
	Timeout time.Duration

	// Stdout / Stderr receive the formatted output and diagnostics
	// respectively. Default to os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// Passthrough claude flags (in alphabetical order).

	AddDirs                          []string
	Agent                            string
	Agents                           string
	AllowedTools                     []string
	AppendSystemPrompt               string
	Betas                            []string
	Brief                            bool
	Chrome                           bool
	NoChrome                         bool
	ContinueSession                  bool
	DangerouslySkipPermissions       bool
	AllowDangerouslySkipPermissions  bool
	Debug                            string // optional value; empty + DebugSet = bare "--debug"
	DebugSet                         bool
	DebugFile                        string
	DisableSlashCommands             bool
	DisallowedTools                  []string
	Effort                           string
	ExcludeDynamicSystemPromptSections bool
	Files                            []string
	ForkSession                      bool
	FromPR                           string
	FromPRSet                        bool
	IDE                              bool
	JSONSchema                       string
	MCPConfig                        []string
	MCPDebug                         bool
	Model                            string
	Name                             string
	PermissionMode                   string
	PluginDirs                       []string
	PluginURLs                       []string
	RemoteControl                    string
	RemoteControlSet                 bool
	RemoteControlSessionNamePrefix   string
	Resume                           string
	ResumeSet                        bool
	SettingSources                   string
	Settings                         string
	StrictMCPConfig                  bool
	SystemPrompt                     string
	Tools                            []string
	Tmux                             string
	TmuxSet                          bool
	Worktree                         string
	WorktreeSet                      bool
}
