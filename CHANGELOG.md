# Changelog

## Unreleased

## [0.3.0](https://github.com/aiibe/midwing/releases/tag/v0.3.0) — 2026-10-21

### Added

- `midwing post SERVICE PATH JSON --output FILE` accepts any response type and saves up to 8 MiB to a private file without overwriting existing files. Failed requests remove the output file, and credential checks cover both raw bytes and decoded JSON strings.

## [0.2.0](https://github.com/aiibe/midwing/releases/tag/v0.2.0) — 2026-10-21

### Added

- Custom service updates for endpoints, methods, and query parameters. Request changes require reconnecting and reviewing permissions; description edits keep connections active.
- Optional static headers in custom definitions and updates. Authentication and transport overrides are blocked; header changes require reconnecting.
- `midwing services delete ID` to remove a custom service, its connection, and saved credential.
- Release script that versions the changelog and app, then creates a commit and tag.
- Draft GitHub release notes from the changelog, with installation instructions.

### Changed

- Moved agent guidance into CLI help and removed the bundled skill and Agent setup screen.
- Added an agent reminder to review, publish, and verify each new GitHub release.
- Updated CI to Node.js 24.22.1 LTS and GitHub Actions that use the Node 24 runtime.

## [0.1.2](https://github.com/aiibe/midwing/releases/tag/v0.1.2) — 2026-10-21

### Added

- POST requests through `midwing post SERVICE PATH JSON`, with JSON bodies up to 1 MiB.
- Separate GET and POST permissions for each endpoint, configurable in the app and CLI service definitions.

### Changed

- Updated service discovery, schema, and agent guidance to include supported HTTP methods. Existing definitions default to GET only.
- Upgraded Wails to 2.12.0.

## [0.1.1](https://github.com/aiibe/midwing/releases/tag/v0.1.1) — 2026-10-07

### Fixed

- Open the Add service form automatically when the service library is empty, with an initial endpoint row and the correct authentication fields.

### Changed

- Simplified the sidebar footer.

## [0.1.0](https://github.com/aiibe/midwing/releases/tag/v0.1.0) — 2026-10-07

### Added

- Initial desktop app and CLI for GET requests through connected API accounts.
- Custom services with token or email/password authentication, OS keychain storage, and endpoint permissions.
- CLI commands to discover, validate, create, and update service definitions.
- Connection management, editable permission descriptions, and local request activity.
- Agent setup prompt and bundled skill for discovering and using the CLI.
- Release update notices and universal macOS packages with app and CLI installation scripts.
