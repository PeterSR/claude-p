package claudep

import "testing"

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
