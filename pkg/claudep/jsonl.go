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
	Message json.RawMessage `json:"message,omitempty"`

	// Pre-decoded for the cases we care about.
	parsed *parsedMessage
}

type parsedMessage struct {
	ID         string          `json:"id,omitempty"`
	Model      string          `json:"model,omitempty"`
	StopReason string          `json:"stop_reason,omitempty"`
	Content    []contentBlock  `json:"content,omitempty"`
	Usage      json.RawMessage `json:"usage,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// tailEvent is what tailJSONL hands to its callback. raw is the full
// JSONL line (without the trailing newline) so callers can forward it
// verbatim if they wish.
type tailEvent struct {
	Raw      []byte
	Type     string
	Message  json.RawMessage
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
		// Read whatever's available.
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			full := append(partial, line...)
			if full[len(full)-1] != '\n' {
				// Partial line; remember the bytes and wait for more.
				partial = full
			} else {
				partial = nil
				cleaned := bytes_trimNewline(full)
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
			// EOF on a file we're tailing — sleep and retry.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
}

func bytes_trimNewline(b []byte) []byte {
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
	if (ev.Type == "assistant" || ev.Type == "user") && len(ev.Message) > 0 {
		var m parsedMessage
		if err := json.Unmarshal(ev.Message, &m); err == nil {
			ev.parsed = &m
			out.Text = textFromContent(m.Content)
			if ev.Type == "assistant" {
				_, nonTerminal := nonTerminalStopReasons[m.StopReason]
				out.Terminal = m.StopReason != "" && !nonTerminal
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
