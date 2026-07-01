package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/PeterSR/claude-p/pkg/claudep"
)

var (
	awaitSessionID string
	awaitSince     int64
	awaitTimeout   int
)

// awaitTurnCmd is a machine-facing helper: it blocks until an inner claude turn
// completes, then exits 0. It exists to be armed as a background/monitor command
// by a client of the `claude-p mcp` server - prompt_async hands back the exact
// invocation string - so it is hidden from the everyday CLI (like mcp-relay).
// Because it waits on claude's persisted transcript, not the pty, it is
// authoritative (the end-of-turn marker, not screen quiescence) and works for
// both the in-process and daemon backends.
var awaitTurnCmd = &cobra.Command{
	Use:    "await-turn",
	Short:  "Block until an inner claude turn completes (internal; armed by prompt_async)",
	Hidden: true,
	Long: `await-turn blocks until the claude turn that started at --since (a transcript
byte offset, as returned by the MCP server's prompt_async tool) reaches its
end-of-turn marker, then exits 0. On timeout it exits non-zero.

You normally don't run this by hand: the claude-p mcp server's prompt_async tool
returns a ready-to-arm invocation string for it, so a client can wait for a turn
out-of-band (arm it as a monitor, do other work, get woken on exit) instead of
blocking a tool call. Confirm the result afterwards with the read_response tool.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if awaitSessionID == "" {
			return fmt.Errorf("--session-id is required")
		}
		ctx := context.Background()
		if awaitTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(awaitTimeout)*time.Second)
			defer cancel()
		}
		_, done, err := claudep.AwaitTurn(ctx, awaitSessionID, awaitSince)
		if err != nil {
			return err
		}
		if !done {
			return fmt.Errorf("await-turn: timed out after %ds before the turn completed", awaitTimeout)
		}
		return nil
	},
}

func init() {
	f := awaitTurnCmd.Flags()
	f.StringVar(&awaitSessionID, "session-id", "", "session id whose turn to await")
	f.Int64Var(&awaitSince, "since", 0, "transcript byte offset captured before the turn (from prompt_async)")
	f.IntVar(&awaitTimeout, "timeout", 600, "max seconds to wait before giving up (0 = no timeout)")
	rootCmd.AddCommand(awaitTurnCmd)
}
