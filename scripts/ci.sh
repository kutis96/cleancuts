#!/usr/bin/env sh
# CleanCuts CI — the single, portable, locally-runnable check.
#
# This is the source of truth for "is the repo healthy." The GitHub Actions
# workflow and the Makefile both invoke it, so what runs on CI is exactly what
# runs on your machine — no hidden CI-only steps, and no Make dependency.
#
# Requires: Go 1.22+.
set -eu

# Run from the repository root, regardless of where this is invoked from.
cd "$(dirname "$0")/.."

echo "==> gofmt (check)"
files=$(gofmt -l .)
if [ -n "$files" ]; then
  echo "    gofmt needed on:" >&2
  echo "$files" >&2
  exit 1
fi
echo "    gofmt: clean"

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./...

echo "==> go build"
go build ./...

echo "==> OK: gofmt, vet, test, build all pass"
