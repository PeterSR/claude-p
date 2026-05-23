package claudep

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// emitter buffers state across tail events and writes the chosen
// output format. One emitter per Query call.
type emitter struct {
	w           io.Writer
	format      OutputFormat
	sessionID   string
	model       string
	startedAt   time.Time
	finalText   string
	finalUsage  json.RawMessage
	terminalSeen bool
}

func newEmitter(w io.Writer, format OutputFormat, sessionID string) *emitter {
	return &emitter{
		w:         w,
		format:    format,
		sessionID: sessionID,
		startedAt: time.Now(),
	}
}

// init emits the synthetic "system init" event for stream-json mode.
// No-op for text/json modes.
func (e *emitter) init() {
	if e.format != FormatStreamJSON {
		return
	}
	e.writeJSON(map[string]any{
		"type":       "system",
		"subtype":    "init",
		"session_id": e.sessionID,
	})
}

// handle processes one tail event from the JSONL. Always tracks final
// text + usage; for stream-json it also forwards events as they arrive.
func (e *emitter) handle(ev tailEvent) {
	if ev.Type == "assistant" && len(ev.Message) > 0 {
		// Track potential final text. Updated each time so the latest
		// assistant message wins.
		if ev.Text != "" {
			e.finalText = ev.Text
		}
		if ev.Terminal {
			e.terminalSeen = true
		}
	}

	if e.format != FormatStreamJSON {
		return
	}

	// stream-json: forward assistant + user + tool events. Drop the
	// noisier internal types (file-history-snapshot, last-prompt, etc.)
	// since the Anthropic CLI's stream-json doesn't surface them.
	switch ev.Type {
	case "assistant", "user":
		// Forward the line verbatim — claude already shaped it as the
		// right event envelope for us.
		_, _ = e.w.Write(append(ev.Raw, '\n'))
	}
}

// finish emits the final summary line (json mode) or result event
// (stream-json mode). text mode prints just the final text.
func (e *emitter) finish() {
	switch e.format {
	case FormatText, "":
		if e.finalText != "" {
			fmt.Fprintln(e.w, e.finalText)
		}
	case FormatJSON:
		out := map[string]any{
			"type":        "result",
			"subtype":     "success",
			"session_id":  e.sessionID,
			"duration_ms": time.Since(e.startedAt).Milliseconds(),
			"result":      e.finalText,
			"is_error":    !e.terminalSeen,
			// Cost / token usage are not reliably available from the
			// interactive TUI. Mirror Python claude-p and leave them
			// nil / zero so the shape matches `claude -p`.
			"num_turns":      nil,
			"total_cost_usd": nil,
			"usage":          nil,
		}
		e.writeJSON(out)
	case FormatStreamJSON:
		e.writeJSON(map[string]any{
			"type":           "result",
			"subtype":        "success",
			"session_id":     e.sessionID,
			"duration_ms":    time.Since(e.startedAt).Milliseconds(),
			"result":         e.finalText,
			"is_error":       !e.terminalSeen,
			"num_turns":      nil,
			"total_cost_usd": nil,
			"usage":          nil,
		})
	}
}

func (e *emitter) writeJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = e.w.Write(append(b, '\n'))
}
