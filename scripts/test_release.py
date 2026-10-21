import contextlib
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import release


CHANGELOG = """# Changelog

## Unreleased

### Added

- New capability.

## [0.1.0](https://github.com/aiibe/midwing/releases/tag/v0.1.0) — 2026-10-07

### Added

- Initial release.
"""


class ChangelogTests(unittest.TestCase):
    def test_prepare_preserves_history_and_extracts_only_requested_release(self):
        updated = release.prepare_changelog(CHANGELOG, "0.2.0", "2026-10-21")
        self.assertIn("## Unreleased\n\n## [0.2.0]", updated)
        self.assertIn("— 2026-10-21", updated)
        self.assertTrue(updated.endswith(CHANGELOG[CHANGELOG.index("## [0.1.0]"):]))
        self.assertEqual(release.release_section(updated, "0.2.0"), "### Added\n\n- New capability.")
        self.assertEqual(release.release_section(updated, "0.1.0"), "### Added\n\n- Initial release.")

    def test_reject_invalid_or_old_versions(self):
        for version in ["v0.2.0", "0.02.0", "0.2", "0.2.0-beta", "0.1.0", "0.0.9"]:
            with self.subTest(version=version), self.assertRaises(ValueError):
                release.prepare_changelog(CHANGELOG, version, "2026-10-21")

    def test_reject_empty_or_duplicate_unreleased(self):
        for text in [CHANGELOG.replace("- New capability.", ""),
                     CHANGELOG.replace("## Unreleased", "## Pending"),
                     CHANGELOG + "\n## Unreleased\n\n- Duplicate.\n"]:
            with self.subTest(text=text), self.assertRaises(ValueError):
                release.prepare_changelog(text, "0.2.0", "2026-10-21")

    def test_reject_missing_empty_or_duplicate_release_notes(self):
        for text, version in [(CHANGELOG, "0.2.0"),
                              (CHANGELOG.replace("- Initial release.", ""), "0.1.0"),
                              (CHANGELOG + CHANGELOG, "0.1.0")]:
            with self.subTest(version=version), self.assertRaises(ValueError):
                release.release_section(text, version)


class ReleaseRepositoryTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.root_patch = patch.object(release, "ROOT", self.root)
        self.root_patch.start()
        self.addCleanup(self.root_patch.stop)
        self.git("init", "-b", "main")
        self.git("config", "user.name", "Release test")
        self.git("config", "user.email", "release@example.com")
        self.git("config", "commit.gpgsign", "false")
        self.git("config", "tag.gpgsign", "false")
        (self.root / "CHANGELOG.md").write_text(CHANGELOG)
        (self.root / "wails.json").write_text('{"info":{"productVersion":"0.1.0"}}\n')
        self.git("add", ".")
        self.git("commit", "-m", "Initial release")
        self.git("tag", "v0.1.0")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, text=True, stderr=subprocess.PIPE).strip()

    def test_prepare_creates_commit_and_annotated_tag_without_pushing(self):
        with contextlib.redirect_stdout(io.StringIO()) as output:
            release.prepare("0.2.0")
        self.assertEqual(self.git("log", "-1", "--format=%s"), "Release v0.2.0")
        self.assertEqual(self.git("cat-file", "-t", "v0.2.0"), "tag")
        self.assertEqual(self.git("rev-parse", "v0.2.0^{}"), self.git("rev-parse", "HEAD"))
        self.assertEqual(self.git("status", "--porcelain"), "")
        self.assertEqual(json.loads((self.root / "wails.json").read_text())["info"]["productVersion"], "0.2.0")
        self.assertIn("git push --atomic origin HEAD refs/tags/v0.2.0", output.getvalue())

    def test_dirty_tree_is_rejected_without_changes(self):
        (self.root / "untracked.txt").write_text("Work in progress")
        with self.assertRaisesRegex(ValueError, "Commit or stash"):
            release.prepare("0.2.0")
        self.assertEqual((self.root / "CHANGELOG.md").read_text(), CHANGELOG)
        self.assertEqual(self.git("tag", "--list"), "v0.1.0")

    def test_newer_tag_is_rejected(self):
        self.git("tag", "v0.3.0")
        with self.assertRaisesRegex(ValueError, "local release tags"):
            release.prepare("0.2.0")
        self.assertEqual(self.git("status", "--porcelain"), "")

    def test_detached_head_is_rejected(self):
        self.git("checkout", "--detach")
        with self.assertRaisesRegex(ValueError, "Check out a branch"):
            release.prepare("0.2.0")


if __name__ == "__main__":
    unittest.main()
