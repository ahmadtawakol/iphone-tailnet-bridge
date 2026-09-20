#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONFIGURATION="${CONFIGURATION:-release}"
APP="$ROOT/.build/iPhone Tailnet Bridge.app"
STAGE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/iphone-tailnet-bridge.XXXXXX")"
STAGE_APP="$STAGE_ROOT/iPhone Tailnet Bridge.app"
CONTENTS="$STAGE_APP/Contents"

cleanup() {
    /bin/rm -rf "$STAGE_ROOT"
}
trap cleanup EXIT

cd "$ROOT"
go test ./...
go build -trimpath -ldflags "-s -w -X main.version=1.0.0" -o "$ROOT/.build/iphone-tailnet-bridge" ./cmd/iphone-tailnet-bridge
swift build -c "$CONFIGURATION" --product Tailbridge

mkdir -p "$CONTENTS/MacOS" "$CONTENTS/Resources"
/usr/bin/ditto --noextattr --noqtn "$ROOT/.build/$CONFIGURATION/Tailbridge" "$CONTENTS/MacOS/Tailbridge"
/usr/bin/ditto --noextattr --noqtn "$ROOT/.build/iphone-tailnet-bridge" "$CONTENTS/Resources/iphone-tailnet-bridge"
/usr/bin/ditto --noextattr --noqtn "$ROOT/macapp/Resources/Info.plist" "$CONTENTS/Info.plist"
chmod 755 "$CONTENTS/MacOS/Tailbridge" "$CONTENTS/Resources/iphone-tailnet-bridge"
/usr/bin/plutil -lint "$CONTENTS/Info.plist"
/usr/bin/xattr -cr "$STAGE_APP"

SIGNING_IDENTITY="${CODESIGN_IDENTITY:--}"
/usr/bin/codesign --force --sign "$SIGNING_IDENTITY" \
    --identifier com.github.ahmadtawakol.iPhoneTailnetBridge.helper \
    "$CONTENTS/Resources/iphone-tailnet-bridge"
/usr/bin/codesign --force --sign "$SIGNING_IDENTITY" --options runtime "$STAGE_APP"
/usr/bin/codesign --verify --deep --strict "$STAGE_APP"

/bin/rm -rf "$APP"
/usr/bin/ditto --noextattr --noqtn "$STAGE_APP" "$APP"
/usr/bin/xattr -cr "$APP"
echo "$APP"
