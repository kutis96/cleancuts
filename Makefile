GO ?= go

.PHONY: all ci build test vet fmt fmtcheck tidy demo clean

## all: run the full portable check (the default target).
all: ci

## ci: the single source of truth for "is the repo healthy" — gofmt, vet, test,
## and build — via scripts/ci.sh, so local and GitHub CI run the exact same thing.
ci:
	sh scripts/ci.sh

## build: compile the whole module.
build:
	$(GO) build ./...

## test: run the full test suite.
test:
	$(GO) test ./...

## vet: static analysis.
vet:
	$(GO) vet ./...

## fmt: format all Go source in place.
fmt:
	gofmt -w .

## fmtcheck: fail if any Go file is not gofmt-clean (used by CI).
fmtcheck:
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then echo "gofmt needed on:"; echo "$$files"; exit 1; \
	else echo "gofmt: clean"; fi

## tidy: sync go.mod/go.sum.
tidy:
	$(GO) mod tidy

## demo: generate a sample 2D profile and print generic G-code.
demo:
	$(GO) run ./cmd/cleancuts demo

## clean: clear the Go build cache (and remove build artifacts).
clean:
	$(GO) clean -cache 2>/dev/null || true
