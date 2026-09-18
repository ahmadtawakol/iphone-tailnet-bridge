# iPhone CoreDevice bridge over Tailscale

Make a paired iPhone appear local to a remote Mac by proxying Apple's Bonjour
and CoreDevice traffic through a private Tailscale network.

The bridge does not build apps itself. It lets the Mac's normal Apple tooling
(`xcodebuild`, `devicectl`, and related services) reach the iPhone without a
USB cable or a shared physical Wi-Fi network.

```text
iPhone CoreDevice services
        │
        │ Tailscale address
        ▼
Mac socat TCP/UDP relays
        │
        │ local Bonjour advertisement on en0
        ▼
Apple CoreDevice on the Mac sees a paired iPhone
```

## Important security boundary

This project requires device-specific pairing material. Never publish or share
your `bridge.local.conf`, especially `authTag`. Do not expose the forwarded
ports to the public internet. Use Tailscale ACLs and only pair devices you
trust.

The repository contains only templates and code. Every user must capture the
values for their own paired iPhone.

## Requirements

- A Mac with Xcode and its command-line tools installed.
- A paired iPhone with Developer Mode enabled.
- Tailscale installed and connected on both devices, in the same tailnet.
- Homebrew's `socat`: `brew install socat`.
- The iPhone associated with Wi-Fi. Tailscale can provide the cross-network
  path, but pure-cellular CoreDevice installation is not assumed to work.

## One-time pairing and capture

Pair the iPhone to the Mac using USB first. Trust the Mac, enable Developer
Mode if prompted, allow device preparation to finish, and enable the wireless
connection in Xcode when offered.

With the iPhone on Wi-Fi and Tailscale connected, capture its Bonjour record:

```bash
dns-sd -B _remotepairing._tcp local.
dns-sd -L "<the-instance-name>" _remotepairing._tcp local.
tailscale ip -4
```

Record the instance name, hostname, port, and TXT values:
`identifier`, `authTag`, `ver`, `minVer`, and `flags`. Keep the pairing data
private.

## Configure and install

```bash
cp bridge.local.conf.example bridge.local.conf
```

Edit `bridge.local.conf` with the values captured from your own iPhone. Then:

```bash
./install-agent.sh
```

The installer adds one tagged `/etc/hosts` entry, installs a per-user
LaunchAgent, and starts the bridge as the logged-in user. Logs are written to
`~/Library/Logs/iphone-tailnet-bridge/`.

Verify the connection:

```bash
xcrun devicectl list devices
```

The device should appear as paired and available. Build and install using the
normal project workflow or directly with Apple's tools:

```bash
xcrun devicectl device install app --device '<device-id>' '/absolute/path/App.app'
xcrun devicectl device process launch --device '<device-id>' 'com.example.app'
```

## Troubleshooting

- If the Mac's `en0` address changes, run `./bridge.sh --prepare-hosts` and
  restart the LaunchAgent.
- If CoreDevice requests a port outside the configured ranges, add only a
  bounded range around the observed port and restart. Do not open thousands of
  listeners blindly.
- If the device is unavailable, confirm Wi-Fi association, Tailscale status,
  the phone is unlocked, and the bridge logs before pairing again.
- If pairing identity or TXT values change after an OS update, repeat capture.

Stop and remove the bridge with:

```bash
./teardown.sh
```

## License

MIT. See [LICENSE](LICENSE).
