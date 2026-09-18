# Security policy

## Keep pairing data private

Never commit or paste:

- `bridge.local.conf`
- `authTag`
- captured pairing records or authentication logs
- private Tailscale ACLs, keys, or device credentials

The public repository intentionally contains placeholders only.

## Network exposure

Use Tailscale or another authenticated private network. Do not forward the
bridge ports from the public internet. The relay passes Apple traffic through;
it is not an authorization layer by itself.

## Reporting

Please do not open a public issue for a suspected security vulnerability.
Contact the repository maintainer privately with reproduction details and avoid
including pairing credentials in the report.
