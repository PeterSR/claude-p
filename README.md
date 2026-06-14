# claude-p — a Go drop-in for `claude -p`

> Use what you already paid for: `claude -p`-style automation on top of
> your interactive Claude Code subscription session.

[![CI](https://github.com/PeterSR/claude-p/actions/workflows/ci.yml/badge.svg)](https://github.com/PeterSR/claude-p/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/PeterSR/claude-p.svg)](https://pkg.go.dev/github.com/PeterSR/claude-p)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`claude-p` is a Go implementation of a `claude -p`-compatible CLI and
library, backed by the interactive Claude Code TUI.

It is built for the gap where interactive Claude Code works with your
subscription login, but `claude -p` / programmatic agent workflows are
limited, capped, or otherwise differently-billed in the environment
you're running in.

> **Inspired by [Equality-Machine/claude-p](https://github.com/Equality-Machine/claude-p)** (Python). 

## Why claude-p

Claude Code users increasingly rely on `claude -p` for scripts, agent
harnesses, local evals, and CI-style workflows. As of June 2026,
programmatic `claude -p` usage on Max plans is routed through a separate
Agent SDK credit pool (then "extra usage"), separate from the
interactive subscription limits the user already pays for.

`claude-p` bridges that gap. It exposes the **interactive** Claude Code
TUI through a `claude -p`-compatible CLI and Go library:

- no Anthropic API key required;
- no new account or subscription;
- same local Claude Code login state;
- familiar `claude -p` output formats: `text`, `json`, `stream-json`.

Under the hood, `claude-p` starts interactive `claude` in a pty, types
the prompt, watches Claude Code's own persisted JSONL transcript for
the canonical assistant text, and emits the answer in your chosen
format. The pty substrate comes from
[pupptyeer](https://github.com/PeterSR/pupptyeer): one-shot runs drive an
in-process pty (no extra binary), and `--pupptyeer-daemon` drives a
long-lived daemon so repeated calls continue the same conversation
without paying the TUI-startup cost each time (see
[Daemon mode](#daemon-mode-persistent-multi-turn)).

It also ships an **MCP bridge framework** so the same primitives that
power the CLI can be embedded in your own Go program — for example, an
orchestrator that drives an inner interactive `claude` via MCP tools.

## Install

### npm (prebuilt, resolves your OS/arch automatically)

```sh
npm i -g @petersr/claude-p
```

### Pre-built binaries

Download a release archive from the
[Releases page](https://github.com/PeterSR/claude-p/releases) and drop
the `claude-p` binary somewhere on your `PATH`. Linux, macOS, and
Windows builds are provided for both `amd64` and `arm64`.

### From source

```sh
go install github.com/PeterSR/claude-p/cmd/claude-p@latest
```

### Prerequisites

- `claude` (the official Claude Code CLI) installed and logged in.
  Run `claude` once interactively first and confirm you reach the
  welcome screen.

## CLI usage

```sh
# Same shape as `claude -p`:
claude-p "summarise CHANGELOG.md"

# Or via stdin:
echo "what does this script do?" | claude-p

# JSON envelope:
claude-p --output-format json "write a haiku about Go modules"

# Stream events as they happen:
claude-p --output-format stream-json "factor x^2 - 4"

# Forward arbitrary claude flags:
claude-p --model sonnet --append-system-prompt "Be terse." "..."
```

Run `claude-p --help` for the full list of forwarded flags. The
forwarded set tracks what interactive `claude` accepts; we currently
ship the flags listed by `claude --help` on Claude Code v2.1+.

### Output formats

| Format | What it writes |
|--------|----------------|
| `text` (default) | The final assistant text, nothing else. |
| `json` | A single JSON object on completion: `{"type":"result","subtype":"success","session_id":"…","duration_ms":…,"result":"…","is_error":…,"num_turns":null,"total_cost_usd":null,"usage":null}`. |
| `stream-json` | One JSON event per line as they arrive: a synthetic `system init`, claude's own `assistant` / `user` events from the persisted JSONL, then a final `result` event. |

Note: `total_cost_usd`, `num_turns`, and `usage` are emitted as `null`
because the interactive Claude Code TUI does not expose per-turn token
counts. The shape matches `claude -p` for tools that consume it; the
absence of accurate cost data is a property of the interactive backend.

### Daemon mode (persistent, multi-turn)

By default each `claude-p` call spawns a fresh in-process `claude`, runs
one prompt, and exits — no daemon, no extra binary. That pays the
TUI-startup cost every time and keeps no conversation state between
calls.

With `--pupptyeer-daemon`, claude-p instead drives `claude` inside a
[pupptyeer](https://github.com/PeterSR/pupptyeer) daemon. The `claude`
TUI stays alive between invocations, so a later call with the **same
`--session-id`** continues the same conversation — skipping startup
*and* keeping context:

```sh
# Boots a persistent claude keyed by this session id (in this cwd):
claude-p --pupptyeer-daemon --session-id 5b1d… "remember the number 7"

# Same id, same cwd → reattaches to the live session and continues:
claude-p --pupptyeer-daemon --session-id 5b1d… "what number did I ask you to remember?"
```

The session id you pass *is* the pupptyeer session id; the working
directory ties in because claude resumes a session in the directory it
was created in. If no live session holds the id but a transcript exists,
claude-p boots a fresh `claude --resume <id>` to reload the conversation.

Requirements for daemon mode: **pupptyeer >= 0.6.0** (earlier daemons lack
the caller-supplied session ids continuation relies on; claude-p detects an
older daemon and tells you to upgrade) at `$PUPPTYEER_BIN` or on `PATH`
(claude-p will auto-start a daemon if one isn't running) and a reachable
daemon socket. Override the socket with `--pupptyeer-socket`
and the binary with `--pupptyeer-bin`.

To get both binaries via npm (the batteries-included setup — claude-p
auto-starts the daemon once `pupptyeer` is on `PATH`):

```sh
npm i -g @petersr/claude-p @petersr/pupptyeer
```

## Library usage

### Quick query

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/PeterSR/claude-p/pkg/claudep"
)

func main() {
    res, err := claudep.Query(context.Background(), claudep.Options{
        Prompt:       "what is 6 * 7?",
        OutputFormat: claudep.FormatText,
        Stdout:       os.Stdout,
    })
    if err != nil {
        panic(err)
    }
    fmt.Fprintln(os.Stderr, "session:", res.SessionID, "took", res.DurationMs, "ms")
}
```

### Driving a pty session yourself

If you want to drive interactive `claude` for something other than the
`claude -p` use case — say, you want to send multiple prompts, observe
the TUI directly, or interleave keystrokes with tool calls — use
`pkg/claudepty` instead.

```go
import (
    "context"
    "fmt"
    "time"

    "github.com/PeterSR/claude-p/pkg/claudepty"
)

func main() {
    ctx := context.Background()
    sess, err := claudepty.LaunchClaude(ctx, claudepty.ClaudeLaunch{
        SessionID:      claudepty.NewSessionID(),
        PermissionMode: "acceptEdits",
    })
    if err != nil { panic(err) }
    defer sess.Close()

    if err := sess.WaitForReady(ctx, 20*time.Second); err != nil { panic(err) }
    _ = sess.SendPrompt("hello")
    _, _ = sess.SettleSnapshot(800*time.Millisecond, 30*time.Second)
    fmt.Println(sess.RenderGrid())
}
```

### MCP bridge: outer claude drives inner claude

`pkg/claudemcp` is a small framework for exposing arbitrary Go tools to
an interactive claude session over MCP. It is the substrate behind the
orchestrator pattern used by, e.g., [bloodhound](https://github.com/PeterSR/claude-code-bloodhound):
an outer interactive claude drives an inner interactive claude through
tools that read its rendered screen and send keystrokes.

The pattern:

1. Your process hosts an in-process `BridgeServer` on a unix socket,
   with whatever tools you want to expose registered against it.
2. You write an MCP config pointing at a small "relay" subcommand in
   your own binary — that subcommand calls `claudemcp/relay.Serve`.
3. You spawn interactive `claude` with `--mcp-config` pointing at the
   config you just wrote.
4. claude launches the relay; the relay dials your bridge socket; tool
   calls flow over the socket into your handlers.

```go
import (
    "encoding/json"

    "github.com/PeterSR/claude-p/pkg/claudemcp"
    "github.com/PeterSR/claude-p/pkg/claudemcp/relay"
    "github.com/PeterSR/claude-p/pkg/claudepty"
)

// In the host process:
bridge, _ := claudemcp.NewServer("")
defer bridge.Close()

// Built-in pty tools (read_pty + send_keys), bound to whichever
// ClaudeSession you want them to act on.
inner, _ := claudepty.LaunchClaude(ctx, claudepty.ClaudeLaunch{ /* ... */ })
bridge.AddTools(claudemcp.PtyTools(inner)...)

// Your own custom tools.
bridge.AddTool(claudemcp.NewTool(
    "my_tool",
    "Description shown to the LLM.",
    []claudemcp.Param{{Name: "arg", Type: "string", Required: true}},
    func(raw json.RawMessage) (any, error) { /* ... */ return nil, nil },
))

go bridge.Serve()

// In your binary's relay subcommand (Cobra/etc.):
//   relay.Serve(relay.Options{SocketPath: socketPath})
```

This pattern lets you keep tool implementations in your own process
(with access to your state, logs, etc.) while still letting an LLM call
them through claude's MCP support.

## FAQ

### Is this an API client / Anthropic SDK wrapper?

No. `claude-p` spawns the official `claude` CLI in interactive mode
and reads its persisted session JSONL. No HTTP calls are made directly
to Anthropic.

### Does it work with my Pro / Max / Team subscription?

Whatever interactive `claude` is logged into is what `claude-p` uses.
If `claude` works for you, `claude-p` does.

### Does it consume my Agent SDK credit?

No — that's the point. Tokens are billed against your interactive
subscription limits, the same way as normal Claude Code TUI usage.

### Why is `total_cost_usd` always null?

The interactive Claude Code TUI doesn't expose per-turn token counts in
its persisted JSONL the way `claude -p` does in its JSON envelope. We
emit `null` in those fields to keep the shape compatible without making
numbers up.

### Why is `stream-json` simpler than the Anthropic SDK protocol?

We emit Claude Code's own `assistant` / `user` / `result` envelope
events (the same shape `claude -p --output-format stream-json` uses for
its higher-level events), not the lower-level `message_start` /
`content_block_delta` chunks from the streaming API. The interactive
TUI doesn't persist token-level deltas; rebuilding them from the TUI
transcript would be lossy and slow. If you need true per-token
streaming, use the Anthropic API directly.

### Does it work on Windows?

It should — the pty layer uses [creack/pty](https://github.com/creack/pty),
which has ConPTY support on Windows. The persisted JSONL lookup uses
`%USERPROFILE%\.claude\projects`. Windows is built in CI and shipped as
a release archive, but the project is developed primarily on Linux; if
you hit Windows-specific issues, please file them.

### How is this different from the Python `claude-p`?

[Equality-Machine/claude-p](https://github.com/Equality-Machine/claude-p)
is a Python project with the same goal. This is an independent Go
implementation. There is no shared code; the two projects can be used
side by side or chosen based on your runtime preference. We also expose
a Go library (`pkg/claudepty`, `pkg/claudemcp`, `pkg/claudep`) for
programs that want to embed the behaviour rather than shell out.

### Can I use `--dangerously-skip-permissions`?

You can — the flag is forwarded — but you usually shouldn't. Combining
`--permission-mode acceptEdits` with an explicit `--allowedTools`
allow-list achieves the same auto-approval effect with much smaller
blast radius.

### What's the relationship to bloodhound?

[Bloodhound](https://github.com/PeterSR/claude-code-bloodhound) is the
project that pushed the need for this library. Its self-heal feature
needed to drive an outer interactive `claude` (so its tokens stayed on
the subscription) and have that outer claude drive an inner claude over
MCP. claude-p is the spinout of those primitives. Bloodhound imports
this library; this library has no bloodhound dependencies.

## License

[MIT](LICENSE). © Peter Severin Rasmussen.
