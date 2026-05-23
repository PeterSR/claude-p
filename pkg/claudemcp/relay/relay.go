// Package relay runs the MCP-over-stdio server that claude launches
// when it loads our bridge via --mcp-config. The relay dials the
// in-process BridgeServer, asks it for the list of registered tools,
// re-exposes them as MCP tools, and forwards each tool call across the
// unix socket.
//
// Callers usually invoke this from a small subcommand in their own
// binary (so they don't need a separate `claude-p` binary on PATH):
//
//	func runRelay(socketPath string) error {
//	    return relay.Serve(relay.Options{
//	        SocketPath: socketPath,
//	        ServerName: "myapp-bridge",
//	    })
//	}
//
// Then in the MCP config you write for claude:
//
//	{"mcpServers":{"myapp-bridge":{"command":"myapp","args":["_mcp_relay","/path/sock"]}}}
package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/PeterSR/claude-p/pkg/claudemcp"
)

// Options configures one relay run.
type Options struct {
	// SocketPath is the unix socket the in-process BridgeServer is
	// listening on. Required.
	SocketPath string

	// ServerName appears in MCP discovery; defaults to "claude-p".
	ServerName string

	// ServerVersion appears in MCP discovery; defaults to "0.1.0".
	ServerVersion string
}

// Serve runs until stdin EOF or an unrecoverable error. Designed to be
// the entire body of a subcommand: just call it and return its error.
func Serve(opts Options) error {
	if opts.SocketPath == "" {
		return fmt.Errorf("relay: SocketPath is required")
	}
	if opts.ServerName == "" {
		opts.ServerName = "claude-p"
	}
	if opts.ServerVersion == "" {
		opts.ServerVersion = "0.1.0"
	}

	bridge, err := claudemcp.Dial(opts.SocketPath)
	if err != nil {
		return fmt.Errorf("relay: %w", err)
	}
	defer bridge.Close()

	tools, err := bridge.ListTools()
	if err != nil {
		return fmt.Errorf("relay: list tools: %w", err)
	}

	mcpServer := server.NewMCPServer(opts.ServerName, opts.ServerVersion)
	for _, t := range tools {
		registerTool(mcpServer, bridge, t)
	}

	// Helpful diagnostic when run by mistake without claude on the
	// other side. Stderr is invisible to claude itself.
	fmt.Fprintf(os.Stderr, "claude-p relay: serving %d tool(s) for %s on %s\n",
		len(tools), opts.ServerName, opts.SocketPath)

	return server.ServeStdio(mcpServer)
}

func registerTool(mcpServer *server.MCPServer, bridge *claudemcp.Client, spec claudemcp.ToolSpec) {
	options := []mcpgo.ToolOption{mcpgo.WithDescription(spec.Description)}
	for _, p := range spec.Params {
		options = append(options, paramOption(p))
	}
	tool := mcpgo.NewTool(spec.Name, options...)

	mcpServer.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		args := req.GetArguments()
		raw, err := bridge.Call(spec.Name, args)
		if err != nil {
			return mcpgo.NewToolResultErrorf("bridge: %v", err), nil
		}
		// If the result is a JSON object, expose it as structured
		// content so the LLM can reason over the fields; otherwise
		// return as text.
		var asMap map[string]any
		if uerr := json.Unmarshal(raw, &asMap); uerr == nil {
			return mcpgo.NewToolResultStructured(asMap, string(raw)), nil
		}
		return mcpgo.NewToolResultText(string(raw)), nil
	})
}

func paramOption(p claudemcp.Param) mcpgo.ToolOption {
	innerOpts := []mcpgo.PropertyOption{}
	if p.Description != "" {
		innerOpts = append(innerOpts, mcpgo.Description(p.Description))
	}
	if p.Required {
		innerOpts = append(innerOpts, mcpgo.Required())
	}
	switch p.Type {
	case "integer":
		return mcpgo.WithNumber(p.Name, innerOpts...)
	case "number":
		return mcpgo.WithNumber(p.Name, innerOpts...)
	case "boolean":
		return mcpgo.WithBoolean(p.Name, innerOpts...)
	case "array":
		return mcpgo.WithArray(p.Name, innerOpts...)
	case "object":
		return mcpgo.WithObject(p.Name, innerOpts...)
	default:
		return mcpgo.WithString(p.Name, innerOpts...)
	}
}
