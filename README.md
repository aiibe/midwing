<p align="center">
  <img src="docs/logo.png" alt="Midwing logo" width="310">
</p>

# Midwing

Give your coding agent a little wingman. 🪽

Midwing is a small desktop app and CLI for reading APIs through your connected accounts. Connect an account, pick the endpoints your agent can use, and let it get to work. Tokens stay tucked away in your OS keychain.

Bring your own APIs with token or email/password authentication. Manage connections in the app, then close it whenever you like—the CLI keeps working.

Release builds check for newer published versions when the desktop app opens and at most once a day while it stays open. A notice links to the release download; install updates manually to preserve your connections. Checks send only a public release request to GitHub, without account credentials.

## Download and install on macOS

Download the universal macOS ZIP from this repository's **Releases** page. It supports Apple Silicon and Intel Macs. Extract it, then run from the extracted directory:

```sh
./install.sh ./Midwing.app
open "$HOME/Applications/Midwing.app"
```

These releases are ad hoc signed, without an Apple Developer ID or Apple notarization. If macOS blocks the app and you trust the download, attempt to open it, then use **System Settings → Privacy & Security → Open Anyway**. See [Apple's instructions](https://support.apple.com/102445). Open the desktop app before using the CLI.

The installer also exposes `midwing` in `~/.local/bin`; add that directory to your PATH if needed. To verify the downloaded ZIP, run `shasum -a 256 -c midwing-v0.1.0-macos-universal.zip.sha256` with its matching checksum file in the same directory, substituting your release version.

## Build and install from source

Requires Go 1.24+, Node 22.20+ on the 22.x line or Node 24.12+, and the [Wails v2 native prerequisites](https://v2.wails.io/docs/gettingstarted/installation/).

Run from the repository root:

```sh
go run github.com/wailsapp/wails/v2/cmd/wails@v2.11.0 build
```

On macOS, install the app and CLI:

```sh
./scripts/install.sh
midwing
```

The installer uses `~/Applications/Midwing.app` and `~/.local/bin/midwing`. Add `~/.local/bin` to your PATH if needed. Close Midwing before updating; rerunning the installer preserves your connections. Custom locations use `MIDWING_APP_DIR` and `MIDWING_BIN_DIR`.

On Windows and Linux, put the executable from `build/bin` on PATH. Linux also needs an unlocked Secret Service keyring.

## Take it for a spin

Run `midwing` to open the app. Add your API with **+ Add service**, enter its credentials, choose endpoint permissions, and save the connection. The examples below use a service named `my-service` with allowed `/items` and `/items/{id}` endpoints.

```sh
midwing services
midwing services my-service
midwing get my-service /items
midwing get my-service /items/123
```

The CLI returns JSON, one page per call. Errors go to stderr with a nonzero exit code. Use `midwing --help` or append `--help` to a command for details.

### Introduce your agent

Open **Agent setup → Copy prompt** and paste it into your coding agent. The prompt walks your agent through installing a reusable skill and checking the CLI. If it does not support persistent skills, it can follow the guidance in the conversation.

The prompt contains no credentials. To refresh an installed skill after an update, copy the prompt again.

## Bring your own API

On **Connections**, click **+ Add service**. Enter a name, CLI ID, HTTPS base URL, authentication settings, and allowed GET paths such as `/items` or `/items/{id}`. List any allowed query names. Create the service, then connect and select its endpoint permissions.

For bearer tokens, use header `Authorization` and prefix `Bearer` in the form. For raw API keys, use the API's header (such as `X-API-Key`) and leave the prefix empty.

Email/password services need a JSON login endpoint and a token response field. Only the returned token is stored. An optional refresh endpoint renews it before each API read.

More at home in a terminal? Create definitions through the CLI:

```sh
midwing services schema               # Fields, defaults, and examples
midwing services validate service.json
midwing services create service.json
midwing services my-service
midwing get my-service /items
```

CLI definitions use `"Bearer "` with a trailing space. That little space matters. Enter credentials in the desktop app after creation.

Edit permission descriptions in the app or with `midwing services update service.json`. Other definition changes require deleting and recreating the service. **Disconnect** removes its saved connection; **Delete service** also removes the custom definition.

## Under the wing

Midwing checks endpoint and connection permissions before API reads. Tokens live in the OS keychain; settings and activity are stored in the OS user config directory under `midwing/`. Activity excludes query values and response bodies.

Requests use fixed HTTPS destinations, disable redirects, and time out after 30 seconds. Responses must be JSON and are limited to 8 MiB (1 MiB for authentication). Midwing runs locally without a background service or hosted backend. Processes with unrestricted access under your OS user can still access the credential store.

## Tinker with it

```sh
go run github.com/wailsapp/wails/v2/cmd/wails@v2.11.0 dev
```

Give your changes a check before takeoff:

```sh
npm --prefix frontend run build
npm --prefix frontend test
go test ./...
go vet ./...
```

Tests use an in-memory credential vault and HTTP transport; no live credentials are needed. Service validation lives in [internal/midwing/services.go](internal/midwing/services.go), and the bundled agent skill lives in [internal/midwing/skill/SKILL.md](internal/midwing/skill/SKILL.md).
