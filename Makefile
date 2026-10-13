BIN     := bin/graft
PREFIX  ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test vet install

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/graft

test:
	go test -race ./...

vet:
	go vet ./...

install: build
	mkdir -p $(PREFIX)/bin
	install -m 755 $(BIN) $(PREFIX)/bin/graft
