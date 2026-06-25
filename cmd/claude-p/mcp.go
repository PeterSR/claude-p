package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/PeterSR/claude-p/pkg/drivemcp"
)

var (
	mcpBackend        string
	mcpSocket         string
	mcpBinary         string
	mcpPermissionMode string
	mcpLaunchTimeout  int
	mcpPromptTimeout  int
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run a high-level claude-driving MCP server over stdio",
	Long: `mcp runs a Model Context Protocol server (over stdio) that lets an outer
Claude Code — or any MCP client — drive inner, interactive ` + "`claude`" + ` sessions
with conversation-shaped tools instead of raw keystrokes.

Tools exposed:
  launch_claude   boot (or continue) a session and wait until it is ready
  prompt          send a message and get the model's full answer back
  prompt_async    send a message without blocking; collect it later
  read_response   read the result of a prompt_async turn (poll or wait)
  read_transcript review past turns / what an in-flight turn is doing
  read_screen     peek at the TUI when a turn needs a keystroke, not a reply
  send_keys       answer an interactive prompt (menu choice, Esc, Ctrl-C)
  wait_for_ready  block until the session is back at its input prompt
  interrupt       send Esc to cancel a running turn
  list_sessions   list the sessions this server manages
  stop_claude     cleanly stop a session (Ctrl-C twice, then terminate)

This is higher-level than a raw pty MCP (e.g. pupptyeer's): prompt does the
send-and-wait-for-the-answer dance for you, lifting the reply straight from
claude's persisted transcript rather than scraping the screen.

Add it to Claude Code with:
  claude mcp add claude-p-drive -- claude-p mcp

The --backend default (daemon) needs a running pupptyeer daemon; pass
--backend inproc for a zero-dependency in-process pty whose sessions live for
this server's lifetime. Individual launch_claude calls can override the backend.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var daemon bool
		switch strings.ToLower(mcpBackend) {
		case "daemon":
			daemon = true
		case "inproc", "inprocess", "in-process":
			daemon = false
		default:
			return fmt.Errorf("unknown --backend %q (want daemon or inproc)", mcpBackend)
		}

		srv := drivemcp.New(drivemcp.Config{
			ServerName:            "claude-p-drive",
			ServerVersion:         version,
			Binary:                mcpBinary,
			DefaultDaemon:         daemon,
			PupptyeerSocket:       mcpSocket,
			DefaultPermissionMode: mcpPermissionMode,
			LaunchTimeout:         time.Duration(mcpLaunchTimeout) * time.Second,
			PromptTimeout:         time.Duration(mcpPromptTimeout) * time.Second,
		})
		if err := srv.Serve(); err != nil {
			return fmt.Errorf("mcp: %w", err)
		}
		return nil
	},
}

func init() {
	f := mcpCmd.Flags()
	f.StringVar(&mcpBackend, "backend", "daemon", "default backend for launched sessions: daemon (persistent; needs a running pupptyeer daemon) or inproc (no external dependency; sessions live for this server's lifetime)")
	f.StringVar(&mcpSocket, "pupptyeer-socket", "", "pupptyeer daemon socket path (daemon backend only; default: $PUPPTYEER_SOCK or the standard per-user location)")
	f.StringVar(&mcpBinary, "binary", "", "path to the claude executable launched sessions use (default: claude on PATH)")
	f.StringVar(&mcpPermissionMode, "permission-mode", "", "default --permission-mode for launched sessions (default|acceptEdits|bypassPermissions|plan); per-session overridable")
	f.IntVar(&mcpLaunchTimeout, "launch-timeout", 60, "seconds to wait for a launched session to reach its prompt")
	f.IntVar(&mcpPromptTimeout, "prompt-timeout", 300, "default seconds to wait for a prompt turn to finish; per-call overridable")
	rootCmd.AddCommand(mcpCmd)
}
