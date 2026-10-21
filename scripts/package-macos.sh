#!/bin/sh
set -eu
# Sign and notarize a universal app and its disk image.
if [ "$#" -ne 1 ] || ! printf '%s\n' "$1" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo 'Usage: ./scripts/package-macos.sh v0.1.0' >&2
  exit 1
fi
release_tag=$1
signing_identity() {
  class=$1 variable=$2 identity=$3
  if [ -z "$identity" ]; then
    if [ -n "${NOTARY_KEYCHAIN:-}" ]; then
      identities=$(security find-identity -v -p basic "$NOTARY_KEYCHAIN")
    else
      identities=$(security find-identity -v -p basic)
    fi
    identities=$(printf '%s\n' "$identities" | sed -n "s/.*\"\($class:.*\)\".*/\1/p")
    count=$(printf '%s\n' "$identities" | sed '/^$/d' | wc -l | tr -d ' ')
    if [ "$count" -ne 1 ]; then
      echo "Expected one $class identity; found $count. Set $variable explicitly." >&2
      exit 1
    fi
    identity=$identities
  fi
  case "$identity" in
    "$class:"*) printf '%s\n' "$identity" ;;
    *) echo "$variable must be a $class certificate name." >&2; exit 1 ;;
  esac
}
SIGNING_IDENTITY=$(signing_identity 'Developer ID Application' SIGNING_IDENTITY "${SIGNING_IDENTITY:-}")
NOTARY_PROFILE=${NOTARY_PROFILE:-midwing-notary}
cd "$(dirname "$0")/.."
source_app='build/bin/midwing.app'
if [ ! -d "$source_app" ]; then
  echo 'Build the universal macOS app first; see README.md.' >&2
  exit 1
fi
lipo "$source_app/Contents/MacOS/midwing" -verify_arch arm64 x86_64
release_dir='build/release'
mkdir -p "$release_dir"
stage_dir=$(mktemp -d)
trap 'rm -rf "$stage_dir"' EXIT
trap 'exit 1' HUP INT TERM
payload="$stage_dir/image"
mkdir -p "$payload"
app="$payload/Midwing.app"
ditto "$source_app" "$app"
ln -s /Applications "$payload/Applications"

notarize() {
  submission=$1
  staple_target=$2
  shift 2
  xcrun notarytool submit "$submission" --keychain-profile "$NOTARY_PROFILE" "$@" \
    --wait --timeout 30m --output-format json > "$stage_dir/notarization.json"
  python3 - "$stage_dir/notarization.json" <<'PY'
import json
from pathlib import Path
import sys

result = json.loads(Path(sys.argv[1]).read_text())
if result.get('status') != 'Accepted':
    sys.exit('Notarization failed: ' + json.dumps(result))
print('Apple notarization accepted: ' + result['id'])
PY
  xcrun stapler staple "$staple_target"
  xcrun stapler validate "$staple_target"
}

# The Wails bundle contains one native executable and no nested code bundles.
set --
if [ -n "${NOTARY_KEYCHAIN:-}" ]; then set -- --keychain "$NOTARY_KEYCHAIN"; fi
codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$@" "$app"
# Restrict verification to the expected Developer ID certificate class.
codesign --verify --deep --strict \
  -R '=anchor apple generic and certificate leaf[field.1.2.840.113635.100.6.1.13] exists' "$app"
ditto -c -k --sequesterRsrc --keepParent "$app" "$stage_dir/notarize.zip"
notarize "$stage_dir/notarize.zip" "$app" "$@"
spctl --assess --type execute --verbose=2 "$app"
archive="midwing-$release_tag-macos-universal.dmg"
hdiutil create -volname Midwing -srcfolder "$payload" -format UDZO "$stage_dir/$archive"
codesign --force --timestamp --sign "$SIGNING_IDENTITY" \
  --identifier com.wails.midwing.dmg "$@" "$stage_dir/$archive"
codesign --verify --strict \
  -R '=anchor apple generic and certificate leaf[field.1.2.840.113635.100.6.1.13] exists' "$stage_dir/$archive"
notarize "$stage_dir/$archive" "$stage_dir/$archive" "$@"
hdiutil verify "$stage_dir/$archive"
spctl --assess --type open --context context:primary-signature --verbose=2 "$stage_dir/$archive"
(cd "$stage_dir" && shasum -a 256 "$archive" > "$archive.sha256")
mv "$stage_dir/$archive" "$stage_dir/$archive.sha256" "$release_dir/"
echo "Packaged $release_dir/$archive"
