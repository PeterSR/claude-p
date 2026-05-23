// claude-p is a Go drop-in for `claude -p` backed by an interactive
// Claude Code TUI session. Same CLI shape, same output formats, but
// the tokens come out of your interactive subscription instead of the
// Agent SDK credit / extra-usage path that `claude -p` itself draws on.
//
// Run `claude-p --help` for the CLI surface or import
// github.com/PeterSR/claude-p/pkg/claudep for the library.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Build-time injected; goreleaser fills these in.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "claude-p [flags] <prompt>",
	Short: "Run a one-shot prompt against interactive Claude Code (claude -p drop-in)",
	Long: `claude-p drives interactive Claude Code in a pty so you can use it
the same way you would use ` + "`claude -p`" + ` — with the same flags and the same
output formats — but on top of your existing Claude Code subscription
login instead of the Agent SDK billing surface.

Provide the prompt as the final positional argument, or pipe it on
stdin if you'd prefer.

For library usage and the MCP bridge framework, see
https://github.com/PeterSR/claude-p`,
	SilenceUsage: true,
	Version:      version,
}

func main() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("claude-p %s (commit %s, %s)\n", version, commit, date))
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
