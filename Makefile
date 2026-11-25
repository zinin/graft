BIN     := bin/graft
PREFIX  ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
# The pages that README.md links to: those in git, since docs/ may hold other files.
DOCS    := $(shell git ls-files ':(glob)docs/*.md' 2>/dev/null)

.PHONY: build test test-js ui-smoke vet install dist

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/graft

test:
	go test -race ./...

# test-js tests the logic of the graft ui page; it needs Node.
test-js:
	node --test internal/ui/jstest/draft.test.mjs

# ui-smoke drives graft ui in headless Chrome; it needs Chrome and Node 22
# or later. CHROME names the browser when it is not google-chrome.
ui-smoke: build
	node internal/ui/jstest/smoke.mjs $(BIN)

vet:
	go vet ./...

install: build
	mkdir -p $(PREFIX)/bin
	install -m 755 $(BIN) $(PREFIX)/bin/graft

# dist builds a release archive for each of TARGETS, and their SHA256SUMS, in dist/.
dist:
	rm -rf dist
	mkdir dist
	set -e; for target in $(TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; dir=dist/graft_$(VERSION:v%=%)_$${os}_$${arch}; \
		mkdir -p $$dir/docs; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $$dir/graft ./cmd/graft; \
		cp LICENSE README.md $$dir/; \
		$(if $(DOCS),cp $(DOCS) $$dir/docs/;) \
		tar -czf $$dir.tar.gz -C $$dir graft LICENSE README.md docs; \
		rm -r $$dir; \
	done
	cd dist && sha256sum *.tar.gz > SHA256SUMS
