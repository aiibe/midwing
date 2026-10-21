#!/bin/sh
set -eu
# Import CI-only credentials into a temporary keychain, removed by the workflow.
: "${RUNNER_TEMP:?Run this script on a GitHub Actions runner}"
: "${MACOS_CERTIFICATE_P12:?Set the base64-encoded Developer ID certificate secret}"
: "${MACOS_CERTIFICATE_PASSWORD:?Set the certificate export password secret}"
: "${MACOS_INSTALLER_CERTIFICATE_P12:?Set the base64-encoded Developer ID Installer certificate secret}"
: "${MACOS_INSTALLER_CERTIFICATE_PASSWORD:?Set the installer certificate export password secret}"
: "${SIGNING_IDENTITY:?Set the Developer ID Application identity}"
: "${INSTALLER_SIGNING_IDENTITY:?Set the Developer ID Installer identity}"
: "${APPLE_ID:?Set the Apple ID secret}"
: "${APPLE_TEAM_ID:?Set the Apple Developer team ID secret}"
: "${APPLE_APP_PASSWORD:?Set the Apple app-specific password secret}"
umask 077
keychain="$RUNNER_TEMP/midwing-signing.keychain-db"
certificates=$(mktemp -d "$RUNNER_TEMP/midwing-certificates.XXXXXX")
trap 'rm -rf "$certificates"' EXIT
trap 'exit 1' HUP INT TERM
python3 - "$certificates" <<'PY'
import base64
import os
from pathlib import Path
import sys

for name in ['MACOS_CERTIFICATE_P12', 'MACOS_INSTALLER_CERTIFICATE_P12']:
    (Path(sys.argv[1]) / f'{name}.p12').write_bytes(base64.b64decode(os.environ[name], validate=True))
PY
keychain_password=$(openssl rand -hex 32)
security create-keychain -p "$keychain_password" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$keychain_password" "$keychain"
# Signing tools also consult the user search list when resolving identities and chains.
python3 - "$keychain" <<'PY'
import shlex
import subprocess
import sys

keychain = sys.argv[1]
existing = shlex.split(subprocess.check_output(['security', 'list-keychains', '-d', 'user'], text=True))
subprocess.run(['security', 'list-keychains', '-d', 'user', '-s', keychain,
                *[path for path in existing if path != keychain]], check=True)
PY
security import "$certificates/MACOS_CERTIFICATE_P12.p12" -f pkcs12 -k "$keychain" -P "$MACOS_CERTIFICATE_PASSWORD" \
  -T /usr/bin/codesign > /dev/null
security import "$certificates/MACOS_INSTALLER_CERTIFICATE_P12.p12" -f pkcs12 -k "$keychain" -P "$MACOS_INSTALLER_CERTIFICATE_PASSWORD" \
  -T /usr/bin/productbuild > /dev/null
security set-key-partition-list -S apple-tool:,apple:,codesign: \
  -s -k "$keychain_password" "$keychain" > /dev/null
xcrun notarytool store-credentials midwing-notary --keychain "$keychain" \
  --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD"
