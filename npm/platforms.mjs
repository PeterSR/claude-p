// Single source of the GOOS/GOARCH <-> npm os/cpu matrix for the
// `@petersr/claude-p` binary wrapper. Mirrors the build matrix in
// ../.goreleaser.yaml (linux/darwin/windows x amd64/arm64). Consumed by
// build.mjs; the runtime launcher (claude-p/lib/resolve.cjs) keeps its own
// inlined copy because it must be plain CommonJS with no build step.
//
// Everything is scoped under @petersr/ to match the pupptyeer convention and
// because npm is picky about unscoped platform names.

export const SCOPE = "@petersr";
export const META_PKG = "@petersr/claude-p";

export const PLATFORMS = [
  { goos: "linux", goarch: "amd64", dir: "claude-p-linux-x64", pkg: "@petersr/claude-p-linux-x64", os: "linux", cpu: "x64", ext: "" },
  { goos: "linux", goarch: "arm64", dir: "claude-p-linux-arm64", pkg: "@petersr/claude-p-linux-arm64", os: "linux", cpu: "arm64", ext: "" },
  { goos: "darwin", goarch: "amd64", dir: "claude-p-darwin-x64", pkg: "@petersr/claude-p-darwin-x64", os: "darwin", cpu: "x64", ext: "" },
  { goos: "darwin", goarch: "arm64", dir: "claude-p-darwin-arm64", pkg: "@petersr/claude-p-darwin-arm64", os: "darwin", cpu: "arm64", ext: "" },
  { goos: "windows", goarch: "amd64", dir: "claude-p-win32-x64", pkg: "@petersr/claude-p-win32-x64", os: "win32", cpu: "x64", ext: ".exe" },
  { goos: "windows", goarch: "arm64", dir: "claude-p-win32-arm64", pkg: "@petersr/claude-p-win32-arm64", os: "win32", cpu: "arm64", ext: ".exe" },
];
