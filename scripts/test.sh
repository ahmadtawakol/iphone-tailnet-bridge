#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

gofmt_output="$(gofmt -l cmd internal)"
if [[ -n "$gofmt_output" ]]; then
    echo "Go files need formatting:"
    echo "$gofmt_output"
    exit 1
fi

go vet ./...
go test ./...
go test -race ./...
swift build -c debug --product Tailbridge
bash scripts/build-app.sh >/dev/null
/usr/bin/codesign --verify --deep --strict ".build/iPhone Tailnet Bridge.app"
/usr/bin/plutil -lint ".build/iPhone Tailnet Bridge.app/Contents/Info.plist"

if command -v shellcheck >/dev/null 2>&1; then
    shellcheck bridge.sh install-agent.sh teardown.sh hosts.sh scripts/*.sh
fi

git diff --check
echo "All checks passed."
