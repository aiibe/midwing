# macOS distribution

Starting with v0.6.0, packaging signs the universal app with Developer ID
Application, Hardened Runtime, and a timestamp, then notarizes, staples, and
verifies it. A compressed read-only UDIF DMG contains `Midwing.app` and an
`Applications` shortcut. The image is separately signed, notarized, stapled, and
verified before the DMG and SHA-256 checksum are published.

The two notarization passes retain the existing app verification flow. They are
not a requirement for every DMG workflow. The bundle ID remains
`com.wails.midwing`, with macOS 13 as the minimum. The DMG uses
`com.wails.midwing.dmg`. CLI setup is an explicit per-user step in the app; see
[installation and migration](INSTALL-macos.md).

Historical assets stay available: ZIPs through v0.4.0, then PKGs before v0.6.0.
Release notes preserve the instructions appropriate to each version.

See [Apple's distribution guidance](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution)
and [notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow).

## Local packaging

Install your **Developer ID Application** certificate and private key. Check with
`security find-identity -v -p basic`. Packaging selects it when exactly one is
available; otherwise set `SIGNING_IDENTITY` to its full name. An Installer
certificate is no longer needed.

Create the default Keychain profile once. The command prompts for your Apple ID
and an [app-specific password](https://account.apple.com/):

```sh
xcrun notarytool store-credentials midwing-notary --team-id YOUR_TEAM_ID
```

Build and package the version being released (replace `VERSION`). Set the release
repository to enable update notifications, as CI does:

```sh
export CGO_CFLAGS="-mmacosx-version-min=13.0"
export CGO_LDFLAGS="-mmacosx-version-min=13.0"
go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 build -platform darwin/universal -trimpath -ldflags '-X main.releaseRepository=aiibe/midwing'
./scripts/package-macos.sh vVERSION
```

Output is `build/release/midwing-vVERSION-macos-universal.dmg` and its
`.dmg.sha256`. No new release assets are moved out of staging unless all checks
pass. Set `NOTARY_PROFILE` to reuse another profile, and `NOTARY_KEYCHAIN` if the
profile and identity use a separate keychain.

If Apple rejects a submission, inspect it with
`xcrun notarytool log ID --keychain-profile YOUR_PROFILE`, using the reported ID.
For a release smoke test, mount the DMG read-only, copy the app out with Finder
or `ditto`, verify the copied app with `codesign`, `xcrun stapler validate`, and
`spctl --assess --type execute`, then always detach the image. Verify fresh-user
CLI setup in a terminal and an upgrade from the last PKG. Inspecting universal
architectures does not establish execution coverage on both CPU types.

## GitHub Actions credentials

Export your **Developer ID Application** identity (certificate and private key)
as a password-protected `.p12`. Keep it private and out of the repo.

Under repository **Settings → Secrets and variables → Actions**, set
`SIGNING_IDENTITY` to its full certificate name and add these secrets:

| Secret | Value |
| --- | --- |
| `MACOS_CERTIFICATE_P12` | Base64 of the exported `.p12`, on one line |
| `MACOS_CERTIFICATE_PASSWORD` | Password chosen when exporting the `.p12` |
| `APPLE_ID` | Apple Developer account email |
| `APPLE_TEAM_ID` | Developer team ID |
| `APPLE_APP_PASSWORD` | App-specific password for notarization |

Upload the certificate without printing its contents:

```sh
base64 < /path/to/developer-id.p12 | tr -d '\n' | gh secret set MACOS_CERTIFICATE_P12
```

Use `gh secret set NAME` for the remaining secrets; it prompts for each value.
CI imports PKCS#12 explicitly into a temporary keychain, preserves the keychain
search list, restricts signing access, and removes the keychain even on failure.
Installer secrets and certificates are unused; migrating the code does not
remove repository secrets or revoke certificates.

Follow the [release process](../README.md#release-a-version) to publish.
The next planned version for this capability is v0.6.0; implementation changes
remain under Unreleased until release preparation.
