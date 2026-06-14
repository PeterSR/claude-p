# @petersr/claude-p

A `claude -p` drop-in backed by your interactive Claude Code subscription, not
the Agent SDK credit path. This package installs a prebuilt `claude-p` binary
for your platform.

```sh
npm i -g @petersr/claude-p

claude-p "summarise CHANGELOG.md"
echo "what does this script do?" | claude-p
claude-p --output-format json "write a haiku about Go modules"
```

`claude-p` drives interactive Claude Code in a pty (via
[pupptyeer](https://github.com/PeterSR/pupptyeer)) and reads the answer from
claude's own persisted transcript. It needs the official `claude` CLI installed
and logged in.

For the full CLI surface, daemon mode (`--pupptyeer-daemon`, persistent
multi-turn sessions), the Go library, and the MCP bridge, see the
[project README](https://github.com/PeterSR/claude-p#readme).

## How it resolves the binary

The matching `@petersr/claude-p-<os>-<arch>` package is pulled in as an optional
dependency and the launcher execs its bundled binary. Prefer the raw binary?
Grab one from [GitHub Releases](https://github.com/PeterSR/claude-p/releases).

MIT licensed.
