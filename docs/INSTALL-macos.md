# Install Midwing on macOS

Requires macOS 13 or newer. Supports Apple Silicon and Intel Macs.
The app and DMG are Developer ID signed and notarized by Apple.

1. Download and open the universal macOS `.dmg` from [Releases](https://github.com/aiibe/midwing/releases).
2. Drag **Midwing.app** to **Applications**, then eject the image.
3. Launch Midwing from Applications and configure your connections.

On launch, Midwing creates `~/.local/bin/midwing`, including its folder, and
links it to this app's executable. The CLI works after the GUI closes.
You can also install the app in `~/Applications`.

**CLI setup** shows whether the link is ready, disabled, or needs attention.
Use **Retry setup** to retry a failed operation. **Enable CLI** controls automatic
setup; turning it off removes only Midwing's own link and keeps the app and
connections. Turning it back on sets up the link again. Command paths and the
PATH instruction have copy buttons; technical errors are under **Details**.
If disabling cannot remove the link, the opt-out is still saved. Retry setup
after resolving the error to finish removing it.

CLI setup needs no administrator access. Copying or replacing an app in
`/Applications` can still prompt. Setup requires an installed production build;
it refuses disk images, translocated copies, and development locations.

## Make the command available

In a fresh terminal, run:

```sh
type -a midwing
midwing --help
```

Check that the first command uses the intended app. A ready CLI link does not
confirm that your terminal or coding agent can find it. If `~/.local/bin` is
absent from PATH, add this line to your shell configuration (`~/.zprofile` for
the default login zsh), then open a new terminal:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

For bash, use the startup file your terminal reads, usually `~/.bash_profile` for
login shells. For fish, run `fish_add_path ~/.local/bin`.
Restart coding agents and their host apps after changing PATH. Agents that supply
their own environment may need the full path to `~/.local/bin/midwing`.
Midwing never edits shell startup files automatically.

## Update or move the app

Quit Midwing, replace the installed app with the one from the new DMG, then
relaunch it. Future updates using this installation model keep the same CLI link.
If you move the app between `/Applications` and `~/Applications`, launch it from
its new location; Midwing remembers its link and updates it, even if the previous
app is gone. A disabled CLI stays disabled.

Unrelated files and links are preserved. If another command occupies
`~/.local/bin/midwing`, move it yourself before retrying setup.

## Verify a download

Download the matching `.dmg.sha256` alongside the DMG and run from that folder,
replacing `VERSION`:

```sh
shasum -a 256 -c midwing-vVERSION-macos-universal.dmg.sha256
```

## Remove Midwing

- To remove just the CLI link, turn off **CLI setup → Enable CLI**. Automatic
  setup stays disabled until you turn it back on.
- To remove saved credentials, disconnect each service in the app first. Turn off
  **Enable CLI**, quit Midwing, then move the app to Trash.
- To erase local settings and activity, remove
  `~/Library/Application Support/midwing`. Any credentials not disconnected in
  the app can be reviewed in Keychain Access under service `midwing`.

Ejecting or deleting the DMG does not uninstall Midwing or remove its data.
