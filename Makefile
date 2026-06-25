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

.PHONY: build install vet test ci clean

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
	rm -f bin/claude-p
