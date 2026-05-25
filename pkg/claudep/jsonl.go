package claudep

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// nonTerminalStopReasons are stop_reasons that mean "claude is still
// working on this turn." Anything else (or empty) we treat as final.
// Mirrors Python claude-p's NON_TERMINAL_STOP_REASONS.
var nonTerminalStopReasons = map[string]struct{}{
	"tool_use":   {},
	"pause_turn": {},
}

// jsonlEvent is the union of fields we care about across the events
// claude writes to its persisted JSONL. We use json.RawMessage for the
// pieces we don't introspect so we can forward them verbatim.
type jsonlEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
}

// parsedMessage carries the assistant/user message fields we want to
// echo into the stream-json events and aggregate into the result
// envelope. Pointer fields are nil-able so we can preserve "unknown"
// (emit JSON null) versus "zero" (emit 0) in the output.
type parsedMessage struct {
	ID           string          `json:"id,omitempty"`
	Model        string          `json:"model,omitempty"`
	Role         string          `json:"role,omitempty"`
	StopReason   string          `json:"stop_reason,omitempty"`
	StopSequence json.RawMessage `json:"stop_sequence,omitempty"`
	StopDetails  json.RawMessage `json:"stop_details,omitempty"`
	Content      []contentBlock  `json:"content,omitempty"`
	Usage        *messageUsage   `json:"usage,omitempty"`
}

// contentBlock is the union shape across the block types claude emits:
//   - text: Type + Text
//   - thinking: Type only (Text is set to the thinking string but we
//     filter that out in textFromContent so it doesn't leak into final
//     answers)
//   - tool_use: Type + ID + Name + Input
//   - tool_result: Type + ToolUseID + Content + IsError
//
// Input and Content are kept as json.RawMessage so we can forward them
// verbatim without committing to a schema for tool-specific payloads.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`

	// tool_use fields.
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result fields.
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// messageUsage matches the per-message usage block claude writes inside
// each assistant message. Pointer fields preserve "unknown" (null)
// versus "zero present" (0).
type messageUsage struct {
	InputTokens              *int           `json:"input_tokens"`
	CacheCreationInputTokens *int           `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int           `json:"cache_read_input_tokens"`
	OutputTokens             *int           `json:"output_tokens"`
	ServerToolUse            *serverToolUse `json:"server_tool_use,omitempty"`
	ServiceTier              *string        `json:"service_tier"`
	CacheCreation            *cacheCreation `json:"cache_creation,omitempty"`
	Speed                    *string        `json:"speed,omitempty"`
}

type serverToolUse struct {
	WebSearchRequests int `json:"web_search_requests"`
	WebFetchRequests  int `json:"web_fetch_requests"`
}

type cacheCreation struct {
	Ephemeral1hInputTokens *int `json:"ephemeral_1h_input_tokens"`
	Ephemeral5mInputTokens *int `json:"ephemeral_5m_input_tokens"`
}

// tailEvent is what tailJSONL hands to its callback. raw is the full
// JSONL line (without the trailing newline) so callers can forward it
// verbatim if they wish.
type tailEvent struct {
	Raw      []byte
	Type     string
	Message  json.RawMessage
	Parsed   *parsedMessage
	Terminal bool
	Text     string
}

// tailJSONL polls path as it grows, decoding each new line into a
// tailEvent and calling cb. Returns when ctx fires, the callback
// returns done=true, or the file disappears.
//
// Polling cadence is 100ms — fine for interactive sessions where new
// content arrives at human-conversation rates, not microseconds.
func tailJSONL(ctx context.Context, path string, cb func(tailEvent) (done bool, err error)) error {
	// Wait for the file to appear in the first place.
	deadline := time.Now().Add(15 * time.Second)
	var f *os.File
	var err error
	for time.Now().Before(deadline) {
		f, err = os.Open(path)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if f == nil {
		return errors.New("jsonl: file never appeared")
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var partial []byte

	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			full := append(partial, line...)
			if full[len(full)-1] != '\n' {
				partial = full
			} else {
				partial = nil
				cleaned := bytesTrimNewline(full)
				ev, derr := decodeJSONLLine(cleaned)
				if derr == nil {
					done, cerr := cb(ev)
					if cerr != nil {
						return cerr
					}
					if done {
						return nil
					}
				}
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

func bytesTrimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func decodeJSONLLine(line []byte) (tailEvent, error) {
	var ev jsonlEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return tailEvent{Raw: line}, err
	}
	out := tailEvent{
		Raw:     line,
		Type:    ev.Type,
		Message: ev.Message,
	}
	// claude emits one `system.subtype=turn_duration` event at the end
	// of every turn (after the Stop hook fires, regardless of whether a
	// final text message was produced). This is our most reliable
	// "this turn is over" signal — terminal assistant text isn't always
	// emitted when the model is satisfied with just a tool call.
	if ev.Type == "system" && ev.Subtype == "turn_duration" {
		out.Terminal = true
		return out, nil
	}
	if (ev.Type == "assistant" || ev.Type == "user") && len(ev.Message) > 0 {
		var m parsedMessage
		if err := json.Unmarshal(ev.Message, &m); err == nil {
			out.Parsed = &m
			out.Text = textFromContent(m.Content)
			if ev.Type == "assistant" {
				_, nonTerminal := nonTerminalStopReasons[m.StopReason]
				// Terminal requires both a terminal stop_reason AND at
				// least one user-visible text block. Extended thinking
				// emits intermediate messages with stop_reason=end_turn
				// but only a "thinking" block — those are NOT the final
				// answer; the real text response comes in the next
				// assistant message.
				hasText := false
				for _, b := range m.Content {
					if b.Type == "text" {
						hasText = true
						break
					}
				}
				out.Terminal = m.StopReason != "" && !nonTerminal && hasText
			}
		}
	}
	return out, nil
}

func textFromContent(blocks []contentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type == "text" {
			b.WriteString(blk.Text)
		}
	}
	return strings.TrimSpace(b.String())
}
