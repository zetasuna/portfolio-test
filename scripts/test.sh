#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
command -v go >/dev/null || { echo "Go is required" >&2; exit 1; }
if [[ -n "$(gofmt -l cmd internal tests)" ]]; then
  gofmt -l cmd internal tests
  echo "Go files need gofmt" >&2
  exit 1
fi
go vet ./...
go test ./... -race -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -n 1
