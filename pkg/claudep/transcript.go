package claudep

import (
	"bufio"
	"fmt"
	"os"

	"github.com/PeterSR/claude-p/pkg/claudepty"
)

// TranscriptEntry is one message lifted from a session's persisted JSONL: the
// role, its visible text (thinking is excluded), and - when asked - the names of
// any tools the assistant invoked in that message.
type TranscriptEntry struct {
	Role  string   `json:"role"`            // "user" | "assistant"
	Text  string   `json:"text,omitempty"`  // user/assistant visible text
	Tools []string `json:"tools,omitempty"` // tool_use names (assistant messages)
}

// ReadTranscript returns the conversation recorded for sessionID, reading
// claude's own persisted JSONL off disk (so it works regardless of backend, and
// for sessions this process never launched). lastN > 0 keeps only the most
// recent N entries; includeTools attaches the tool names each assistant message
// invoked (useful for seeing what an in-flight turn is doing). Messages with
// neither visible text nor (when requested) tool activity - bare thinking blocks
// or tool_result echoes - are skipped.
func ReadTranscript(sessionID string, lastN int, includeTools bool) ([]TranscriptEntry, error) {
	path := claudepty.JSONLPath(sessionID)
	if path == "" {
		return nil, fmt.Errorf("claudep: no transcript on disk for session %s", sessionID)
	}
	return readTranscriptFrom(path, lastN, includeTools)
}

// readTranscriptFrom is ReadTranscript against an explicit path, split out so the
// parsing is testable without a real ~/.claude/projects transcript.
func readTranscriptFrom(path string, lastN int, includeTools bool) ([]TranscriptEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var out []TranscriptEntry
	for sc.Scan() {
		ev, derr := decodeJSONLLine(bytesTrimNewline(append([]byte(nil), sc.Bytes()...)))
		if derr != nil || ev.Parsed == nil {
			continue
		}
		if ev.Type != "assistant" && ev.Type != "user" {
			continue
		}
		entry := TranscriptEntry{Role: ev.Parsed.Role, Text: ev.Text}
		if entry.Role == "" {
			entry.Role = ev.Type
		}
		if includeTools {
			for _, b := range ev.Parsed.Content {
				if b.Type == "tool_use" && b.Name != "" {
					entry.Tools = append(entry.Tools, b.Name)
				}
			}
		}
		if entry.Text == "" && len(entry.Tools) == 0 {
			continue
		}
		out = append(out, entry)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if lastN > 0 && len(out) > lastN {
		out = out[len(out)-lastN:]
	}
	return out, nil
}
