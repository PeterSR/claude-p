#!/bin/sh
# claude-p's entrypoint: a thin wrapper over pupptyeer's own helper
# (pupptyeer-entrypoint, shipped beside it), which starts the daemon, waits for
# its socket and execs the command. This adds only claude-p's defaults. Run
# under tini (-g). The helper is called by its real path because it finds its
# sibling binaries (and puts them on PATH) via $0.
set -eu

BIN_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
export DISABLE_AUTOUPDATER="${DISABLE_AUTOUPDATER:-1}"

if [ "$#" -eq 0 ]; then
  set -- claude-p --help
fi
exec "$BIN_DIR/pupptyeer-entrypoint" "$@"
