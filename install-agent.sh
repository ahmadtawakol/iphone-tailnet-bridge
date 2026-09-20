#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/.build/iphone-tailnet-bridge"

cd "$ROOT"
go test ./...
go build -trimpath -ldflags "-s -w -X main.version=1.0.0" -o "$BIN" ./cmd/iphone-tailnet-bridge

if [[ -f "$ROOT/bridge.local.conf" ]]; then
    "$BIN" import-legacy --config "$ROOT/bridge.local.conf"
fi

"$BIN" install
