# claude-p base image: claude-p + pupptyeer + Claude Code CLI, all pinned.
#
# pupptyeer (with its entrypoint helper and a static tini) is taken from its own
# published image, pinned by tag; Claude Code is downloaded and signature-checked.
# The claude-p binary is NOT downloaded here. goreleaser (and `make docker`)
# put the freshly built one in the build context at <os>/<arch>/claude-p,
# because claude-p release assets are drafts while the image builds. The
# context also holds docker/entrypoint.sh, at its repo path.
#
# Everything the image ships lives in /opt/claude-p/bin as real, self-contained
# files, so a consumer on another base can `COPY --from=` that directory.
#
# Needs BuildKit (COPY --chmod), which buildx and current `docker build` use.

ARG DEBIAN_TAG=bookworm-slim
# The pupptyeer pin: the tag of ghcr.io/petersr/pupptyeer and its multi-arch
# index digest, so a re-pushed tag cannot change what goes in (Docker resolves
# by the digest when both are given). Bump both together; the digest is shown
# by `docker buildx imagetools inspect ghcr.io/petersr/pupptyeer:<version>`.
# An empty digest pulls by tag alone.
ARG PUPPTYEER_VERSION=0.10.3
ARG PUPPTYEER_DIGEST=sha256:8c24ba8eb5a3456706557598d5ac89615d69706e24752acdb33fb1513d43b755

FROM ghcr.io/petersr/pupptyeer:${PUPPTYEER_VERSION}${PUPPTYEER_DIGEST:+@${PUPPTYEER_DIGEST}} AS pupptyeer

# ---- fetch: download and verify Claude Code ------------------
# Runs on the build host's own platform (no emulation) and fetches the files
# for TARGETARCH, so a multi-arch build only emulates the small final stage.
FROM --platform=$BUILDPLATFORM debian:${DEBIAN_TAG} AS fetch
ARG TARGETARCH
ARG CLAUDE_CODE_VERSION=2.1.303
# Anthropic's Claude Code release signing key, as published in the Claude Code
# setup docs ("Verify binary integrity"). Each release's manifest.json is
# signed with it, and the manifest holds every binary's sha256.
ARG CLAUDE_CODE_GPG_FINGERPRINT=31DDDE24DDFAB679F42D7BD2BAA929FF1A7ECACE
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gpg jq \
 && rm -rf /var/lib/apt/lists/*
RUN set -eu; \
    mkdir -p /out /work; cd /work; \
    case "${TARGETARCH}" in \
      amd64) cc=x64 ;; \
      arm64) cc=arm64 ;; \
      *) echo "unsupported arch ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    # Claude Code: the exact version's binary from the official download host.
    # The manifest's signature is checked against the pinned key fingerprint,
    # then the binary against the manifest. Same artifact the native installer
    # fetches, minus its "always latest" bootstrap and home-dir symlinks.
    cbase="https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}"; \
    curl -fsSL --proto '=https' -o key.asc https://downloads.claude.ai/keys/claude-code.asc; \
    curl -fsSL --proto '=https' -o manifest.json "${cbase}/manifest.json"; \
    curl -fsSL --proto '=https' -o manifest.json.sig "${cbase}/manifest.json.sig"; \
    export GNUPGHOME=/work/gnupg; mkdir -m 0700 "${GNUPGHOME}"; \
    fpr="$(gpg --batch --with-colons --show-keys key.asc | awk -F: '$1 == "fpr" { print $10; exit }')"; \
    if [ "${fpr}" != "${CLAUDE_CODE_GPG_FINGERPRINT}" ]; then \
      echo "Claude Code signing key is '${fpr}', expected ${CLAUDE_CODE_GPG_FINGERPRINT}" >&2; exit 1; \
    fi; \
    gpg --batch --dearmor -o key.gpg key.asc; \
    gpgv --keyring /work/key.gpg --status-fd 1 manifest.json.sig manifest.json > gpg.status; \
    grep -q "^\[GNUPG:\] VALIDSIG .* ${CLAUDE_CODE_GPG_FINGERPRINT}\$" gpg.status; \
    [ "$(jq -r .version manifest.json)" = "${CLAUDE_CODE_VERSION}" ]; \
    sum="$(jq -r ".platforms[\"linux-${cc}\"].checksum // empty" manifest.json)"; \
    echo "${sum}" | grep -Eq '^[0-9a-f]{64}$'; \
    curl -fsSL --proto '=https' -o /out/claude "${cbase}/linux-${cc}/claude"; \
    echo "${sum}  /out/claude" | sha256sum -c -; \
    chmod 0755 /out/*; \
    cd /; rm -rf /work

# ---- final ------------------------------------------------------------------
FROM debian:${DEBIAN_TAG}
ARG TARGETPLATFORM
ARG PUPPTYEER_VERSION
ARG CLAUDE_CODE_VERSION=2.1.303
ARG CLAUDE_P_VERSION=dev
ARG REVISION=unknown
ARG CLAUDE_UID=1000
ARG CLAUDE_GID=1000

LABEL org.opencontainers.image.title="claude-p" \
      org.opencontainers.image.description="claude-p with pupptyeer and Claude Code CLI, pinned" \
      org.opencontainers.image.source="https://github.com/PeterSR/claude-p" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${CLAUDE_P_VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      dev.petersr.claude-p.version="${CLAUDE_P_VERSION}" \
      dev.petersr.pupptyeer.version="${PUPPTYEER_VERSION}" \
      dev.petersr.claude-code.version="${CLAUDE_CODE_VERSION}"

# ca-certificates: claude talks TLS. tini is pupptyeer's static build, copied
# below into /opt/claude-p/bin, so it runs on any base. git/ripgrep-free on purpose;
# add what your workload needs in a derived image.
# HOME is world-writable and sticky (like /tmp), so `docker run --user <uid>`
# with no passwd entry can still write ~/.claude.json and friends there.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid "${CLAUDE_GID}" claude \
 && useradd --uid "${CLAUDE_UID}" --gid "${CLAUDE_GID}" --create-home --home-dir /home/claude --shell /bin/bash claude \
 && chmod 1777 /home/claude \
 && mkdir -p /opt/claude-p/bin \
 && ln -s /opt/claude-p/bin/claude-p /usr/local/bin/claude-p \
 && ln -s /opt/claude-p/bin/pupptyeer /usr/local/bin/pupptyeer \
 && ln -s /opt/claude-p/bin/claude /usr/local/bin/claude

# --chmod on the COPY itself: a later `RUN chmod` would duplicate the ~285MB
# claude binary into a second layer.
COPY --from=pupptyeer --chown=0:0 --chmod=0755 /opt/pupptyeer/bin/ /opt/claude-p/bin/
COPY --from=fetch --chown=0:0 --chmod=0755 /out/ /opt/claude-p/bin/
COPY --chown=0:0 --chmod=0755 ${TARGETPLATFORM}/claude-p /opt/claude-p/bin/claude-p
# goreleaser keeps extra_files at their repo path, hence docker/ here.
COPY --chown=0:0 --chmod=0755 docker/entrypoint.sh /opt/claude-p/bin/entrypoint.sh

ENV HOME=/home/claude \
    PATH=/opt/claude-p/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    DISABLE_AUTOUPDATER=1 \
    PUPPTYEER_SOCK=/tmp/pupptyeer/daemon.sock

USER claude
WORKDIR /home/claude
ENTRYPOINT ["/opt/claude-p/bin/tini", "-g", "--", "/opt/claude-p/bin/entrypoint.sh"]
CMD ["claude-p", "--help"]
