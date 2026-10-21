#!/usr/bin/env python3
"""Prepare local releases and render their GitHub release notes."""

import argparse
from datetime import date
import json
from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parent.parent
VERSION = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")
INSTALL_NOTES = """## Installation

macOS release for Apple Silicon and Intel Macs.

This app is ad hoc signed, without an Apple Developer ID or notarization. macOS may block its first launch. If you trust this download, attempt to open it, then use System Settings → Privacy & Security → Open Anyway.

Extract the ZIP and run `./install.sh ./Midwing.app` from the extracted directory to install both the desktop app and CLI. See INSTALL.md in the archive for details.
"""


def version_parts(version):
    if not VERSION.fullmatch(version):
        raise ValueError("Use a version such as 0.2.0 (without a v prefix).")
    return tuple(map(int, version.split(".")))


def sections(text):
    headings = list(re.finditer(r"^## (.+)$", text, re.MULTILINE))
    return [(heading.group(1), heading.start(), heading.end(),
             headings[i + 1].start() if i + 1 < len(headings) else len(text))
            for i, heading in enumerate(headings)]


def has_entries(body):
    return bool(re.search(r"^[-*] \S", body, re.MULTILINE))


def release_section(text, version):
    version_parts(version)
    matches = [section for section in sections(text)
               if re.match(r"\[" + re.escape(version) + r"\]\(.*\) — \d{4}-\d{2}-\d{2}$", section[0])]
    if len(matches) != 1:
        raise ValueError(f"Expected one dated, linked changelog section for {version}.")
    _, _, start, end = matches[0]
    body = text[start:end].strip()
    if not has_entries(body):
        raise ValueError(f"Changelog section for {version} is empty.")
    return body


def prepare_changelog(text, version, today):
    parts = version_parts(version)
    existing = sections(text)
    pending = [section for section in existing if section[0] == "Unreleased"]
    if len(pending) != 1 or existing[0] != pending[0]:
        raise ValueError("Expected one Unreleased section before all releases.")
    for heading, *_ in existing:
        match = re.match(r"\[([0-9]+\.[0-9]+\.[0-9]+)\]", heading)
        if match and version_parts(match[1]) >= parts:
            raise ValueError("The release version must be newer than all changelog releases.")
    _, start, body_start, end = pending[0]
    body = text[body_start:end].strip()
    if not has_entries(body):
        raise ValueError("Unreleased must contain at least one changelog entry.")
    heading = f"## [{version}](https://github.com/aiibe/midwing/releases/tag/v{version}) — {today}"
    return text[:start] + f"## Unreleased\n\n{heading}\n\n{body}\n\n" + text[end:]


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()


def prepare(version):
    parts = version_parts(version)
    if git("status", "--porcelain"):
        raise ValueError("Commit or stash all changes, including untracked files, before releasing.")
    if not git("branch", "--show-current"):
        raise ValueError("Check out a branch before releasing.")
    for tag in git("tag", "--list").splitlines():
        if tag.startswith("v") and VERSION.fullmatch(tag[1:]) and version_parts(tag[1:]) >= parts:
            raise ValueError("The release version must be newer than all local release tags. Fetch tags first.")
    changelog = ROOT / "CHANGELOG.md"
    updated = prepare_changelog(changelog.read_text(), version, date.today().isoformat())
    config_path = ROOT / "wails.json"
    config = json.loads(config_path.read_text())
    config["info"]["productVersion"] = version
    changelog.write_text(updated)
    config_path.write_text(json.dumps(config, indent=2) + "\n")
    git("add", "--", "CHANGELOG.md", "wails.json")
    git("commit", "-m", f"Release v{version}")
    git("tag", "-a", f"v{version}", "-m", f"Midwing v{version}")
    print(f"Created release commit and tag v{version}. Review them, then push:")
    print(f"git push --atomic origin HEAD refs/tags/v{version}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("prepare").add_argument("version")
    commands.add_parser("notes").add_argument("tag")
    args = parser.parse_args()
    try:
        if args.command == "prepare":
            prepare(args.version)
        else:
            if not args.tag.startswith("v"):
                raise ValueError("Use a release tag such as v0.2.0.")
            body = release_section((ROOT / "CHANGELOG.md").read_text(), args.tag[1:])
            print(body + "\n\n" + INSTALL_NOTES)
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"Release failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
