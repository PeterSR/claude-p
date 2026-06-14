package claudep

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestQueryInprocE2E drives real claude in-process through the full Query path
// and asserts the answer is captured. Gated behind CLAUDE_P_E2E because it
// spawns claude and spends subscription tokens:
//
//	CLAUDE_P_E2E=1 go test ./pkg/claudep -run InprocE2E -v
func TestQueryInprocE2E(t *testing.T) {
	if os.Getenv("CLAUDE_P_E2E") == "" {
		t.Skip("set CLAUDE_P_E2E=1 to run (spawns claude, spends tokens)")
	}
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := Query(ctx, Options{
		Prompt:       "Reply with only the digit: what is 2+2?",
		OutputFormat: FormatText,
		Cwd:          t.TempDir(),
		Stdout:       &out,
		Timeout:      110 * time.Second,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !strings.Contains(res.FinalText, "4") {
		t.Errorf("FinalText = %q, want it to contain 4", res.FinalText)
	}
	if !res.TerminalSeen {
		t.Error("TerminalSeen = false, want true")
	}
	if res.JSONLPath == "" {
		t.Error("JSONLPath empty; transcript was not located")
	}
}
