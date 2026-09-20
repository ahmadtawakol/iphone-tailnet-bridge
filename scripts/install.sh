#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_SOURCE="$ROOT/.build/iPhone Tailnet Bridge.app"
APP_DEST="$HOME/Applications/iPhone Tailnet Bridge.app"

cd "$ROOT"
bash scripts/build-app.sh
mkdir -p "$HOME/Applications"
rm -rf "$APP_DEST"
/usr/bin/ditto --noextattr --noqtn "$APP_SOURCE" "$APP_DEST"
/usr/bin/xattr -cr "$APP_DEST"
/usr/bin/codesign --verify --deep --strict "$APP_DEST"

if [[ -f "$ROOT/bridge.local.conf" ]]; then
    "$APP_DEST/Contents/Resources/iphone-tailnet-bridge" import-legacy --config "$ROOT/bridge.local.conf"
    "$APP_DEST/Contents/Resources/iphone-tailnet-bridge" install
fi

open "$APP_DEST"
echo "Installed $APP_DEST"
