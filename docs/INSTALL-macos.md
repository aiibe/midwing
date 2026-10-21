# Install Midwing on macOS

Requires macOS 13 or newer. Supports Apple Silicon and Intel Macs.
Starting with v0.6.0, the app and DMG are Developer ID signed and notarized by Apple.

1. Download and open the universal macOS `.dmg` from [Releases](https://github.com/aiibe/midwing/releases).
2. Drag **Midwing.app** to **Applications**, then eject the image.
3. Launch Midwing from Applications and configure your connections.
4. Open **CLI setup → Install CLI**. It creates `~/.local/bin/midwing` as a link to this app's executable.

You can also install in `~/Applications`. CLI setup requires an installed release
build in one of these Applications folders; it refuses disk images, translocated
copies, and development builds. The CLI works after the GUI closes.
CLI setup needs no administrator access. Copying or replacing an app in
`/Applications` can still prompt, especially after a root-owned PKG installation.

## Make the command available

In a **new terminal**, run:

```sh
type -a midwing
midwing --help
```

Check that the first command uses the intended app. The GUI cannot determine the
PATH used by your terminal or coding agent. If `~/.local/bin` is absent, add this
line to your shell configuration (`~/.zprofile` for the default login zsh), then
open a new terminal:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

For bash, use the startup file your terminal reads, usually `~/.bash_profile` for
login shells. For fish, run `fish_add_path ~/.local/bin`.
Restart coding agents and their host apps after changing PATH. Agents that supply
their own environment may need an explicit path to `~/.local/bin/midwing`.
Midwing never edits shell startup files automatically.

## Updates and migration

Quit Midwing, replace the installed app with the one from the new DMG, then
relaunch it. Keep the same location to preserve CLI links. Connections, approved
permissions, and Keychain credentials are retained.

- **PKG releases (v0.5.x):** `/usr/local/bin/midwing` remains valid when the app
  stays at `/Applications/Midwing.app`. CLI setup recognizes it without creating
  another link. Add `/usr/local/bin` to your shell PATH if needed.
- **Source/ZIP installs:** check `type -a midwing` for commands using the old app.
  A verified user link can be repaired explicitly in CLI setup. Remove an old
  app only after confirming which copy you want; connections are shared.
- **Moved apps:** launch the app from its new Applications location and choose
  **Repair CLI link**. Midwing remembers links it created so a dangling link can
  be repaired. An unrecognized broken link is preserved for manual review.
- **Conflicts:** unrelated files and links are preserved. Review them yourself;
  setup never removes a system-wide command or an old application.

Releases through v0.4.0 use ad hoc signed ZIPs, without notarization. Follow the
archive's `INSTALL.md`. If macOS blocks a trusted old download, attempt to open it,
then use **System Settings → Privacy & Security → Open Anyway**
([Apple's instructions](https://support.apple.com/102445)). Existing PKGs remain
available in their historical releases.

## Verify a download

Download the matching `.dmg.sha256` alongside the DMG and run from that folder,
replacing `VERSION`:

```sh
shasum -a 256 -c midwing-vVERSION-macos-universal.dmg.sha256
```

For v0.5.x downloads, use the corresponding `.pkg.sha256` instead.

## Remove Midwing

- To remove just the user CLI link, use **CLI setup → Remove CLI** before moving
  or deleting the app. Connections and the app remain available.
- To remove saved credentials, disconnect each service in the app first. Quit
  Midwing, then move the installed app to Trash.
- Review remaining CLI links with `type -a midwing` and `readlink PATH_TO_COMMAND`.
  Delete only links pointing to the removed Midwing app. Removing a legacy
  `/usr/local/bin/midwing` may require administrator approval.
- To erase local settings and activity, remove
  `~/Library/Application Support/midwing`. Any credentials not disconnected in
  the app can be reviewed in Keychain Access under service `midwing`.
- An old PKG receipt is metadata. Optional
  `sudo pkgutil --forget com.wails.midwing.pkg` forgets that receipt; it does not
  remove the app, CLI, settings, or credentials.

A dedicated uninstaller remains planned; ejecting or deleting the DMG does not
uninstall Midwing or remove its data.
