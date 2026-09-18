#!/bin/bash
# Proxy Apple's paired-iPhone Bonjour/CoreDevice services over Tailscale.
# The script advertises the captured Bonjour identity on the Mac's local
# interface and forwards required TCP/UDP ports to the phone's Tailscale IP.

set -euo pipefail
export PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin

DIR="$(cd "$(dirname "$0")" && pwd)"
CONFIG_FILE="${BRIDGE_CONFIG_FILE:-$DIR/bridge.local.conf}"
STATE="$HOME/Library/Application Support/iphone-tailnet-bridge"
die() { echo "bridge: $*" >&2; exit 1; }

[[ -f "$CONFIG_FILE" ]] || die "Create $CONFIG_FILE from bridge.local.conf.example first."
# shellcheck disable=SC1090
source "$CONFIG_FILE"

en0_ip() { /usr/sbin/ipconfig getifaddr "$INTERFACE" 2>/dev/null || true; }

validate() {
    local value
    for value in "$IPHONE_TAILNET_IP" "$IPHONE_HOSTNAME" "$INSTANCE_UUID" "$IDENTIFIER" "$AUTH_TAG" "$VER" "$MIN_VER" "$FLAGS"; do
        [[ -n "$value" && "$value" != *REPLACE_ME* ]] || die 'Complete bridge.local.conf first.'
    done
    [[ "$IPHONE_TAILNET_IP" =~ ^100\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || die 'Use the phone Tailscale IPv4 address.'
    [[ "$IPHONE_HOSTNAME" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*\.local$ ]] || die 'Use a .local hostname without spaces or trailing dot.'
    [[ "$PAIRING_PORT" =~ ^[0-9]+$ ]] && (( PAIRING_PORT > 0 && PAIRING_PORT < 65536 )) || die 'Invalid pairing port.'
    (( PORT_FIRST > 0 && PORT_LAST >= PORT_FIRST && PORT_LAST < 65536 )) || die 'Invalid primary port range.'
    (( EXTRA_PORT_FIRST > 0 && EXTRA_PORT_LAST >= EXTRA_PORT_FIRST && EXTRA_PORT_LAST < 65536 )) || die 'Invalid extra port range.'
    (( TUNNEL_PORT_FIRST > 0 && TUNNEL_PORT_LAST >= TUNNEL_PORT_FIRST && TUNNEL_PORT_LAST < 65536 )) || die 'Invalid tunnel port range.'
}

validate
IP="$(en0_ip)"
[[ -n "$IP" ]] || die "$INTERFACE has no IPv4 address."

if [[ "${1:-}" == --prepare-hosts ]]; then
    exec /usr/bin/sudo /bin/bash "$DIR/hosts.sh" set "$IP" "$IPHONE_HOSTNAME"
fi
[[ $# == 0 ]] || die 'Usage: bridge.sh [--prepare-hosts]'
[[ $EUID != 0 ]] || die 'Run as your login user, not root.'

SOCAT="$(command -v socat)" || die 'Install socat: brew install socat'
/usr/bin/awk -v ip="$IP" -v host="$IPHONE_HOSTNAME" '$1 == ip && $2 == host && $3 == "#" && $4 == "iphone-tailnet-bridge" {found=1} END {exit !found}' /etc/hosts || die 'Run ./bridge.sh --prepare-hosts first.'

umask 077
mkdir -p "$STATE"
mkdir "$STATE/lock" 2>/dev/null || die "Already running, or stale lock: $STATE/lock"
echo $$ > "$STATE/lock/pid"
pids=()

set -m
cleanup() {
    trap - EXIT TERM INT HUP
    local pid
    for pid in "${pids[@]}"; do kill -TERM -- "-$pid" 2>/dev/null || true; done
    sleep 1
    for pid in "${pids[@]}"; do kill -KILL -- "-$pid" 2>/dev/null || true; done
    wait 2>/dev/null || true
    rm -f "$STATE/lock/pid"
    rmdir "$STATE/lock" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 1' TERM INT HUP

start() { "$@" & pids+=("$!"); }

forward_port() {
    local port="$1"
    start "$SOCAT" "TCP4-LISTEN:$port,bind=$IP,reuseaddr,fork" "TCP4:$IPHONE_TAILNET_IP:$port,connect-timeout=10"
    start "$SOCAT" -T 120 "UDP4-LISTEN:$port,bind=$IP,reuseaddr,fork" "UDP4:$IPHONE_TAILNET_IP:$port"
}

forward_port 49152
if (( PAIRING_PORT != 49152 && (PAIRING_PORT < PORT_FIRST || PAIRING_PORT > PORT_LAST) )); then
    forward_port "$PAIRING_PORT"
fi
for ((port=PORT_FIRST; port<=PORT_LAST; port++)); do forward_port "$port"; done
for ((port=EXTRA_PORT_FIRST; port<=EXTRA_PORT_LAST; port++)); do forward_port "$port"; done
for ((port=TUNNEL_PORT_FIRST; port<=TUNNEL_PORT_LAST; port++)); do forward_port "$port"; done

sleep 2
for pid in "${pids[@]}"; do
    kill -0 "$pid" 2>/dev/null || die 'A listener failed; inspect logs for conflicts or resource limits.'
done

txt=("identifier=$IDENTIFIER" "authTag=$AUTH_TAG" "ver=$VER" "minVer=$MIN_VER" "flags=$FLAGS")
for service in _remotepairing._tcp _remoted._tcp _apple-mobdev2._tcp; do
    start /usr/bin/dns-sd -i "$INTERFACE" -P "$INSTANCE_UUID" "$service" local. "$PAIRING_PORT" "$IPHONE_HOSTNAME." "$IP" "${txt[@]}"
done

echo "bridge: forwarding Mac $IP to iPhone $IPHONE_TAILNET_IP"
while sleep 10; do
    [[ "$(en0_ip)" == "$IP" ]] || die 'Mac interface address changed; refresh hosts and restart.'
    for pid in "${pids[@]}"; do
        kill -0 "$pid" 2>/dev/null || die "Child $pid exited; restarting all listeners and registrations."
    done
done
