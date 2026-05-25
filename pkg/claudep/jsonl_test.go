package claudep

import (
	"encoding/json"
	"testing"
)

func TestDecodeJSONLLine_assistantTerminal(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"id":"x","model":"opus","content":[{"type":"text","text":"42"}],"stop_reason":"end_turn"}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != "assistant" {
		t.Errorf("Type = %q, want assistant", ev.Type)
	}
	if !ev.Terminal {
		t.Errorf("Terminal = false, want true for stop_reason=end_turn")
	}
	if ev.Text != "42" {
		t.Errorf("Text = %q, want %q", ev.Text, "42")
	}
}

func TestDecodeJSONLLine_assistantToolUseNotTerminal(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"id":"x","content":[{"type":"tool_use"}],"stop_reason":"tool_use"}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Terminal {
		t.Error("Terminal = true, want false for stop_reason=tool_use (still working)")
	}
}

func TestDecodeJSONLLine_thinkingOnlyNotTerminal(t *testing.T) {
	// Extended thinking sometimes produces an assistant message with
	// stop_reason=end_turn but only a "thinking" content block — no
	// text. That is NOT the user-visible final answer; the real text
	// arrives in the next assistant message.
	line := []byte(`{"type":"assistant","message":{"id":"x","content":[{"type":"thinking","thinking":""}],"stop_reason":"end_turn"}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Terminal {
		t.Error("thinking-only message should NOT be marked terminal even with stop_reason=end_turn")
	}
}

func TestDecodeJSONLLine_thinkingPlusTextIsTerminal(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"id":"x","content":[{"type":"thinking","thinking":""},{"type":"text","text":"the answer"}],"stop_reason":"end_turn"}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Terminal {
		t.Error("end_turn with text content alongside thinking should be terminal")
	}
	if ev.Text != "the answer" {
		t.Errorf("Text = %q, want %q", ev.Text, "the answer")
	}
}

func TestDecodeJSONLLine_assistantPauseTurnNotTerminal(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"id":"x","content":[],"stop_reason":"pause_turn"}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Terminal {
		t.Error("Terminal = true, want false for stop_reason=pause_turn")
	}
}

func TestDecodeJSONLLine_assistantNoStopReasonNotTerminal(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"id":"x","content":[{"type":"text","text":"partial"}]}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Terminal {
		t.Error("Terminal = true, want false for missing stop_reason")
	}
	if ev.Text != "partial" {
		t.Errorf("Text = %q, want partial", ev.Text)
	}
}

func TestDecodeJSONLLine_userEvent(t *testing.T) {
	line := []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != "user" {
		t.Errorf("Type = %q, want user", ev.Type)
	}
	if ev.Terminal {
		t.Error("user events are never terminal in our model")
	}
}

func TestDecodeJSONLLine_systemTurnDurationIsTerminal(t *testing.T) {
	// claude fires `system.subtype=turn_duration` at the very end of
	// every turn — even when the model finishes with just a tool call
	// and no final assistant text. Tailing on assistant terminal only
	// would block indefinitely on such turns.
	line := []byte(`{"type":"system","subtype":"turn_duration","durationMs":11281,"messageCount":15}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Terminal {
		t.Error("system.subtype=turn_duration should be terminal")
	}
	if ev.Type != "system" {
		t.Errorf("Type = %q, want system", ev.Type)
	}
}

func TestDecodeJSONLLine_otherSystemEventsNotTerminal(t *testing.T) {
	// Other system events (stop_hook_summary, etc.) are NOT the canonical
	// "turn is over" marker — only turn_duration is.
	line := []byte(`{"type":"system","subtype":"stop_hook_summary","hookCount":5}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Terminal {
		t.Error("system.subtype=stop_hook_summary should not be terminal")
	}
}

func TestDecodeJSONLLine_internalEventsIgnored(t *testing.T) {
	// Non-user/assistant events shouldn't crash and shouldn't carry
	// terminal/text fields populated.
	for _, line := range []string{
		`{"type":"file-history-snapshot","messageId":"x"}`,
		`{"type":"permission-mode","permissionMode":"acceptEdits"}`,
		`{"type":"last-prompt","leafUuid":"x"}`,
	} {
		ev, err := decodeJSONLLine([]byte(line))
		if err != nil {
			t.Errorf("%s: %v", line, err)
		}
		if ev.Terminal {
			t.Errorf("%s: should not be terminal", line)
		}
		if ev.Text != "" {
			t.Errorf("%s: text should be empty, got %q", line, ev.Text)
		}
	}
}

func TestDecodeJSONLLine_toolUseFieldsCaptured(t *testing.T) {
	// tool_use blocks must surface id/name/input — they're what
	// observability consumers (e.g. ai-assistant) pair against the
	// later tool_result.
	line := []byte(`{"type":"assistant","message":{"id":"x","content":[{"type":"tool_use","id":"toolu_1","name":"send_message","input":{"text":"hi"}}],"stop_reason":"tool_use"}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Parsed == nil || len(ev.Parsed.Content) != 1 {
		t.Fatalf("expected one parsed content block; got %+v", ev.Parsed)
	}
	b := ev.Parsed.Content[0]
	if b.ID != "toolu_1" {
		t.Errorf("ID = %q, want toolu_1", b.ID)
	}
	if b.Name != "send_message" {
		t.Errorf("Name = %q, want send_message", b.Name)
	}
	if string(b.Input) != `{"text":"hi"}` {
		t.Errorf("Input = %q, want %q", string(b.Input), `{"text":"hi"}`)
	}
}

func TestDecodeJSONLLine_toolResultFieldsCaptured(t *testing.T) {
	// tool_result blocks live inside user events and carry the result
	// claude sees for its own tool calls. Observability needs
	// tool_use_id (to pair) + content + is_error.
	line := []byte(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"Message sent.","is_error":false}]}}`)
	ev, err := decodeJSONLLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Parsed == nil || len(ev.Parsed.Content) != 1 {
		t.Fatalf("expected one parsed content block; got %+v", ev.Parsed)
	}
	b := ev.Parsed.Content[0]
	if b.ToolUseID != "toolu_1" {
		t.Errorf("ToolUseID = %q, want toolu_1", b.ToolUseID)
	}
	if string(b.Content) != `"Message sent."` {
		t.Errorf("Content = %q, want %q", string(b.Content), `"Message sent."`)
	}
	if b.IsError {
		t.Error("IsError = true, want false")
	}
}

func TestContentBlockOut_toolUseShape(t *testing.T) {
	b := contentBlock{
		Type:  "tool_use",
		ID:    "toolu_1",
		Name:  "send_message",
		Input: []byte(`{"text":"hi"}`),
	}
	out := contentBlockOut(b, true)
	if out["type"] != "tool_use" {
		t.Errorf("type = %v", out["type"])
	}
	if out["id"] != "toolu_1" {
		t.Errorf("id = %v", out["id"])
	}
	if out["name"] != "send_message" {
		t.Errorf("name = %v", out["name"])
	}
	// Input must be forwarded as raw JSON, not stringified.
	raw, ok := out["input"].(json.RawMessage)
	if !ok {
		t.Fatalf("input is %T, want json.RawMessage", out["input"])
	}
	if string(raw) != `{"text":"hi"}` {
		t.Errorf("input bytes = %q", string(raw))
	}
}

func TestContentBlockOut_toolUseFallsBackToEmptyObject(t *testing.T) {
	// When claude emits a tool_use with no input (rare but possible),
	// we still want the key present so consumers can rely on it.
	b := contentBlock{Type: "tool_use", ID: "toolu_1", Name: "t"}
	out := contentBlockOut(b, true)
	if _, ok := out["input"]; !ok {
		t.Error("input key missing")
	}
}

func TestHasToolResult(t *testing.T) {
	if hasToolResult([]contentBlock{{Type: "text"}}) {
		t.Error("text-only blocks should not register as tool_result")
	}
	if !hasToolResult([]contentBlock{{Type: "text"}, {Type: "tool_result"}}) {
		t.Error("mixed blocks containing tool_result should register")
	}
}

func TestTextFromContent_multipleBlocks(t *testing.T) {
	blocks := []contentBlock{
		{Type: "text", Text: "Part one. "},
		{Type: "tool_use", Text: ""}, // should be skipped
		{Type: "text", Text: "Part two."},
	}
	got := textFromContent(blocks)
	want := "Part one. Part two."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTextFromContent_trimsSurroundingWhitespace(t *testing.T) {
	blocks := []contentBlock{{Type: "text", Text: "  hello\n\n"}}
	got := textFromContent(blocks)
	if got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
}
