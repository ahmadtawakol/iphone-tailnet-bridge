#!/bin/bash
# Update only this project's tagged /etc/hosts entry.
set -euo pipefail
[[ $EUID == 0 ]] || { echo 'Run this helper with sudo.' >&2; exit 1; }
MODE="${1:-}"
[[ "$MODE" == set || "$MODE" == remove ]] || exit 2
if [[ "$MODE" == set ]]; then
    [[ $# == 3 && "$2" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ && "$3" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*\.local$ ]] || exit 2
fi
LOCK=/private/etc/.iphone-tailnet-bridge-hosts-lock
mkdir "$LOCK" || { echo 'Hosts update already in progress.' >&2; exit 1; }
TEMP=""
trap '[[ -z "$TEMP" ]] || rm -f "$TEMP"; rmdir "$LOCK"' EXIT
TEMP="$(mktemp /private/etc/.iphone-tailnet-bridge.XXXXXX)"
/usr/bin/awk '!($3 == "#" && $4 == "iphone-tailnet-bridge")' /etc/hosts > "$TEMP"
if [[ "$MODE" == set ]]; then
    printf '%s\t%s\t# iphone-tailnet-bridge\n' "$2" "$3" >> "$TEMP"
fi
chown root:wheel "$TEMP"
chmod 644 "$TEMP"
mv -f "$TEMP" /private/etc/hosts
TEMP=""
/usr/bin/dscacheutil -flushcache
echo "Bridge hosts mapping: $MODE"
