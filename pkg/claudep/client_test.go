package claudep

import (
	"context"
	"testing"
	"time"
)

func TestCompactScreen(t *testing.T) {
	in := "  \n" + "hello   \n" + "   \n" + "world \n" + "   \n"
	if got := CompactScreen(in); got != "hello\nworld" {
		t.Errorf("CompactScreen = %q, want %q", got, "hello\nworld")
	}
	if got := CompactScreen("   \n  \n"); got != "" {
		t.Errorf("all-blank input should compact to empty, got %q", got)
	}
}

func TestApplyDefaultsMCPSettle(t *testing.T) {
	var none Options
	applyDefaults(&none)
	if none.MCPSettle != 0 {
		t.Errorf("no MCP config: MCPSettle = %v, want 0", none.MCPSettle)
	}
	with := Options{MCPConfig: []string{"mcp.json"}}
	applyDefaults(&with)
	if with.MCPSettle != DefaultMCPSettle {
		t.Errorf("MCP config: MCPSettle = %v, want %v", with.MCPSettle, DefaultMCPSettle)
	}
	off := Options{MCPConfig: []string{"mcp.json"}, MCPSettle: -1}
	applyDefaults(&off)
	if off.MCPSettle >= 0 {
		t.Errorf("negative MCPSettle was overwritten: %v", off.MCPSettle)
	}
}

func TestSettleMCP(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	if err := settleMCP(ctx, Options{MCPSettle: time.Second}); err != nil || time.Since(start) > 200*time.Millisecond {
		t.Errorf("no MCP config must not wait (err=%v, took %v)", err, time.Since(start))
	}
	start = time.Now()
	if err := settleMCP(ctx, Options{MCPConfig: []string{"m"}, MCPSettle: 50 * time.Millisecond}); err != nil || time.Since(start) < 50*time.Millisecond {
		t.Errorf("must wait the settle (err=%v, took %v)", err, time.Since(start))
	}
	if err := settleMCP(ctx, Options{MCPConfig: []string{"m"}, MCPSettle: -1}); err != nil {
		t.Errorf("negative settle must not wait: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := settleMCP(cancelled, Options{MCPConfig: []string{"m"}, MCPSettle: time.Minute}); err == nil {
		t.Error("a cancelled context must end the wait")
	}
}
