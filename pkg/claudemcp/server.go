package claudemcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BridgeServer accepts connections from the relay subprocess and
// dispatches incoming tool calls to registered handlers. One server
// usually serves one claude session for the lifetime of an orchestrator
// turn; concurrent sessions need separate servers (and separate sockets).
type BridgeServer struct {
	ln   net.Listener
	path string

	mu     sync.RWMutex
	tools  map[string]Tool
	closed bool

	t0 time.Time

	// Trace, if non-nil, receives one JSON-encoded entry per tool call
	// (request + result). Useful for replaying or auditing an
	// orchestrator's drive sequence.
	Trace io.Writer
}

// NewServer creates a server bound to a unix socket. If path is empty
// the server picks a unique path under SocketDir().
//
// The socket is created with 0600 permissions; callers shouldn't need
// to widen this since the relay always runs as the same user.
func NewServer(path string) (*BridgeServer, error) {
	if path == "" {
		dir, err := SocketDir()
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
		path = filepath.Join(dir, fmt.Sprintf("claudemcp-%d-%d.sock",
			os.Getpid(), time.Now().UnixNano()))
	}
	// Remove any stale socket at this path before binding.
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &BridgeServer{
		ln:    ln,
		path:  path,
		tools: make(map[string]Tool),
		t0:    time.Now(),
	}, nil
}

// SocketDir returns a directory under which BridgeServer can drop its
// sockets when no explicit path is given. Honours $XDG_RUNTIME_DIR on
// Unix; falls back to os.TempDir.
func SocketDir() (string, error) {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "claude-p"), nil
	}
	return filepath.Join(os.TempDir(), "claude-p"), nil
}

// Path returns the socket path. Pass this to the relay via argv or env.
func (s *BridgeServer) Path() string { return s.path }

// AddTool registers a tool. Adding a tool with a name that already
// exists overwrites the previous registration. Calling AddTool while
// Serve is running is safe.
func (s *BridgeServer) AddTool(t Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[t.Spec.Name] = t
}

// AddTools registers a batch of tools.
func (s *BridgeServer) AddTools(tools ...Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range tools {
		s.tools[t.Spec.Name] = t
	}
}

// Tools returns the currently-registered tool specs (sorted by name is
// not guaranteed; callers should sort if order matters).
func (s *BridgeServer) Tools() []ToolSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ToolSpec, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, t.Spec)
	}
	return out
}

// Serve runs until Close is called or the listener errors. Each
// accepted connection corresponds to one relay subprocess; we don't
// multiplex across processes on the same socket.
func (s *BridgeServer) Serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()
			if closed {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go s.handle(conn)
	}
}

// Close shuts the listener and removes the socket file. Safe to call
// repeatedly.
func (s *BridgeServer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	err := s.ln.Close()
	_ = os.Remove(s.path)
	return err
}

func (s *BridgeServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if !errors.Is(err, io.EOF) {
				// Drop; the relay died. Nothing to do here.
			}
			return
		}
		var req BridgeRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = writeResp(w, BridgeResponse{Err: "decode request: " + err.Error()})
			continue
		}
		resp := s.dispatch(req)
		s.trace(req, resp)
		if err := writeResp(w, resp); err != nil {
			return
		}
	}
}

func (s *BridgeServer) dispatch(req BridgeRequest) BridgeResponse {
	if req.Tool == SystemToolListTools {
		specs := s.Tools()
		b, err := json.Marshal(map[string]any{"tools": specs})
		if err != nil {
			return BridgeResponse{Err: "encode list_tools: " + err.Error()}
		}
		return BridgeResponse{Result: b}
	}
	s.mu.RLock()
	t, ok := s.tools[req.Tool]
	s.mu.RUnlock()
	if !ok {
		return BridgeResponse{Err: "unknown tool: " + req.Tool}
	}
	out, err := t.Handler(req.Args)
	if err != nil {
		return BridgeResponse{Err: err.Error()}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return BridgeResponse{Err: "encode result: " + err.Error()}
	}
	return BridgeResponse{Result: b}
}

func (s *BridgeServer) trace(req BridgeRequest, resp BridgeResponse) {
	if s.Trace == nil {
		return
	}
	entry := map[string]any{
		"t_ms": time.Since(s.t0).Milliseconds(),
		"tool": req.Tool,
		"args": json.RawMessage(req.Args),
	}
	if resp.Err != "" {
		entry["err"] = resp.Err
	}
	if len(resp.Result) > 0 {
		entry["result"] = json.RawMessage(resp.Result)
	}
	b, _ := json.Marshal(entry)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.Trace.Write(append(b, '\n'))
}

func writeResp(w *bufio.Writer, resp BridgeResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return w.Flush()
}
