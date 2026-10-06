#!/bin/sh
set -eu
# Install or update the bundle and expose the same executable on PATH.
if [ "$#" -gt 1 ]; then
  echo 'Usage: ./scripts/install.sh [path/to/Midwing.app]' >&2
  exit 1
fi
source_app="${1:-build/bin/midwing.app}"
if [ ! -d "$source_app" ]; then
  echo 'Build first: go run github.com/wailsapp/wails/v2/cmd/wails@v2.11.0 build' >&2
  exit 1
fi
app_dir="${MIDWING_APP_DIR:-$HOME/Applications}"
bin_dir="${MIDWING_BIN_DIR:-$HOME/.local/bin}"
mkdir -p "$app_dir" "$bin_dir"
app_dir=$(cd "$app_dir" && pwd -P)
bin_dir=$(cd "$bin_dir" && pwd -P)
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
if [ -e "$bin_dir/midwing" ] || [ -L "$bin_dir/midwing" ]; then
  if [ ! -L "$bin_dir/midwing" ] || [ "$(readlink "$bin_dir/midwing")" != "$target_binary" ]; then
    echo "Refusing to replace an unrelated command at $bin_dir/midwing." >&2
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
if [ ! -L "$bin_dir/midwing" ]; then ln -s "$target_binary" "$bin_dir/midwing"; fi
echo "Installed Midwing.app and $bin_dir/midwing"
case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "Add to your shell profile: export PATH=\"$bin_dir:\$PATH\"" ;;
esac
