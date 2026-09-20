# Security policy

## Sensitive material

Never commit, paste into an issue, or attach to public logs:

- files under `~/Library/Application Support/iPhone Tailnet Bridge/Profiles/`;
- `bridge.local.conf` from the original prototype;
- `authTag`, captured TXT records, or RemotePairing records;
- private Tailscale ACLs, keys, node identifiers, or device addresses;
- signing identities, provisioning profiles, or device diagnostics.

Profiles are written atomically with mode `0600`. The application does not
upload profiles, logs, traffic, analytics, or telemetry.

## Network boundary

The relay is not an authentication layer. Apple's pairing and Tailscale provide
the trust boundaries.

- Never expose relay ports through a public router or public cloud firewall.
- Restrict tailnet access so only the intended development Mac can reach the
  paired iPhone.
- Leave `allowLANClients` unset or `false`. When false, relay sockets accept
  only connections originating from the Mac itself.
- Use only devices and tailnets you control.

The bridge publishes CoreDevice records on the selected physical LAN interface
because Apple does not browse Bonjour on Tailscale's point-to-point interface.
Other LAN devices can observe those multicast advertisements. They cannot use
the relay unless `allowLANClients` is deliberately enabled or another local
security control is bypassed.

## Local process visibility

Apple's `/usr/bin/dns-sd` receives captured TXT values as process arguments.
The bridge discards its output and never writes those values to logs, but
software running under the same macOS user may be able to inspect process
arguments or read the private profile. Protect the Mac account accordingly.

## Reporting a vulnerability

Do not open a public issue containing credentials or reproduction data from a
real device. Contact the repository owner privately with a minimal description,
affected version, and redacted reproduction steps.
