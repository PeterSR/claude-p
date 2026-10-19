# Version metadata baked into the binary at link time. VERSION mirrors what
# goreleaser injects for releases (a tag, or the last tag + commits + -dirty,
# or a bare short SHA, or "dev" outside a git repo); COMMIT/DATE fill in the
# rest of the `claude-p --version` line. Override any of them on the command
# line, e.g. `make build VERSION=v1.2.3`.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

CMD := ./cmd/claude-p

.PHONY: build install vet test ci clean docker

# Build the binary into ./bin (gitignored) with version baked in.
build:
	go build -ldflags "$(LDFLAGS)" -o bin/claude-p $(CMD)

# Install onto PATH (GOBIN, else $GOPATH/bin) with version baked in.
install:
	go install -ldflags "$(LDFLAGS)" $(CMD)

vet:
	go vet ./...

test:
	go test ./...

ci: vet test

clean:
	rm -rf bin/claude-p bin/docker-ctx

# Local image, same Dockerfile as the release. Builds the linux binary for the
# host arch (override DOCKER_ARCH) into a context laid out like goreleaser's
# dockers_v2 context (<os>/<arch>/claude-p, docker/entrypoint.sh), then tags only
# claude-p:local (never the ghcr name: a local tag would shadow the published
# image for any FROM or COPY --from naming it). Pin overrides (a pupptyeer
# version needs its archive's sha256 for DOCKER_ARCH too):
#   make docker DOCKER_PUPPTYEER_VERSION=x.y.z DOCKER_PUPPTYEER_SHA256=<sha256> DOCKER_CLAUDE_CODE_VERSION=x.y.z
DOCKER_ARCH ?= $(shell go env GOARCH)
DOCKER_CTX  := bin/docker-ctx
DOCKER_VER  := $(patsubst v%,%,$(VERSION))
DOCKER_ARGS := $(if $(DOCKER_PUPPTYEER_VERSION),--build-arg PUPPTYEER_VERSION=$(DOCKER_PUPPTYEER_VERSION)) \
	$(if $(DOCKER_PUPPTYEER_SHA256),--build-arg PUPPTYEER_SHA256_$(DOCKER_ARCH)=$(DOCKER_PUPPTYEER_SHA256)) \
	$(if $(DOCKER_CLAUDE_CODE_VERSION),--build-arg CLAUDE_CODE_VERSION=$(DOCKER_CLAUDE_CODE_VERSION))

docker:
	rm -rf $(DOCKER_CTX)
	mkdir -p $(DOCKER_CTX)/linux/$(DOCKER_ARCH) $(DOCKER_CTX)/docker
	CGO_ENABLED=0 GOOS=linux GOARCH=$(DOCKER_ARCH) go build -ldflags "$(LDFLAGS)" -o $(DOCKER_CTX)/linux/$(DOCKER_ARCH)/claude-p $(CMD)
	cp docker/entrypoint.sh $(DOCKER_CTX)/docker/entrypoint.sh
	docker build --platform linux/$(DOCKER_ARCH) -f docker/Dockerfile \
		--build-arg CLAUDE_P_VERSION=$(DOCKER_VER) --build-arg REVISION=$(COMMIT) $(DOCKER_ARGS) \
		-t claude-p:local $(DOCKER_CTX)
