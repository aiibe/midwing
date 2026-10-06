#!/bin/sh
set -eu
# Package a built universal app without Apple Developer credentials.
if [ "$#" -ne 1 ] || ! printf '%s\n' "$1" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo 'Usage: ./scripts/package-macos.sh v0.1.0' >&2
  exit 1
fi
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
cp -R "$source_app" "$stage_dir/Midwing.app"
cp scripts/install.sh "$stage_dir/install.sh"
cat > "$stage_dir/INSTALL.md" <<'NOTES'
# Install Midwing on macOS

This app supports Apple Silicon and Intel Macs. It is ad hoc signed, without
an Apple Developer ID or Apple notarization. Gatekeeper may block first launch.

From this extracted directory, run:

    ./install.sh ./Midwing.app
    open "$HOME/Applications/Midwing.app"

If macOS blocks the app and you trust the download, attempt to open it, then
use System Settings → Privacy & Security → Open Anyway.
Apple's instructions: https://support.apple.com/102445

The installer also creates ~/.local/bin/midwing. Add ~/.local/bin to your PATH
if needed. Approve and open the desktop app before trying the CLI.

Close Midwing before updating. Reinstalling preserves your connections.
NOTES
codesign --force --sign - "$stage_dir/Midwing.app"
codesign --verify --deep --strict "$stage_dir/Midwing.app"
archive="midwing-$1-macos-universal.zip"
ditto -c -k --sequesterRsrc "$stage_dir" "$release_dir/$archive"
(cd "$release_dir" && shasum -a 256 "$archive" > "$archive.sha256")
echo "Packaged $release_dir/$archive"
