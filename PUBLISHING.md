# Publishing

claude-p ships two public artifacts, versioned in lockstep with the `vX.Y.Z`
git tag:

| Artifact | Registry | Source | Auth (steady state) |
| --- | --- | --- | --- |
| GitHub Release (binaries) | GitHub | goreleaser | `GITHUB_TOKEN` (built in) |
| `@petersr/claude-p` (+ 6 platform pkgs) | npm | `npm/` | npm OIDC trusted publishing |

Both are driven by pushing a `v*` tag through `.github/workflows/release.yml`.
The npm job is gated behind a repo variable so the workflow is safe to land
before the packages exist on npm:

- `PUBLISH_NPM=1` enables the npm publish job.

(Set it under **Settings -> Secrets and variables -> Actions -> Variables**.)

## First release (one-time npm bootstrap)

npm has no pending-publisher equivalent: trusted publishing must be enabled on a
package that already exists, so each npm package's **first** publish is done by
hand from a logged-in machine.

```sh
npm login        # or: npm whoami to confirm you're already logged in

# Build the per-platform packages (needs Go), then publish the 6 platform
# packages BEFORE the meta package that depends on them.
node npm/build.mjs 0.1.0          # use the real version, no leading 'v'
for d in npm/claude-p-*/; do ( cd "$d" && npm publish --access public ); done
( cd npm/claude-p && npm publish --access public )
```

Then enable OIDC for each package so CI handles every later release:

1. For `@petersr/claude-p` and each `@petersr/claude-p-<os>-<arch>`: on
   npmjs.com open the package **Settings -> Trusted publishing -> GitHub
   Actions**, owner `PeterSR`, repo `claude-p`, workflow `release.yml`.
2. Set repo variable `PUBLISH_NPM=1`.

(Prefer a token over OIDC? Skip the steps above, add an `NPM_TOKEN` granular
automation secret, and set `NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}` on the
`publish-npm` job instead. OIDC is recommended: no long-lived secret, and
`--provenance` works out of the box.)

## Every release after that

1. Bump the version in `npm/claude-p/package.json` (its `version` and
   `optionalDependencies` are also rewritten by `npm/build.mjs` at publish time,
   but keep the committed copy in sync). The Go binary's version is tag-driven
   via the `-X main.version` ldflag and needs no manual bump.
2. Commit, then `git tag vX.Y.Z && git push origin main --tags`.
3. The `release` workflow publishes the GitHub binaries and (if `PUBLISH_NPM=1`)
   the npm packages.

## Local validation (no upload)

```sh
# Build all platform packages, then exercise the launcher. Install the
# host-platform package locally so require.resolve can find it (a real
# `npm i -g @petersr/claude-p` gets it via optionalDependencies).
node npm/build.mjs 0.1.0
cd npm/claude-p && npm i ../claude-p-linux-x64 --no-save   # match your host os/arch
node bin/claude-p.cjs --version                            # resolves + execs the binary
```
