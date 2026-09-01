#!/bin/zsh
set -euo pipefail

go run ./cmd/api &
api_pid=$!
go run ./cmd/worker &
worker_pid=$!

cleanup() {
  kill "$api_pid" "$worker_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
wait "$api_pid" "$worker_pid"
