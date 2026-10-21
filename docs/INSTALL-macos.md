# Install Midwing on macOS

Requires macOS 13 or newer. Supports Apple Silicon and Intel Macs.
The app and installer are Developer ID signed and notarized by Apple.

1. Download and open the universal macOS `.pkg` from [Releases](https://github.com/aiibe/midwing/releases).
2. Follow the installer and approve installation with an administrator account.
3. Open Midwing from Applications to configure connections, then use `midwing` in your terminal.

The installer puts the app in `/Applications/Midwing.app` and links
`/usr/local/bin/midwing` to its executable. Keep the app there so the CLI works.
If your shell does not include `/usr/local/bin` in PATH, add it to your shell configuration.

Close Midwing before updating, then open the new PKG. Connections are preserved.
The installer refuses to overwrite an unrelated app or command at these locations.

## Moving from the script installer

After installing the PKG, remove the old `~/Applications/Midwing.app` and
`~/.local/bin/midwing` (or your custom install paths). In Finder, use
**Go → Go to Folder** to reach each folder and move the old items to Trash.
Keep the app in `/Applications`; saved connections and Keychain credentials are shared.

## Verify a download

Download the matching `.sha256` file alongside the PKG, then run from that folder,
replacing `VERSION` with the release version:

```sh
shasum -a 256 -c midwing-vVERSION-macos-universal.pkg.sha256
```

## Older ZIP releases

Releases through v0.4.0 use ad hoc signed ZIP archives, without notarization.
Extract the ZIP and follow its `INSTALL.md` to install both the app and CLI.
If macOS blocks a trusted download, attempt to open it, then use
**System Settings → Privacy & Security → Open Anyway**
([Apple's instructions](https://support.apple.com/102445)).
