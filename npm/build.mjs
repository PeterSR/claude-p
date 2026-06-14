// Build the per-platform npm packages for the `claude-p` binary wrapper.
//
//   node npm/build.mjs <version>
//
// For each platform in platforms.mjs this cross-compiles ./cmd/claude-p with
// the same flags as .goreleaser.yaml, drops the binary into npm/<pkg>/bin/, and
// writes npm/<pkg>/package.json pinned to <version>. It also rewrites
// npm/claude-p/package.json so its own version and every optionalDependencies
// entry equal <version>.
//
// The generated npm/claude-p-*/ dirs are release artifacts (gitignored); CI
// runs this on a tag, then `npm publish`es each platform package followed by
// the meta package. Run it locally to smoke-test the wrapper.

import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { PLATFORMS } from "./platforms.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..");

const version = process.argv[2];
if (!version || /^v/.test(version)) {
  console.error("usage: node npm/build.mjs <version>   (semver, no leading 'v')");
  process.exit(1);
}

const ldflags = `-s -w -X main.version=${version}`;

function goBuild({ goos, goarch, outFile }) {
  mkdirSync(dirname(outFile), { recursive: true });
  execFileSync("go", ["build", "-trimpath", "-ldflags", ldflags, "-o", outFile, "./cmd/claude-p"], {
    cwd: repoRoot,
    stdio: "inherit",
    env: { ...process.env, GOOS: goos, GOARCH: goarch, CGO_ENABLED: "0" },
  });
}

function writeJson(file, obj) {
  writeFileSync(file, JSON.stringify(obj, null, 2) + "\n");
}

for (const p of PLATFORMS) {
  const pkgDir = join(here, p.dir);
  const binDir = join(pkgDir, "bin");
  console.log(`==> ${p.pkg} (${p.goos}/${p.goarch})`);
  rmSync(pkgDir, { recursive: true, force: true });
  mkdirSync(binDir, { recursive: true });

  goBuild({ goos: p.goos, goarch: p.goarch, outFile: join(binDir, `claude-p${p.ext}`) });

  writeJson(join(pkgDir, "package.json"), {
    name: p.pkg,
    version,
    description: `Prebuilt claude-p binary for ${p.os} ${p.cpu}. Installed automatically by the "@petersr/claude-p" package.`,
    repository: {
      type: "git",
      url: "git+https://github.com/PeterSR/claude-p.git",
    },
    homepage: "https://github.com/PeterSR/claude-p#readme",
    license: "MIT",
    author: "PeterSR",
    os: [p.os],
    cpu: [p.cpu],
    files: ["bin"],
    publishConfig: { access: "public" },
  });
}

// Sync the meta package: its own version and all optionalDependencies pins.
const metaFile = join(here, "claude-p", "package.json");
const meta = JSON.parse(readFileSync(metaFile, "utf8"));
meta.version = version;
meta.optionalDependencies = Object.fromEntries(PLATFORMS.map((p) => [p.pkg, version]));
writeJson(metaFile, meta);

console.log(`\nBuilt ${PLATFORMS.length} platform packages + meta at version ${version}.`);
