package claudep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTranscript drops the given JSONL lines into a temp file and returns its path.
func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A representative turn: a user prompt, a thinking-only assistant message (no
// visible text), a tool-use-only assistant message, the final text answer, and
// the trailing turn_duration marker.
var transcriptLines = []string{
	`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"}],"stop_reason":"end_turn"}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{}}],"stop_reason":"tool_use"}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn"}}`,
	`{"type":"system","subtype":"turn_duration"}`,
}

func TestReadTranscriptSkipsEmptyAndSystem(t *testing.T) {
	// Without tools, the thinking-only and tool-use-only messages have no visible
	// text and are dropped; the system marker is never a message. Left: the user
	// prompt and the final assistant answer.
	got, err := readTranscriptFrom(writeTranscript(t, transcriptLines...), 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].Role != "user" || got[0].Text != "hello" {
		t.Errorf("entry 0 = %+v, want {user, hello}", got[0])
	}
	if got[1].Role != "assistant" || got[1].Text != "done" {
		t.Errorf("entry 1 = %+v, want {assistant, done}", got[1])
	}
	for _, e := range got {
		if len(e.Tools) != 0 {
			t.Errorf("tools should be omitted when include_tools=false, got %+v", e)
		}
	}
}

func TestReadTranscriptIncludeTools(t *testing.T) {
	// With tools, the tool-use-only message is kept and carries the tool name,
	// even though it has no visible text.
	got, err := readTranscriptFrom(writeTranscript(t, transcriptLines...), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d: %+v", len(got), got)
	}
	mid := got[1]
	if mid.Text != "" || len(mid.Tools) != 1 || mid.Tools[0] != "Bash" {
		t.Errorf("middle entry = %+v, want empty text with Tools [Bash]", mid)
	}
}

func TestReadTranscriptLastN(t *testing.T) {
	got, err := readTranscriptFrom(writeTranscript(t, transcriptLines...), 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "done" {
		t.Fatalf("last_n=1 should keep only the final answer, got %+v", got)
	}
}

func TestReadTranscriptMissingFile(t *testing.T) {
	if _, err := readTranscriptFrom(filepath.Join(t.TempDir(), "nope.jsonl"), 0, false); err == nil {
		t.Fatal("expected an error for a missing transcript file")
	}
}
