# iPhone Tailnet Bridge

An open-source macOS menu-bar app that keeps a paired iPhone visible to Xcode
when the Mac and iPhone are on different Wi-Fi networks connected by Tailscale.

The bridge captures Apple's real CoreDevice Bonjour records while the phone is
local, republishes them on the Mac when the phone is remote, and relays the
associated TCP and UDP traffic over the tailnet. Xcode, `devicectl`, and normal
project tooling continue to use Apple's own pairing and debugging stack.

> [!IMPORTANT]
> This is an Xcode/CoreDevice bridge, not a general Bonjour, AirDrop, AirPlay,
> Handoff, or iPhone Mirroring bridge.

## What changed from the original prototype

- Native menu-bar app with device setup, live state, start/stop, and diagnostics.
- Automatic Bonjour capture—no hand-copying `authTag` or TXT records.
- Automatic matching against online iOS devices from `tailscale status --json`.
- One compiled relay process instead of roughly 950 `socat` processes.
- Adaptive discovery for the dynamic `apple-mobdev2` port exposed on Tailscale.
- Broader, configurable CoreDevice tunnel coverage in one event-driven process.
- Automatic recovery after Mac IP, Wi-Fi, or tailnet changes.
- No `/etc/hosts` modification and no root requirement.
- LAN clients are rejected by default; only connections originating on the Mac
  can use the local proxy.
- Owner-only profiles, atomic writes, a per-user LaunchAgent, status reporting,
  migration from `bridge.local.conf`, tests, race checks, and CI.

```text
Xcode / devicectl
       │
       │ local Bonjour + local-only relay listeners
       ▼
iPhone Tailnet Bridge (one process)
       │
       │ authenticated Tailscale path
       ▼
paired iPhone CoreDevice services
```

## Requirements

- macOS 13 or later and Xcode command-line tools.
- An iPhone or iPad paired with this Mac once through Xcode/USB.
- Developer Mode and wireless development enabled on the iOS device.
- Tailscale connected to the same tailnet on both devices.
- The iOS device on Wi-Fi. It may use a different Wi-Fi network from the Mac;
  cold-starting Apple's wireless-debugging listener on cellular is unsupported.

No Homebrew package and no `socat` installation are required.

## Install the app from source

```bash
git clone https://github.com/ahmadtawakol/iphone-tailnet-bridge.git
cd iphone-tailnet-bridge
bash scripts/install.sh
```

The installer builds, signs locally, and opens:

```text
~/Applications/iPhone Tailnet Bridge.app
```

In the app:

1. Keep the paired iPhone unlocked on the same Wi-Fi as the Mac.
2. Select **Add iPhone**.
3. Match the discovered iPhone to its Tailscale device.
4. Select **Add Device**.

The app captures the real service identity, stores it in an owner-only profile,
installs the per-user background bridge, and leaves normal local Xcode discovery
alone. When the iPhone moves to another Wi-Fi, the bridge activates automatically.

## Command-line use

Build the helper:

```bash
go build -o .build/iphone-tailnet-bridge ./cmd/iphone-tailnet-bridge
```

Useful commands:

```bash
.build/iphone-tailnet-bridge discover
.build/iphone-tailnet-bridge peers
.build/iphone-tailnet-bridge setup
.build/iphone-tailnet-bridge doctor
.build/iphone-tailnet-bridge install
.build/iphone-tailnet-bridge status
.build/iphone-tailnet-bridge scan
```

`setup` must run while the phone is local. `doctor` verifies the profile, LAN
interface, tailnet peer, listener count, and RemotePairing reachability without
altering device trust.

Profiles and status live under:

```text
~/Library/Application Support/iPhone Tailnet Bridge/
```

Logs live under:

```text
~/Library/Logs/iPhone Tailnet Bridge/
```

## Migrate the original shell configuration

The old format is supported without sourcing the file as shell code:

```bash
.build/iphone-tailnet-bridge import-legacy --config bridge.local.conf
.build/iphone-tailnet-bridge install
```

The importer writes a private JSON profile. After verifying the new bridge,
remove the old LaunchAgent and tagged hosts entry with `./teardown.sh`.

## Verification

The bridge reports build, discovery, transport, and Apple-device availability
as separate layers. A healthy transport is not by itself proof that Xcode has
finished preparing the device.

```bash
xcrun devicectl list devices
```

When remote, the paired phone should appear as `available`. Then use Xcode or
the project's normal commands, for example:

```bash
xcrun devicectl device install app --device '<device-id>' '/absolute/path/App.app'
xcrun devicectl device process launch --device '<device-id>' 'com.example.app'
```

For automated work, adapt [CHATGPT-PROMPT.md](CHATGPT-PROMPT.md).

## Network and security boundary

- The bridge never sends traffic to a project-operated server. Traffic stays
  between devices on the mesh VPN.
- Captured TXT data is written with mode `0600` and is never logged by the
  bridge. Treat profiles as pairing material.
- Listener sockets bind to the Mac's current LAN IPv4 address because Apple's
  Bonjour/CoreDevice stack ignores point-to-point VPN interfaces. By default,
  the relay immediately rejects clients whose source address is not the Mac
  itself.
- Tailscale ACLs should permit only the development Mac to reach the iPhone.
- The helper uses Apple's `dns-sd` process for compatible proxy registration;
  TXT values may therefore be visible to another process running as the same
  local user while a bridge is active.

See [SECURITY.md](SECURITY.md) before sharing diagnostics.

## Known limits

- Apple does not document remote CoreDevice bridging as a supported product
  surface. A future iOS, macOS, or Xcode release may change service records,
  port behavior, or tunnel negotiation.
- Initial capture requires the phone and Mac on the same LAN.
- The phone must remain on Wi-Fi for cold connection establishment.
- Only one device profile can be active at once because Apple reuses the same
  local CoreDevice port space.
- The default tunnel range favors current observed Xcode/iOS behavior. Advanced
  users can narrow or extend `portRanges` in the private profile; run `doctor`
  after changing it.

## Development

```bash
bash scripts/test.sh
```

The test suite includes config and parser tests, TCP/UDP round trips, race
detection, a native Swift build, and app-bundle signature verification.
Opt-in integration checks can also exercise system Bonjour and a real iPhone:

```bash
TAILBRIDGE_INTEGRATION=1 \
TAILBRIDGE_TARGET_IP='<iPhone tailnet IPv4>' \
go test -count=1 -v ./internal/bonjour ./internal/bridge ./internal/relay
```

## License

MIT. See [LICENSE](LICENSE).
