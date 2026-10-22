#!/bin/sh
set -eu
# Install or update the bundle and expose the same executable on PATH.
if [ "$#" -gt 1 ]; then
  echo 'Usage: ./scripts/install.sh [path/to/Midwing.app]' >&2
  exit 1
fi
source_app="${1:-build/bin/midwing.app}"
if [ ! -d "$source_app" ]; then
  echo 'Build first: go run github.com/wailsapp/wails/v2/cmd/wails@v2.17.0 build' >&2
  exit 1
fi
source_app=$(cd "$source_app" && pwd -P)
identifier=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$source_app/Contents/Info.plist" 2>/dev/null || true)
if [ ! -x "$source_app/Contents/MacOS/midwing" ] || [ "$identifier" != 'com.wails.midwing' ]; then
  echo "Not a Midwing app with an executable CLI: $source_app" >&2
  exit 1
fi
app_dir="$HOME/Applications"
mkdir -p "$app_dir"
app_dir=$(cd "$app_dir" && pwd -P)
target_app="$app_dir/Midwing.app"
target_binary="$target_app/Contents/MacOS/midwing"
if [ -L "$target_app" ]; then
  echo "Refusing to replace a symlink at $target_app." >&2
  exit 1
fi
if [ -e "$target_app" ]; then
  identifier=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$target_app/Contents/Info.plist" 2>/dev/null || true)
  if [ "$identifier" != 'com.wails.midwing' ]; then
    echo "Refusing to replace an unrelated app at $target_app." >&2
    exit 1
  fi
fi
stage_dir=$(mktemp -d "$app_dir/.midwing-install.XXXXXX")
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ "$status" -ne 0 ] && [ -d "$stage_dir/previous.app" ]; then
    if [ -d "$target_app" ]; then mv "$target_app" "$stage_dir/failed.app"; fi
    mv "$stage_dir/previous.app" "$target_app"
  fi
  rm -rf "$stage_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
cp -R "$source_app" "$stage_dir/Midwing.app"
if [ -d "$target_app" ]; then mv "$target_app" "$stage_dir/previous.app"; fi
mv "$stage_dir/Midwing.app" "$target_app"
# Bundle installation is complete. CLI setup can be retried in the installed app.
rm -rf "$stage_dir"
trap - EXIT HUP INT TERM
echo "Installed $target_app"
if ! "$target_binary" --setup-cli; then
  echo 'Midwing is installed, but CLI setup failed. Launch the app and use CLI setup → Retry setup.' >&2
  exit 1
fi
