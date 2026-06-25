package drivemcp

import (
	"context"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// call finds a registered tool by name and invokes its handler with args.
func call(t *testing.T, s *Server, name string, args map[string]any) *mcpgo.CallToolResult {
	t.Helper()
	for _, reg := range s.tools() {
		if reg.tool.Name == name {
			var req mcpgo.CallToolRequest
			req.Params.Name = name
			req.Params.Arguments = args
			res, err := reg.handler(context.Background(), req)
			if err != nil {
				t.Fatalf("%s handler returned transport error: %v", name, err)
			}
			return res
		}
	}
	t.Fatalf("tool %q not registered", name)
	return nil
}

func resultText(res *mcpgo.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcpgo.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestToolsRegistered(t *testing.T) {
	s := New(Config{})
	want := []string{
		"launch_claude", "prompt", "prompt_async", "read_response",
		"read_transcript", "read_screen", "send_keys", "wait_for_ready",
		"interrupt", "list_sessions", "stop_claude",
	}
	got := map[string]bool{}
	for _, reg := range s.tools() {
		got[reg.tool.Name] = true
		if reg.tool.Description == "" {
			t.Errorf("tool %q has no description", reg.tool.Name)
		}
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing tool %q", w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d tools, want %d", len(got), len(want))
	}
}

func TestRequiredArgsEnforced(t *testing.T) {
	s := New(Config{})
	// prompt requires session_id and text; omit both.
	res := call(t, s, "prompt", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected error result for missing required args, got %+v", res)
	}
	if !strings.Contains(resultText(res), "session_id") {
		t.Errorf("error should mention the missing session_id, got %q", resultText(res))
	}
}

func TestUnknownSessionErrors(t *testing.T) {
	s := New(Config{})
	for _, name := range []string{"prompt", "prompt_async", "read_response", "read_screen", "send_keys", "wait_for_ready", "interrupt", "stop_claude"} {
		args := map[string]any{"session_id": "does-not-exist", "text": "x", "since_offset": 0}
		res := call(t, s, name, args)
		if !res.IsError {
			t.Errorf("%s: expected error for unknown session, got %+v", name, res)
		}
		if !strings.Contains(resultText(res), "unknown session") {
			t.Errorf("%s: error should say 'unknown session', got %q", name, resultText(res))
		}
	}
}

func TestUnknownBackendRejected(t *testing.T) {
	s := New(Config{})
	res := call(t, s, "launch_claude", map[string]any{"backend": "bogus"})
	if !res.IsError {
		t.Fatalf("expected error for bogus backend, got %+v", res)
	}
	if !strings.Contains(resultText(res), "unknown backend") {
		t.Errorf("error should say 'unknown backend', got %q", resultText(res))
	}
}

func TestListSessionsNoInproc(t *testing.T) {
	// A fresh server tracks no in-process sessions. It may still surface daemon
	// sessions if a pupptyeer daemon happens to be running, so assert on the
	// tracked (in-process) entries rather than the total count.
	s := New(Config{})
	res := call(t, s, "list_sessions", map[string]any{})
	if res.IsError {
		t.Fatalf("list_sessions errored: %q", resultText(res))
	}
	sc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("expected structured content, got %T", res.StructuredContent)
	}
	sessions, _ := sc["sessions"].([]map[string]any)
	for _, entry := range sessions {
		if entry["tracked"] == true {
			t.Errorf("fresh server should track no in-process sessions, got %v", entry)
		}
	}
}

func TestReadTranscriptUnknownSession(t *testing.T) {
	s := New(Config{})
	res := call(t, s, "read_transcript", map[string]any{"session_id": "does-not-exist"})
	if !res.IsError {
		t.Fatalf("expected error for a session with no transcript, got %+v", res)
	}
	if !strings.Contains(resultText(res), "no transcript") {
		t.Errorf("error should mention the missing transcript, got %q", resultText(res))
	}
}

func TestConfigDefaults(t *testing.T) {
	s := New(Config{})
	if s.cfg.ServerName == "" || s.cfg.LaunchTimeout == 0 || s.cfg.PromptTimeout == 0 {
		t.Errorf("withDefaults left fields unset: %+v", s.cfg)
	}
}

func TestCompactScreen(t *testing.T) {
	in := "  \n" + "hello   \n" + "   \n" + "world \n" + "   \n"
	got := compactScreen(in)
	if got != "hello\nworld" {
		t.Errorf("compactScreen = %q, want %q", got, "hello\nworld")
	}
}
