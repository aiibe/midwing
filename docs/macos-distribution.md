# macOS distribution

Release packaging signs the app with Developer ID and Hardened Runtime, notarizes
and staples it, then creates a Developer ID Installer signed PKG with its own
notarization ticket. Signature and Gatekeeper verification must pass before
publishing the PKG and checksum. Releases through v0.4.0 use ad hoc signed ZIP archives.

The package installs `/Applications/Midwing.app` and its CLI link in
`/usr/local/bin`, requires macOS 13 and administrator approval, and refuses to
overwrite unrelated files. App relocation is disabled to keep the CLI link valid.

See [Apple's notarization guide](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution).

## Local packaging

Install your **Developer ID Application** and **Developer ID Installer** certificates
and private keys. Check with `security find-identity -v -p basic`. Packaging selects
each identity when exactly one of its class is available; otherwise set
`SIGNING_IDENTITY` and `INSTALLER_SIGNING_IDENTITY` to their full names.

Create the default Keychain profile once. The command prompts for your Apple ID
and an [app-specific password](https://account.apple.com/):

```sh
xcrun notarytool store-credentials midwing-notary --team-id YOUR_TEAM_ID
```

Build and package the version being released:

```sh
export CGO_CFLAGS="-mmacosx-version-min=13.0"
export CGO_LDFLAGS="-mmacosx-version-min=13.0"
go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 build -platform darwin/universal -trimpath
./scripts/package-macos.sh vVERSION
```

Output is in `build/release/`. To reuse an existing profile, set
`NOTARY_PROFILE=your-profile` when running the packaging script. Set
`NOTARY_KEYCHAIN` if the profile and signing identity use a separate keychain.

If Apple rejects a submission, inspect it with
`xcrun notarytool log ID --keychain-profile YOUR_PROFILE`, using the reported ID.

## GitHub Actions credentials

In Keychain Access, export your **Developer ID Application** and **Developer ID
Installer** identities (certificate and private key) as separate password-protected
`.p12` files. Keep them private and out of the repo.

Under repository **Settings → Secrets and variables → Actions**, set the variables
`SIGNING_IDENTITY` and `INSTALLER_SIGNING_IDENTITY` to the corresponding full
certificate names and add these secrets:

| Secret | Value |
| --- | --- |
| `MACOS_CERTIFICATE_P12` | Base64 of the exported `.p12`, on one line |
| `MACOS_CERTIFICATE_PASSWORD` | Password chosen when exporting the `.p12` |
| `MACOS_INSTALLER_CERTIFICATE_P12` | Base64 of the exported installer `.p12`, on one line |
| `MACOS_INSTALLER_CERTIFICATE_PASSWORD` | Password chosen when exporting the installer `.p12` |
| `APPLE_ID` | Apple Developer account email |
| `APPLE_TEAM_ID` | Developer team ID |
| `APPLE_APP_PASSWORD` | App-specific password for notarization |

Upload the certificate without printing its contents:

```sh
base64 < /path/to/developer-id.p12 | tr -d '\n' | gh secret set MACOS_CERTIFICATE_P12
base64 < /path/to/developer-id-installer.p12 | tr -d '\n' | gh secret set MACOS_INSTALLER_CERTIFICATE_P12
```

Use `gh secret set NAME` for the remaining secrets; it prompts for each value.
For release tags, CI imports the credentials into a temporary keychain, creates
a `midwing-notary` profile, and removes the keychain even if packaging fails.

Follow the [release process](../README.md#release-a-version) to publish.
