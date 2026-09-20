#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/.build/iphone-tailnet-bridge"

if [[ "${1:-}" == "--prepare-hosts" ]]; then
    echo "The compiled bridge no longer modifies /etc/hosts." >&2
    exit 0
fi

if [[ ! -x "$BIN" ]]; then
    cd "$ROOT"
    go build -o "$BIN" ./cmd/iphone-tailnet-bridge
fi

exec "$BIN" run "$@"
