# Changelog

## Unreleased

### Changed

- Automatically create or update `~/.local/bin/midwing` when the installed macOS app launches. Preserve unrelated commands and respect the saved CLI opt-out, including when setup needs a retry.
- Simplify CLI setup to a link status, Enable CLI preference, and one retry action. Keep terminal PATH help and copy buttons available, with technical errors under Details and sidebar notices only when setup needs attention.
- Use the same CLI setup for source installations in `~/Applications`. Custom installation directories and legacy PKG CLI links are no longer supported; this installation model replaces earlier release behavior without migration support.

### Fixed

- Wait for active CLI setup operations when retrying manually or installing from source; keep app startup nonblocking.
- Keep the CLI opt-out and setup controls available when inspecting or removing the link fails. Keep a successfully installed app available for retry if source CLI setup fails.

## [0.6.0](https://github.com/aiibe/midwing/releases/tag/v0.6.0) — 2026-10-22

### Added

- CLI setup in the macOS app installs and removes a per-user command link, recognizes working PKG links, and offers repair after moving the app. Unrelated commands are preserved, with terminal PATH and migration guidance.

### Changed

- Distribute macOS releases as signed, notarized universal DMGs with drag-and-drop installation. CLI installation is now an explicit setup step; existing connections and credentials are retained. Historical PKG and ZIP assets and release instructions remain available.

### Fixed

- Preserve original CLI links when setup fails on macOS, and show the correct PATH command for working legacy PKG links.

## [0.5.2](https://github.com/aiibe/midwing/releases/tag/v0.5.2) — 2026-10-22

### Fixed

- Make the CI signing keychain available to macOS signing tools while preserving the existing keychain search list.

## [0.5.1](https://github.com/aiibe/midwing/releases/tag/v0.5.1) — 2026-10-22

### Fixed

- Fix macOS release signing failing with “Unknown format in import” when loading PKCS#12 certificates into the CI keychain.
- Correct release command examples, document update-check retries, and enable update notifications in the local release build instructions.

## [0.5.0](https://github.com/aiibe/midwing/releases/tag/v0.5.0) — 2026-10-22

### Added

- Signed and notarized macOS PKG downloads install the app in Applications and the CLI together, without terminal commands. Updates preserve connections; administrator approval is required.

### Security

- Limit activity logs to two 1 MiB files, bound reads of older oversized logs, and keep history access from blocking settings operations.
- Require reviewed service revisions for all connection approvals; reject stale token and password approvals after request settings change.
- Require Go 1.27.3 or newer for patched HTTPS handling and certificate validation; CI now blocks known vulnerabilities in reachable Go code.
- Upgrade Wails to 2.17.0 for compiler compatibility and development-server security fixes.

### Changed

- Require macOS 13 or newer, matching Go 1.27; update app bundle metadata.

### Fixed

- Reject operations that exceed the activity record limit before sending requests, avoiding a logging failure after a successful POST.
- Recover valid activity entries from damaged logs and display a warning instead of hiding newer history.

## [0.4.0](https://github.com/aiibe/midwing/releases/tag/v0.4.0) — 2026-10-21

### Added

- `--output FILE` for GET requests, plus `--accept TYPE` and `--max-bytes N` for GET and POST downloads.

### Fixed

- Empty successful responses print nothing.
- Block credentials in incomplete JSON and JSON Lines downloads, including escaped characters and invalid UTF-8.
- Clean up failed or interrupted downloads, report cleanup errors, and record output failures accurately.

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
