// Package claudemcp is the bridge framework used to expose a set of
// in-process Go tools to an interactive `claude` session over the MCP
// protocol. The pattern is:
//
//  1. The orchestrator host (whatever process spawns claude) starts a
//     BridgeServer in-process. The server listens on a unix socket and
//     dispatches incoming requests to tool handlers the host registered.
//  2. The host writes an MCP config that points claude at a small
//     "relay" subprocess (via --mcp-config). claude launches the relay
//     when it needs the MCP server.
//  3. The relay dials the bridge socket, asks for the list of tools,
//     and re-exposes them over MCP's stdio JSON-RPC. When claude calls
//     a tool, the relay forwards the call across the socket to the host
//     and returns the result.
//
// This lets you keep tool implementations in your own process (with
// access to your own state, logging, etc.) while still letting an
// interactive claude session call them.
package claudemcp

import "encoding/json"

// ProtocolVersion bumps when the wire format between BridgeServer and
// relay subprocess changes. Set as the first request from the relay so
// older bridges can reject incompatible newer relays gracefully.
const ProtocolVersion = 1

// SystemToolListTools is the special tool name the relay uses to ask
// the bridge "what tools do you expose?" — not registered by callers,
// always present.
const SystemToolListTools = "__list_tools__"

// BridgeRequest is the message the relay sends to the bridge. Tool is
// the tool name; Args is the raw JSON arguments. Args may be empty for
// system tools.
type BridgeRequest struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args,omitempty"`
}

// BridgeResponse is the message the bridge sends back. Exactly one of
// Result and Err is populated per request.
type BridgeResponse struct {
	Result json.RawMessage `json:"result,omitempty"`
	Err    string          `json:"err,omitempty"`
}

// Param describes one tool parameter. It's a deliberately small subset
// of JSON Schema — enough to map cleanly onto mcp-go's WithXxx options
// without dragging the whole schema vocabulary across the wire.
type Param struct {
	// Name is the argument key the caller will pass.
	Name string `json:"name"`

	// Type is one of: "string", "integer", "number", "boolean",
	// "array", "object". Anything else is treated as "string" by the
	// relay for forward-compat.
	Type string `json:"type"`

	// Description is the human-readable hint shown to the LLM.
	Description string `json:"description,omitempty"`

	// Required marks the param as mandatory. The relay translates this
	// to an mcp.Required() option.
	Required bool `json:"required,omitempty"`
}

// ToolSpec is the public-facing description of one tool. The bridge
// returns a list of these in response to SystemToolListTools so the
// relay knows what to register with claude.
type ToolSpec struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Params      []Param `json:"params,omitempty"`
}
