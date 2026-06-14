"use strict";

const { spawnSync } = require("node:child_process");
const { resolveBinary } = require("./resolve.cjs");

// Resolve the named bundled binary, exec it with this process's args, and
// propagate its exit status (or terminating signal). Used by the bin shim.
function launch(name) {
  let bin;
  try {
    bin = resolveBinary(name);
  } catch (err) {
    console.error(err.message);
    process.exit(1);
  }

  const result = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });

  if (result.error) {
    console.error(`claude-p: failed to launch ${bin}: ${result.error.message}`);
    process.exit(1);
  }
  if (result.signal) {
    // Re-raise the signal so callers see the child's termination cause.
    process.kill(process.pid, result.signal);
    return;
  }
  process.exit(result.status === null ? 1 : result.status);
}

module.exports = { launch };
