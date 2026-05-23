package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/PeterSR/claude-p/pkg/claudemcp/relay"
)

var (
	relayServerName    string
	relayServerVersion string
)

var mcpRelayCmd = &cobra.Command{
	Use:   "mcp-relay <socket-path>",
	Short: "Run the MCP bridge relay (claude launches this via --mcp-config)",
	Long: `mcp-relay dials a unix-socket BridgeServer hosted by another process,
asks for the list of tools it has registered, and re-exposes them as
MCP tools over stdio. claude launches this command when it needs the
MCP server you configured via --mcp-config.

You usually don't run mcp-relay manually. Use the
pkg/claudemcp/relay.Serve library function instead if you want to host
the relay inside your own binary (no PATH dependency on the claude-p
executable).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := relay.Serve(relay.Options{
			SocketPath:    args[0],
			ServerName:    relayServerName,
			ServerVersion: relayServerVersion,
		}); err != nil {
			return fmt.Errorf("relay: %w", err)
		}
		return nil
	},
}

func init() {
	mcpRelayCmd.Flags().StringVar(&relayServerName, "server-name", "claude-p", "name reported to MCP discovery")
	mcpRelayCmd.Flags().StringVar(&relayServerVersion, "server-version", version, "version reported to MCP discovery")
	rootCmd.AddCommand(mcpRelayCmd)
}
