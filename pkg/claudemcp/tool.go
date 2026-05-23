package claudemcp

import (
	"encoding/json"
	"fmt"
)

// Handler is the server-side function that runs when a registered tool
// is invoked. It receives the raw JSON args (caller-provided) and must
// return a value that can be JSON-marshaled, or an error.
type Handler func(args json.RawMessage) (any, error)

// Tool bundles a tool's spec (what gets shipped to the relay) and its
// handler (what runs when the tool fires). Callers build these via
// NewTool and register them with BridgeServer.AddTool.
type Tool struct {
	Spec    ToolSpec
	Handler Handler
}

// NewTool constructs a Tool with a name, description, parameter list,
// and a handler that already knows how to decode the args. The handler
// is responsible for unmarshalling args into whatever struct it needs.
//
// Typical usage:
//
//	type ReadPTYArgs struct{ SettleMs int `json:"settle_ms"` }
//	t := claudemcp.NewTool(
//	    "read_pty",
//	    "Read the rendered screen after waiting settle_ms for the pty to be quiet.",
//	    []claudemcp.Param{{Name: "settle_ms", Type: "integer", Required: true}},
//	    func(raw json.RawMessage) (any, error) {
//	        var a ReadPTYArgs
//	        if err := json.Unmarshal(raw, &a); err != nil { return nil, err }
//	        return doRead(a.SettleMs), nil
//	    },
//	)
func NewTool(name, description string, params []Param, handler Handler) Tool {
	return Tool{
		Spec: ToolSpec{
			Name:        name,
			Description: description,
			Params:      params,
		},
		Handler: handler,
	}
}

// DecodeArgs is a small helper for handlers: unmarshals raw args into a
// destination struct, returning a wrapped error if it fails.
func DecodeArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode args: %w", err)
	}
	return nil
}
