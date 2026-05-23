package claudep

// BuildArgs converts the passthrough flag fields of Options into the
// argv string we hand to interactive `claude`. The prompt itself is
// NOT included here — it is typed into the live pty after the input
// row renders.
//
// Kept in its own file because the list is long and noisy; exported so
// CLI / tests can preview the argv.
func BuildArgs(o Options) []string {
	var args []string

	appendRepeated := func(flag string, values []string) {
		for _, v := range values {
			args = append(args, flag, v)
		}
	}
	appendValue := func(flag, value string) {
		if value != "" {
			args = append(args, flag, value)
		}
	}
	appendFlag := func(flag string, enabled bool) {
		if enabled {
			args = append(args, flag)
		}
	}
	appendOptional := func(flag, value string, set bool) {
		if !set {
			return
		}
		args = append(args, flag)
		if value != "" {
			args = append(args, value)
		}
	}

	appendRepeated("--add-dir", o.AddDirs)
	appendValue("--agent", o.Agent)
	appendValue("--agents", o.Agents)
	appendFlag("--allow-dangerously-skip-permissions", o.AllowDangerouslySkipPermissions)
	appendRepeated("--allowedTools", o.AllowedTools)
	appendValue("--append-system-prompt", o.AppendSystemPrompt)
	appendRepeated("--betas", o.Betas)
	appendFlag("--brief", o.Brief)
	appendFlag("--chrome", o.Chrome)
	appendFlag("--no-chrome", o.NoChrome)
	appendFlag("--continue", o.ContinueSession)
	appendFlag("--dangerously-skip-permissions", o.DangerouslySkipPermissions)
	appendOptional("--debug", o.Debug, o.DebugSet)
	appendValue("--debug-file", o.DebugFile)
	appendFlag("--disable-slash-commands", o.DisableSlashCommands)
	appendRepeated("--disallowedTools", o.DisallowedTools)
	appendValue("--effort", o.Effort)
	appendFlag("--exclude-dynamic-system-prompt-sections", o.ExcludeDynamicSystemPromptSections)
	appendRepeated("--file", o.Files)
	appendFlag("--fork-session", o.ForkSession)
	appendOptional("--from-pr", o.FromPR, o.FromPRSet)
	appendFlag("--ide", o.IDE)
	appendValue("--json-schema", o.JSONSchema)
	appendRepeated("--mcp-config", o.MCPConfig)
	appendFlag("--mcp-debug", o.MCPDebug)
	appendRepeated("--tools", o.Tools)
	appendValue("--model", o.Model)
	appendValue("--name", o.Name)
	appendValue("--permission-mode", o.PermissionMode)
	appendRepeated("--plugin-dir", o.PluginDirs)
	appendRepeated("--plugin-url", o.PluginURLs)
	appendOptional("--remote-control", o.RemoteControl, o.RemoteControlSet)
	appendValue("--remote-control-session-name-prefix", o.RemoteControlSessionNamePrefix)
	appendOptional("--resume", o.Resume, o.ResumeSet)
	appendValue("--setting-sources", o.SettingSources)
	appendValue("--settings", o.Settings)
	appendFlag("--strict-mcp-config", o.StrictMCPConfig)
	appendValue("--system-prompt", o.SystemPrompt)
	appendOptional("--tmux", o.Tmux, o.TmuxSet)
	appendOptional("--worktree", o.Worktree, o.WorktreeSet)

	return args
}
