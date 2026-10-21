<p align="center">
  <img src="docs/logo.png" alt="Midwing logo" width="310">
</p>

# Midwing

Give your coding agent a little wingman. 🪽

Midwing is a desktop app and CLI for calling APIs through your connected accounts. Connect an account and choose which requests your agent can make. Tokens stay in your OS keychain.

Bring your own APIs with token or email/password authentication. Manage connections in the app, then close it whenever you like—the CLI keeps working.

## Download and install on macOS

Download the universal macOS ZIP from [Releases](https://github.com/aiibe/midwing/releases) for Apple Silicon or Intel. See the [changelog](CHANGELOG.md) for changes. Extract the ZIP, then run:

```sh
./install.sh ./Midwing.app
open "$HOME/Applications/Midwing.app"
```

These releases are ad hoc signed, without an Apple Developer ID or Apple notarization. If macOS blocks the app and you trust the download, attempt to open it, then use **System Settings → Privacy & Security → Open Anyway**. See [Apple's instructions](https://support.apple.com/102445). Open the desktop app before using the CLI.

The installer puts the app in `~/Applications` and the CLI in `~/.local/bin`; add the latter to your PATH if needed. Close Midwing before updating; rerunning the installer preserves your connections. Override the locations with `MIDWING_APP_DIR` and `MIDWING_BIN_DIR`.

To verify the ZIP, run `shasum -a 256 -c midwing-v0.1.0-macos-universal.zip.sha256` alongside the matching checksum file, substituting your version.

Release builds check GitHub for updates on launch and at most daily, without sending account credentials. Follow the update notice to download and install a newer version manually.

## Build and install from source

Requires Go 1.24+, Node 22.20+ on the 22.x line or Node 24.12+, and the [Wails v2 native prerequisites](https://v2.wails.io/docs/gettingstarted/installation/).

Run from the repository root:

```sh
go run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 build
```

On macOS, install the app and CLI:

```sh
./scripts/install.sh
midwing
```

On Windows and Linux, put the executable from `build/bin` on PATH. Linux also needs an unlocked Secret Service keyring.

## Take it for a spin

Run `midwing` to open the app and [add your API](#bring-your-own-api). These examples use `my-service` with GET and POST allowed on `/items`, and GET on `/items/{id}`:

```sh
midwing services
midwing services my-service
midwing get my-service /items
midwing post my-service /items '{"name":"Example"}'
midwing get my-service /items/123
```

The CLI returns JSON, one page per call. Errors go to stderr with a nonzero exit code. Use `midwing --help` or append `--help` to a command for details.

### Introduce your agent

Open **Agent setup → Copy prompt** and paste it into your coding agent. It guides the agent through installing a skill and checking the CLI, or following the instructions in conversation if skills are unsupported. The prompt contains no credentials. Copy it again to refresh an installed skill after an update.

## Bring your own API

On **Connections**, click **+ Add service**. Enter a name, CLI ID, HTTPS base URL, authentication settings, and allowed paths such as `/items` or `/items/{id}`. Select GET and/or POST for each path and list any allowed query names. Each method has a separate permission; connect the service and select the requests agents may make.

For bearer tokens, use header `Authorization` and prefix `Bearer` in the form. For raw API keys, use the API's header (such as `X-API-Key`) and leave the prefix empty.

Email/password services need a JSON login endpoint and a token response field. Only the returned token is stored. An optional refresh endpoint renews it before each API request.

To create a service through the CLI:

```sh
midwing services schema               # Fields, defaults, and examples
midwing services validate service.json
midwing services create service.json
```

Omitting `methods` means GET only. This definition enables GET and POST; POST accepts a JSON body up to 1 MiB:

```json
{"id":"my-service","name":"My service","baseUrl":"https://api.example.com/v1","authKind":"token","header":"Authorization","prefix":"Bearer ","endpoints":[{"path":"/items","methods":["GET","POST"],"queryParameters":["limit"]}]}
```

CLI definitions use `"Bearer "` with a trailing space. Enter credentials in the desktop app after creation.

Edit permission descriptions in the app or with `midwing services update service.json`. Other definition changes require deleting and recreating the service. **Disconnect** removes its saved connection; **Delete service** also removes the custom definition.

## Under the wing

Midwing checks endpoint method and connection permissions before API requests. Tokens live in the OS keychain; settings and activity are stored in the OS user config directory under `midwing/`. Activity excludes query values and request/response bodies.

Requests use fixed HTTPS destinations, disable redirects, and time out after 30 seconds. Responses must be JSON and are limited to 8 MiB (1 MiB for authentication). Midwing runs locally without a background service or hosted backend. Processes with unrestricted access under your OS user can still access the credential store.

## Tinker with it

```sh
go run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 dev
```

Run checks:

```sh
npm --prefix frontend run build
npm --prefix frontend test
go test ./...
go vet ./...
```

Tests use an in-memory credential vault and HTTP transport; no live credentials are needed. Service validation lives in [internal/midwing/services.go](internal/midwing/services.go), and the bundled agent skill lives in [internal/midwing/skill/SKILL.md](internal/midwing/skill/SKILL.md).

## Release a version

Add notable changes under **Unreleased** in [CHANGELOG.md](CHANGELOG.md). Choose a patch for fixes or a minor for new capabilities, including during `0.x`.

Commit all changes. From a clean, up-to-date release branch, run (requires Python 3):

```sh
git fetch origin --tags
./scripts/release.sh 0.2.0
```

The script versions the pending changelog entries, updates `wails.json`, and creates a commit and annotated tag. Notes must be nonempty and the version newer than existing releases.

Review the commit and tag, then run the printed atomic push command. CI builds and checks the app, then creates a **draft** GitHub release using the changelog section and installation instructions. Review and publish the draft on GitHub.

Preview notes and test release tooling:

```sh
python3 scripts/release.py notes v0.1.2
python3 -m unittest discover -s scripts -p 'test_*.py'
```
