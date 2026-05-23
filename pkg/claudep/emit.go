package claudep

import (
	"encoding/json"
	"io"
	"strings"
	"time"
)

// apiKeySource matches Python claude-p's marker so consumers can tell
// at a glance "this came from the interactive TUI, not a real API key."
const apiKeySource = "interactive_tui_subscription"

// emitter buffers state across tail events and writes the chosen
// output format. One emitter per Query call.
type emitter struct {
	w         io.Writer
	format    OutputFormat
	sessionID string
	cwd       string
	startedAt time.Time

	// Aggregated across assistant turns.
	finalText        string
	finalStopReason  string
	terminalSeen     bool
	statusEmitted    bool
	rateLimitEmitted bool
	assistants       []*parsedMessage

	// JSONL path stored so the result envelope can echo it.
	jsonlPath string

	// Process exit code captured at finish time. Nil = unknown.
	exitCode *int
}

func newEmitter(w io.Writer, format OutputFormat, sessionID, cwd string) *emitter {
	return &emitter{
		w:         w,
		format:    format,
		sessionID: sessionID,
		cwd:       cwd,
		startedAt: time.Now(),
	}
}

// setJSONLPath is called once we know where the persisted JSONL lives.
func (e *emitter) setJSONLPath(p string) { e.jsonlPath = p }

// setExitCode lets the caller record the claude process exit status
// before finish() emits the result envelope.
func (e *emitter) setExitCode(code *int) { e.exitCode = code }

// init emits the synthetic "system init" + "system status" events for
// stream-json. No-op for text/json modes.
func (e *emitter) init() {
	if e.format != FormatStreamJSON {
		return
	}
	e.writeJSON(map[string]any{
		"type":                "system",
		"subtype":             "init",
		"cwd":                 e.cwd,
		"session_id":          e.sessionID,
		"tools":               []any{},
		"mcp_servers":         []any{},
		"model":               nil,
		"permissionMode":      "default",
		"apiKeySource":        apiKeySource,
		"claude_code_version": nil,
		"output_style":        "default",
		"uuid":                newEventUUID(),
		"fast_mode_state":     "off",
	})
}

// handle processes one tail event from the JSONL. Tracks state for the
// final envelope; for stream-json it also synthesizes the SDK-shaped
// stream_event chunks and forwards the high-level assistant/user line.
func (e *emitter) handle(ev tailEvent) {
	if ev.Type == "assistant" && ev.Parsed != nil {
		if ev.Text != "" {
			e.finalText = ev.Text
		}
		if ev.Terminal {
			e.terminalSeen = true
			e.finalStopReason = ev.Parsed.StopReason
		}
		e.assistants = append(e.assistants, ev.Parsed)
	}

	if e.format != FormatStreamJSON {
		return
	}

	switch ev.Type {
	case "assistant":
		// The first time we see an assistant message, emit the system
		// status placeholder Python emits between init and the first
		// real event.
		if !e.statusEmitted {
			e.writeJSON(map[string]any{
				"type":       "system",
				"subtype":    "status",
				"status":     "requesting",
				"uuid":       newEventUUID(),
				"session_id": e.sessionID,
			})
			e.statusEmitted = true
		}
		e.emitAssistantStreamEvents(ev)
	}
	// User events are intentionally NOT forwarded — matches Python
	// claude-p's stream-json (which only surfaces assistant turns via
	// the synthesized stream_event chunks). Forwarding them adds noise
	// and breaks shape parity for consumers that hard-code the Python
	// event vocabulary.
}

// emitAssistantStreamEvents synthesizes the SDK-shaped chunk sequence
// for one assistant message. Sequence mirrors Python claude-p:
//
//	stream_event message_start
//	stream_event content_block_start    (one per block)
//	stream_event content_block_delta    (one per text block — whole text)
//	{assistant wrapper}                  (Python emits this mid-sequence)
//	stream_event content_block_stop     (one per block)
//	stream_event message_delta
//	stream_event message_stop
func (e *emitter) emitAssistantStreamEvents(ev tailEvent) {
	m := ev.Parsed
	if m == nil {
		return
	}

	// message_start: full message envelope but with empty content and
	// no stop_reason yet (we're pretending we don't know either).
	startMsg := map[string]any{
		"model":         nonEmpty(m.Model, "claude-tui"),
		"id":            nonEmpty(m.ID, "msg_tui_"+newEventUUID()),
		"type":          "message",
		"role":          nonEmpty(m.Role, "assistant"),
		"content":       []any{},
		"stop_reason":   nil,
		"stop_sequence": nil,
		"stop_details":  nil,
		"usage":         startUsage(m.Usage),
	}
	e.emitStreamEvent(map[string]any{
		"type":    "message_start",
		"message": startMsg,
	}, map[string]any{"ttft_ms": nil})

	// content_block_start for each block (we always have at least one;
	// claude may emit tool_use or text blocks).
	for i, b := range m.Content {
		cb := map[string]any{
			"type": b.Type,
		}
		if b.Type == "text" {
			cb["text"] = ""
		}
		e.emitStreamEvent(map[string]any{
			"type":          "content_block_start",
			"index":         i,
			"content_block": cb,
		}, nil)
	}

	// content_block_delta for each text block, carrying the whole text
	// at once. Non-text blocks (tool_use) get a placeholder delta so
	// consumers see the index move.
	for i, b := range m.Content {
		var delta map[string]any
		if b.Type == "text" {
			delta = map[string]any{"type": "text_delta", "text": b.Text}
		} else {
			// Use input_json_delta with empty string. Python's behavior
			// here is similar — best-effort placeholder.
			delta = map[string]any{"type": "input_json_delta", "partial_json": ""}
		}
		e.emitStreamEvent(map[string]any{
			"type":  "content_block_delta",
			"index": i,
			"delta": delta,
		}, nil)
	}

	// Mid-sequence high-level wrapper. Python emits this between the
	// last content_block_delta and the first content_block_stop, with
	// a curated 5-key envelope (the claude JSONL line has ~13 fields
	// we don't want to leak).
	e.writeJSON(map[string]any{
		"type":               "assistant",
		"message":            assistantMessageOut(m),
		"parent_tool_use_id": nil,
		"session_id":         e.sessionID,
		"uuid":               newEventUUID(),
	})

	// content_block_stop for each block.
	for i := range m.Content {
		e.emitStreamEvent(map[string]any{
			"type":  "content_block_stop",
			"index": i,
		}, nil)
	}

	// message_delta with the stop info + usage. context_management goes
	// inside the .event object (not at the stream_event top level).
	e.emitStreamEvent(map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   nilIfEmpty(m.StopReason),
			"stop_sequence": nil,
			"stop_details":  nil,
		},
		"usage":              deltaUsage(m.Usage),
		"context_management": map[string]any{"applied_edits": []any{}},
	}, nil)

	// message_stop.
	e.emitStreamEvent(map[string]any{"type": "message_stop"}, nil)
}

// emitStreamEvent wraps an event payload in the standard stream_event
// envelope (session_id, uuid, parent_tool_use_id null) and writes it.
// extra, if non-nil, is merged at top level (used for ttft_ms /
// context_management which Python emits as siblings of "event").
func (e *emitter) emitStreamEvent(event map[string]any, extra map[string]any) {
	env := map[string]any{
		"type":               "stream_event",
		"event":              event,
		"session_id":         e.sessionID,
		"parent_tool_use_id": nil,
		"uuid":               newEventUUID(),
	}
	for k, v := range extra {
		env[k] = v
	}
	e.writeJSON(env)
}

// finish emits the final summary. text mode prints just the text; json
// + stream-json modes emit the full result envelope (and stream-json
// also emits a rate_limit_event before it).
func (e *emitter) finish() {
	switch e.format {
	case FormatText, "":
		if e.finalText != "" {
			_, _ = io.WriteString(e.w, e.finalText)
			_, _ = io.WriteString(e.w, "\n")
		}
		return
	case FormatStreamJSON:
		if !e.rateLimitEmitted {
			e.writeJSON(map[string]any{
				"type":            "rate_limit_event",
				"rate_limit_info": map[string]any{"status": "unknown"},
				"session_id":      e.sessionID,
				"uuid":            newEventUUID(),
			})
			e.rateLimitEmitted = true
		}
	}
	e.writeJSON(e.resultEnvelope())
}

// resultEnvelope builds the "type":"result" object. Two shapes:
//
//   - lean (FormatJSON, matches `claude -p --output-format json`): the
//     12-key envelope Python claude-p emits in plain JSON mode.
//   - rich (FormatStreamJSON, matches the result event Python emits as
//     the last line of a stream-json sequence): 18 keys including uuid,
//     stop_reason, api_error_status, fast_mode_state, modelUsage,
//     permission_denials.
//
// Both populate usage / num_turns from the persisted JSONL when
// available.
func (e *emitter) resultEnvelope() map[string]any {
	subtype := "success"
	terminalReason := "completed"
	if !e.terminalSeen {
		subtype = "error"
		terminalReason = "incomplete"
	}
	if e.exitCode != nil && *e.exitCode != 0 && *e.exitCode != 143 {
		// 143 == SIGTERM (clean shutdown signal from our own Exit()).
		// Other non-zero exits mean claude crashed or errored out.
		terminalReason = "exit_code_" + itoa(*e.exitCode)
	}

	tuiBackend := map[string]any{
		"raw_log":               nil,
		"session_jsonl":         e.jsonlPath,
		"tui_answer":            "",
		"final_answer_source":   "session_jsonl",
		"timed_out":             false,
		"exit_code":             e.exitCodeOrNil(),
		"extraction_confidence": "high",
		"compatibility_note":    "Shape-compatible with claude -p stream-json core events. usage/cost/tool counters are best-effort: populated from claude's persisted JSONL when present, else null.",
	}

	// Lean shape: the 12 keys Python's plain --output-format json emits.
	if e.format == FormatJSON {
		return map[string]any{
			"type":                    "result",
			"subtype":                 subtype,
			"is_error":                !e.terminalSeen,
			"duration_ms":             time.Since(e.startedAt).Milliseconds(),
			"duration_api_ms":         nil,
			"num_turns":               len(e.assistants),
			"result":                  e.finalText,
			"session_id":              e.sessionID,
			"total_cost_usd":          nil,
			"usage":                   e.aggregatedResultUsage(),
			"terminal_reason":         terminalReason,
			"interactive_tui_backend": tuiBackend,
		}
	}

	// Rich shape: stream-json's terminating result event.
	return map[string]any{
		"type":                    "result",
		"subtype":                 subtype,
		"is_error":                !e.terminalSeen,
		"api_error_status":        nil,
		"duration_ms":             time.Since(e.startedAt).Milliseconds(),
		"duration_api_ms":         nil,
		"num_turns":               len(e.assistants),
		"result":                  e.finalText,
		"stop_reason":             nilIfEmpty(e.finalStopReason),
		"session_id":              e.sessionID,
		"total_cost_usd":          nil,
		"usage":                   e.aggregatedResultUsage(),
		"modelUsage":              map[string]any{},
		"permission_denials":      []any{},
		"terminal_reason":         terminalReason,
		"fast_mode_state":         "off",
		"uuid":                    newEventUUID(),
		"interactive_tui_backend": tuiBackend,
	}
}

func (e *emitter) exitCodeOrNil() any {
	if e.exitCode == nil {
		return nil
	}
	return *e.exitCode
}

// aggregatedResultUsage sums per-assistant-message usage into one block
// shaped like Python claude-p's result.usage. We harvest real numbers
// from claude's JSONL when the message includes a usage block; missing
// values stay null.
func (e *emitter) aggregatedResultUsage() map[string]any {
	var (
		inputTokens, cacheCreate, cacheRead, outputTokens *int
		serviceTier                                       *string
		speed                                             *string
		serverTool                                        *serverToolUse
		cacheCreationBlock                                *cacheCreation
		iterations                                        []map[string]any
	)

	hadAnyUsage := false
	for _, m := range e.assistants {
		if m.Usage == nil {
			continue
		}
		hadAnyUsage = true
		u := m.Usage
		inputTokens = addPtr(inputTokens, u.InputTokens)
		cacheCreate = addPtr(cacheCreate, u.CacheCreationInputTokens)
		cacheRead = addPtr(cacheRead, u.CacheReadInputTokens)
		outputTokens = addPtr(outputTokens, u.OutputTokens)
		if u.ServiceTier != nil {
			serviceTier = u.ServiceTier
		}
		if u.Speed != nil {
			speed = u.Speed
		}
		if u.ServerToolUse != nil {
			if serverTool == nil {
				serverTool = &serverToolUse{}
			}
			serverTool.WebSearchRequests += u.ServerToolUse.WebSearchRequests
			serverTool.WebFetchRequests += u.ServerToolUse.WebFetchRequests
		}
		if u.CacheCreation != nil {
			if cacheCreationBlock == nil {
				cacheCreationBlock = &cacheCreation{}
			}
			cacheCreationBlock.Ephemeral1hInputTokens = addPtr(
				cacheCreationBlock.Ephemeral1hInputTokens, u.CacheCreation.Ephemeral1hInputTokens)
			cacheCreationBlock.Ephemeral5mInputTokens = addPtr(
				cacheCreationBlock.Ephemeral5mInputTokens, u.CacheCreation.Ephemeral5mInputTokens)
		}
		iterations = append(iterations, map[string]any{
			"input_tokens":                ptrToAny(u.InputTokens),
			"output_tokens":               ptrToAny(u.OutputTokens),
			"cache_read_input_tokens":     ptrToAny(u.CacheReadInputTokens),
			"cache_creation_input_tokens": ptrToAny(u.CacheCreationInputTokens),
			"cache_creation":              cacheCreationToMap(u.CacheCreation),
			"type":                        "message",
		})
	}

	// Word-count fallback if no usage data was present in JSONL (older
	// claude versions). Matches Python claude-p's behavior.
	if !hadAnyUsage && e.finalText != "" {
		words := len(strings.Fields(e.finalText))
		if words < 1 {
			words = 1
		}
		outputTokens = &words
		iterations = []map[string]any{{
			"input_tokens":                nil,
			"output_tokens":               words,
			"cache_read_input_tokens":     nil,
			"cache_creation_input_tokens": nil,
			"cache_creation": map[string]any{
				"ephemeral_5m_input_tokens": nil,
				"ephemeral_1h_input_tokens": nil,
			},
			"type": "message",
		}}
	}

	if serverTool == nil {
		serverTool = &serverToolUse{}
	}
	cacheCreationOut := map[string]any{
		"ephemeral_1h_input_tokens": nil,
		"ephemeral_5m_input_tokens": nil,
	}
	if cacheCreationBlock != nil {
		cacheCreationOut["ephemeral_1h_input_tokens"] = ptrToAny(cacheCreationBlock.Ephemeral1hInputTokens)
		cacheCreationOut["ephemeral_5m_input_tokens"] = ptrToAny(cacheCreationBlock.Ephemeral5mInputTokens)
	}

	out := map[string]any{
		"input_tokens":                ptrToAny(inputTokens),
		"cache_creation_input_tokens": ptrToAny(cacheCreate),
		"cache_read_input_tokens":     ptrToAny(cacheRead),
		"output_tokens":               ptrToAny(outputTokens),
		"server_tool_use": map[string]any{
			"web_search_requests": serverTool.WebSearchRequests,
			"web_fetch_requests":  serverTool.WebFetchRequests,
		},
		"service_tier":   strPtrToAny(serviceTier),
		"cache_creation": cacheCreationOut,
		"iterations":     iterations,
		"speed":          strPtrToAny(speed),
	}
	if iterations == nil {
		out["iterations"] = []any{}
	}
	return out
}

// startUsage shapes the usage block for the message_start event.
// Always null-shaped — Python doesn't claim to know tokens at start.
func startUsage(u *messageUsage) map[string]any {
	return map[string]any{
		"input_tokens":                nil,
		"cache_creation_input_tokens": nil,
		"cache_read_input_tokens":     nil,
		"output_tokens":               nil,
		"service_tier":                nil,
	}
}

// deltaUsage shapes the usage block for the message_delta event.
// Mirrors Python: includes per-message iterations of size 1.
func deltaUsage(u *messageUsage) map[string]any {
	if u == nil {
		return map[string]any{
			"input_tokens":                nil,
			"cache_creation_input_tokens": nil,
			"cache_read_input_tokens":     nil,
			"output_tokens":               nil,
			"iterations": []any{
				map[string]any{
					"input_tokens":                nil,
					"output_tokens":               nil,
					"cache_read_input_tokens":     nil,
					"cache_creation_input_tokens": nil,
					"cache_creation": map[string]any{
						"ephemeral_5m_input_tokens": nil,
						"ephemeral_1h_input_tokens": nil,
					},
					"type": "message",
				},
			},
		}
	}
	return map[string]any{
		"input_tokens":                ptrToAny(u.InputTokens),
		"cache_creation_input_tokens": ptrToAny(u.CacheCreationInputTokens),
		"cache_read_input_tokens":     ptrToAny(u.CacheReadInputTokens),
		"output_tokens":               ptrToAny(u.OutputTokens),
		"iterations": []any{
			map[string]any{
				"input_tokens":                ptrToAny(u.InputTokens),
				"output_tokens":               ptrToAny(u.OutputTokens),
				"cache_read_input_tokens":     ptrToAny(u.CacheReadInputTokens),
				"cache_creation_input_tokens": ptrToAny(u.CacheCreationInputTokens),
				"cache_creation":              cacheCreationToMap(u.CacheCreation),
				"type":                        "message",
			},
		},
	}
}

// assistantMessageOut produces the curated assistant.message payload
// for stream-json's "assistant" event. Python emits a subset of the
// raw JSONL assistant message: model, id, type, role, content,
// stop_reason, stop_sequence, stop_details, usage, context_management.
func assistantMessageOut(m *parsedMessage) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	contentOut := make([]any, 0, len(m.Content))
	for _, b := range m.Content {
		blk := map[string]any{"type": b.Type}
		if b.Type == "text" {
			blk["text"] = b.Text
		}
		contentOut = append(contentOut, blk)
	}
	out := map[string]any{
		"model":         nonEmpty(m.Model, "claude-tui"),
		"id":            nonEmpty(m.ID, "msg_tui_"+newEventUUID()),
		"type":          "message",
		"role":          nonEmpty(m.Role, "assistant"),
		"content":       contentOut,
		"stop_reason":   nilIfEmpty(m.StopReason),
		"stop_sequence": nil,
		"stop_details":  nil,
		"usage":         startUsage(m.Usage),
		"context_management": nil,
	}
	if m.Usage != nil {
		// Replace the null-shaped startUsage with real numbers when
		// claude included them.
		out["usage"] = map[string]any{
			"input_tokens":                ptrToAny(m.Usage.InputTokens),
			"cache_creation_input_tokens": ptrToAny(m.Usage.CacheCreationInputTokens),
			"cache_read_input_tokens":     ptrToAny(m.Usage.CacheReadInputTokens),
			"output_tokens":               ptrToAny(m.Usage.OutputTokens),
			"service_tier":                strPtrToAny(m.Usage.ServiceTier),
		}
	}
	return out
}

func cacheCreationToMap(c *cacheCreation) map[string]any {
	if c == nil {
		return map[string]any{
			"ephemeral_5m_input_tokens": nil,
			"ephemeral_1h_input_tokens": nil,
		}
	}
	return map[string]any{
		"ephemeral_5m_input_tokens": ptrToAny(c.Ephemeral5mInputTokens),
		"ephemeral_1h_input_tokens": ptrToAny(c.Ephemeral1hInputTokens),
	}
}

func (e *emitter) writeJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = e.w.Write(append(b, '\n'))
}

func nonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func ptrToAny(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func strPtrToAny(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func addPtr(a, b *int) *int {
	if a == nil && b == nil {
		return nil
	}
	v := 0
	if a != nil {
		v += *a
	}
	if b != nil {
		v += *b
	}
	return &v
}

func itoa(i int) string {
	// Avoid pulling strconv just for this; the values we format here
	// are small process exit codes.
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [11]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
