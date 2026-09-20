# ChatGPT Remote / Codex prompt

Copy this prompt into ChatGPT Remote after the bridge is configured and the Mac
host is awake and online:

```text
Use the physical iPhone connected through iPhone Tailnet Bridge. Do not use a
simulator unless I explicitly ask for one.

1. Run `iphone-tailnet-bridge status`, then `xcrun devicectl list devices`, and
   identify the paired, available iPhone.
2. If the bridge says `local`, use Apple's normal LAN connection. If it says
   `active`, use the tailnet bridge. Do not start a second bridge process.
3. Inspect the project instructions and use the project's normal build command.
   If there is no project-specific command, use Apple's command-line tooling
   (`xcodebuild` and `xcrun devicectl`) rather than opening the Xcode GUI.
4. Build and sign the app for the physical iPhone.
5. Install the resulting signed .app on the identified device.
6. Launch the app on the device.
7. Report these as separate results: bridge, build, installation, and launch.

Do not claim success until the relevant command exits successfully. If the
device is unavailable, diagnose the bridge, Tailscale path, Mac network
interface, and the phone's Wi-Fi/unlock state before suggesting a USB cable.

Do not print, expose, copy, or modify bridge profiles, `bridge.local.conf`,
authTag, pairing records, signing credentials, tailnet addresses, or other
secrets. Do not change the bridge configuration unless I explicitly ask.
```

For a specific project, append its build command, scheme, device name, or bundle
identifier after the prompt. Never paste the private bridge configuration into
ChatGPT.
