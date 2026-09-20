# Changelog

## 1.0.0 — 2026-09-20

- Replaced the shell and `socat` process farm with one tested Go relay.
- Added a native macOS menu-bar app and automatic paired-device setup.
- Added Tailscale peer matching, adaptive dynamic-port discovery, and network-change recovery.
- Removed the `/etc/hosts`, root, and Homebrew requirements.
- Added owner-only profiles, local-client enforcement, migration, diagnostics, CI, and signed app builds.
- Added explicit `Local`, `Active`, `Wi-Fi Required`, `Degraded`, and error states.
