#!/bin/sh
# Start the pupptyeer daemon in the background, wait for its socket, then exec
# the command. Run under tini (-g) so the daemon and its pty children die with
# the container. Self-contained: finds its sibling binaries by its own location.
set -eu

BIN_DIR="$(cd "$(dirname "$0")" && pwd)"
# claude-p asks the daemon to start a bare `claude`, so it must resolve through
# PATH both here and in the daemon, also on a base that did not set PATH.
PATH="$BIN_DIR:$PATH"
export PATH
export DISABLE_AUTOUPDATER="${DISABLE_AUTOUPDATER:-1}"

SOCK="${PUPPTYEER_SOCK:-/tmp/pupptyeer/daemon.sock}"
export PUPPTYEER_SOCK="$SOCK"
SOCK_DIR="$(dirname "$SOCK")"
# Only a directory made here is made private: chmod on an existing one (/tmp,
# say) would be wrong or fail.
if [ ! -d "$SOCK_DIR" ]; then
  mkdir -p -m 0700 "$SOCK_DIR"
fi
rm -f "$SOCK"

# Its banner goes to stderr so it never pollutes the command's stdout.
"$BIN_DIR/pupptyeer" daemon >&2 &
daemon=$!

# Up to 10 seconds.
i=0
while [ ! -S "$SOCK" ]; do
  if ! kill -0 "$daemon" 2>/dev/null; then
    echo "entrypoint: pupptyeer daemon exited before $SOCK appeared" >&2
    exit 1
  fi
  i=$((i + 1))
  if [ "$i" -gt 100 ]; then
    echo "entrypoint: timed out waiting for $SOCK" >&2
    exit 1
  fi
  sleep 0.1
done

if [ "$#" -eq 0 ]; then
  set -- claude-p --help
fi
exec "$@"
