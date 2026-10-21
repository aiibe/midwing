<p align="center">
  <img src="docs/logo.png" alt="Midwing logo" width="310">
</p>

# Midwing

Give your coding agent a little wingman. 🪽

Connect your APIs, choose which requests your coding agent can make, and keep tokens in your OS keychain. Midwing supports token and email/password authentication. Manage connections in the desktop app; the CLI works even when it's closed.

## Download and install on macOS

Download and extract the universal macOS ZIP from [Releases](https://github.com/aiibe/midwing/releases) (Apple Silicon and Intel), then run:

```sh
./install.sh ./Midwing.app
open "$HOME/Applications/Midwing.app"
```

Releases are ad hoc signed and not notarized. If macOS blocks a trusted download, attempt to open it, then use **System Settings → Privacy & Security → Open Anyway** ([Apple's instructions](https://support.apple.com/102445)). Open the app once before using the CLI.

The installer uses `~/Applications` and `~/.local/bin`; add the latter to PATH. Override them with `MIDWING_APP_DIR` and `MIDWING_BIN_DIR`. To update, close Midwing and rerun the installer; connections are preserved.

Verify downloads with `shasum -a 256 -c midwing-vVERSION-macos-universal.zip.sha256`, using your version and the matching checksum file.

Release builds check GitHub for updates on launch and at most daily, without sending credentials. Updates are installed manually. See the [changelog](CHANGELOG.md) for changes.

## Build and install from source

Requires Go 1.24+, Node 22.20+ on the 22.x line or Node 24.12+, and the [Wails v2 native prerequisites](https://v2.wails.io/docs/gettingstarted/installation/).

```sh
go run github.com/wailsapp/wails/v2/cmd/wails@v2.12.0 build
./scripts/install.sh # macOS
midwing
```

On Windows and Linux, put the executable from `build/bin` on PATH. Linux also needs an unlocked Secret Service keyring.

## Take it for a spin

Run `midwing` to [add your API](#bring-your-own-api) and connect it. With the corresponding paths and methods enabled:

```sh
midwing services
midwing services my-service
midwing get my-service /items
midwing post my-service /items '{"name":"Example"}'
midwing get my-service /items/123
```

The CLI returns JSON, one page per call; errors go to stderr with a nonzero exit code. Append `--help` to any command for details.

### Introduce your agent

Tell your coding agent: “Use Midwing for connected APIs. Start with `midwing --help` and `midwing services`. Manage credentials and permissions in the desktop app.”

## Bring your own API

On **Connections → + Add service**, enter a name, CLI ID, HTTPS base URL, authentication settings, and paths such as `/items` or `/items/{id}`. Choose GET/POST and allowed query names per path. Connect the service and grant permissions separately for each method.

- **Bearer token:** header `Authorization`, prefix `Bearer` in the app.
- **API key:** the API's header (e.g. `X-API-Key`), empty prefix.
- **Email/password:** a JSON login endpoint and token response field. Only the token is stored; an optional refresh endpoint renews it before each API request.

To create a service through the CLI:

```sh
midwing services schema               # Fields, defaults, and examples
midwing services validate service.json
midwing services create service.json
```

Example definition (omitted `methods` defaults to GET; POST bodies are JSON, up to 1 MiB):

```json
{"id":"my-service","name":"My service","baseUrl":"https://api.example.com/v1","authKind":"token","header":"Authorization","prefix":"Bearer ","endpoints":[{"path":"/items","methods":["GET","POST"],"queryParameters":["limit"]}]}
```

CLI bearer prefixes need a trailing space: `"Bearer "`. Enter credentials in the app after creation.

Optional `"headers": {"X-API-Version": "2026-08-14"}` adds non-secret headers to API, login, and refresh requests. Authentication, transport, and Midwing-managed headers are reserved.

Use `midwing services update service.json` with a full definition:

- Replace endpoints, methods, query parameters, and static headers; keep ID, name, base URL, and authentication unchanged.
- Omitted endpoints are removed; omitted or empty `headers` removes static headers.
- Request changes require reconnecting and reviewing permissions. Saved credentials and selections remain stored. Description-only edits, also available in the app, keep connections active.

`midwing services delete my-service` or **Delete service** removes the definition, connection, and credential. **Disconnect** keeps the definition.

## Under the wing

- Local app and CLI, with no background service or hosted backend.
- Requests require endpoint and connection permissions, use fixed HTTPS destinations, disable redirects, and time out after 30 seconds.
- Responses are limited to 8 MiB (1 MiB for authentication). Use `midwing post SERVICE PATH JSON --output FILE` to save binary responses; existing files are never overwritten.
- Tokens stay in the OS keychain; settings and activity live under `midwing/` in the OS user config directory. Activity excludes query values and request/response bodies.
- Processes with unrestricted access as your OS user can still access the credential store.

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

Tests need no live credentials. See [service validation](internal/midwing/custom.go).

## Release a version

Record changes under **Unreleased** in [CHANGELOG.md](CHANGELOG.md). Use a patch for fixes or a minor for new capabilities, including during `0.x`. Commit changes, then run from a clean, up-to-date release branch (Python 3 required):

```sh
git fetch origin --tags
./scripts/release.sh 0.2.0
```

The script versions nonempty changelog entries, updates `wails.json`, and creates a commit and annotated tag. The version must be newer than existing releases.

Review the commit and tag, then run the printed atomic push command. CI builds and checks the app and creates a **draft** GitHub release. Review and publish it, then verify the release is public.

Preview notes and test release tooling:

```sh
python3 scripts/release.py notes v0.1.2
python3 -m unittest discover -s scripts -p 'test_*.py'
```
