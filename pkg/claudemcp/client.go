package claudemcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// Client is a thin connection to a running BridgeServer. The relay
// subprocess uses one; callers writing tests may also want to use it
// directly.
//
// Not safe for concurrent use: one Call at a time per Client. If you
// need parallelism, open multiple clients (cheap — just another unix
// socket connection).
type Client struct {
	conn net.Conn
	mu   sync.Mutex
	r    *bufio.Reader
	w    *bufio.Writer
}

// Dial opens a connection to the bridge at path.
func Dial(path string) (*Client, error) {
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("dial bridge %s: %w", path, err)
	}
	return &Client{
		conn: c,
		r:    bufio.NewReader(c),
		w:    bufio.NewWriter(c),
	}, nil
}

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Call sends one request and waits for the response. tool is the tool
// name; args is anything JSON-marshallable (may be nil for no-arg
// tools / system tools).
//
// Returns the raw result bytes on success or an error if the bridge
// reported one or the connection died mid-call.
func (c *Client) Call(tool string, args any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var raw json.RawMessage
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("encode args for %s: %w", tool, err)
		}
		raw = b
	}
	req := BridgeRequest{Tool: tool, Args: raw}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode request for %s: %w", tool, err)
	}
	if _, err := c.w.Write(append(b, '\n')); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	if err := c.w.Flush(); err != nil {
		return nil, fmt.Errorf("flush request: %w", err)
	}

	line, err := c.r.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var resp BridgeResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.Err != "" {
		return nil, fmt.Errorf("bridge: %s", resp.Err)
	}
	return resp.Result, nil
}

// ListTools fetches the list of registered tool specs from the bridge.
// Convenience over Call(SystemToolListTools, nil).
func (c *Client) ListTools() ([]ToolSpec, error) {
	raw, err := c.Call(SystemToolListTools, nil)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Tools []ToolSpec `json:"tools"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("decode tools list: %w", err)
	}
	return wrapped.Tools, nil
}
