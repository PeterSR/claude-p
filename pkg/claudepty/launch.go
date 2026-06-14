package claudepty

// ClaudeLaunch is the set of knobs we hand to interactive `claude` when
// driving it. Empty strings / zero values mean "don't pass that flag" rather
// than "pass an empty value." Higher-level packages translate richer options
// into these; both PTY backends (in-process and daemon) consume them.
type ClaudeLaunch struct {
	// Binary is the resolved path to `claude`. Empty defaults to "claude" on
	// PATH (claude.exe on Windows).
	Binary string

	// ExtraArgs are forwarded verbatim. Prefer the named fields below when one
	// exists; ExtraArgs is the escape hatch for flags passed unmolested.
	ExtraArgs []string

	// MCPConfig is the path passed to --mcp-config. Empty = no flag.
	MCPConfig string

	// StrictMCPConfig adds --strict-mcp-config so the launched session loads
	// only the servers in MCPConfig (not the user's global ones).
	StrictMCPConfig bool

	// AllowedTools is the comma-joined list for --allowedTools.
	AllowedTools string

	// AppendSystemPrompt is forwarded to --append-system-prompt.
	AppendSystemPrompt string

	// SystemPrompt replaces the system prompt via --system-prompt. Mutually
	// exclusive with AppendSystemPrompt in claude's CLI; the caller is
	// responsible for not setting both.
	SystemPrompt string

	// PermissionMode is forwarded to --permission-mode (default, acceptEdits,
	// bypassPermissions, plan). Empty = no flag.
	PermissionMode string

	// SessionID, if non-empty, is forwarded to --session-id. Correlates the run
	// with the JSONL claude persists at ~/.claude/projects/**/<id>.jsonl.
	SessionID string

	// Resume, if non-empty, is forwarded to --resume <id> to reload a prior
	// conversation. Mutually exclusive with SessionID; the caller picks one.
	Resume string

	// Model is forwarded to --model. Empty = let claude pick.
	Model string

	// AddDirs are forwarded one-per --add-dir flag.
	AddDirs []string

	// Cwd, if non-empty, becomes the child's working directory.
	Cwd string

	// Env, if non-nil, fully replaces the child env. Leave nil to use
	// SubscriptionEnv() (which strips ANTHROPIC_* provider keys).
	Env []string
}

// BuildClaudeArgs assembles the argv for the launch. Exported so callers can
// preview / log the command without starting it.
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
	// --session-id and --resume are mutually exclusive; Resume wins when set
	// (continuation reloads an existing transcript).
	if l.Resume != "" {
		args = append(args, "--resume", l.Resume)
	} else if l.SessionID != "" {
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
