"use strict";

// Map this Node process's platform to the per-platform npm package that ships
// its prebuilt binary. Keep this table in step with ../../platforms.mjs (this
// copy stays plain CommonJS so the wrapper needs no build step).
const SUPPORTED = {
  "linux-x64": "@petersr/claude-p-linux-x64",
  "linux-arm64": "@petersr/claude-p-linux-arm64",
  "darwin-x64": "@petersr/claude-p-darwin-x64",
  "darwin-arm64": "@petersr/claude-p-darwin-arm64",
  "win32-x64": "@petersr/claude-p-win32-x64",
  "win32-arm64": "@petersr/claude-p-win32-arm64",
};

function platformKey() {
  return `${process.platform}-${process.arch}`;
}

// Resolve the absolute path to the bundled "claude-p" binary inside the
// matching platform package. Throws with an actionable message if the platform
// is unsupported or its package is absent.
function resolveBinary(name) {
  const key = platformKey();
  const pkg = SUPPORTED[key];
  if (!pkg) {
    throw new Error(
      `claude-p: unsupported platform "${key}". Supported: ${Object.keys(SUPPORTED).join(", ")}.\n` +
        `Build from source instead: https://github.com/PeterSR/claude-p`
    );
  }
  const ext = process.platform === "win32" ? ".exe" : "";
  try {
    return require.resolve(`${pkg}/bin/${name}${ext}`);
  } catch (err) {
    throw new Error(
      `claude-p: the platform package "${pkg}" is not installed.\n` +
        `It should install automatically as an optional dependency of "@petersr/claude-p".\n` +
        `If you used --no-optional or --omit=optional, reinstall without it, or run:\n` +
        `  npm i ${pkg}\n` +
        `Underlying error: ${err && err.message ? err.message : err}`
    );
  }
}

module.exports = { resolveBinary, platformKey, SUPPORTED };
