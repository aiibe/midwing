"""Exercise source installation with a fake app that records setup calls."""
import os
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(sys.platform == 'darwin', 'requires macOS PlistBuddy')
class SourceInstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='midwing source install ')
        self.addCleanup(self.temp.cleanup)
        self.home = Path(self.temp.name).resolve() / 'Home With Spaces'
        self.home.mkdir()
        self.source = Path(self.temp.name) / 'Build With Spaces' / 'Midwing.app'
        self.make_app(self.source, 'new app')
        binary = self.source / 'Contents/MacOS/midwing'
        binary.write_text('#!/bin/sh\nset -eu\n'
                          'printf "%s\\n" "$@" > "$HOME/setup-call"\n'
                          'exit "${MIDWING_TEST_SETUP_EXIT:-0}"\n')
        binary.chmod(0o755)
        self.target = self.home / 'Applications/Midwing.app'

    def make_app(self, app, contents, identifier='com.wails.midwing'):
        (app / 'Contents/MacOS').mkdir(parents=True)
        (app / 'Contents/MacOS/midwing').write_text(contents)
        with (app / 'Contents/Info.plist').open('wb') as stream:
            plistlib.dump({'CFBundleIdentifier': identifier}, stream)

    def install(self, **extra):
        return subprocess.run(['sh', str(ROOT / 'scripts/install.sh'), str(self.source)],
                              env=dict(os.environ, HOME=str(self.home), **extra),
                              capture_output=True, text=True)

    def test_installer_delegates_cli_setup_to_the_copied_app(self):
        # Previously supported directory overrides must not create another policy.
        result = self.install(MIDWING_APP_DIR=str(self.home / 'custom-apps'),
                              MIDWING_BIN_DIR=str(self.home / 'custom-bin'))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.target.is_dir())
        self.assertEqual((self.home / 'setup-call').read_text(), '--setup-cli\n')
        self.assertFalse((self.home / 'custom-apps').exists())
        self.assertFalse((self.home / 'custom-bin').exists())

    def test_installer_does_not_change_cli_links_itself(self):
        link = self.home / '.local/bin/midwing'
        link.parent.mkdir(parents=True)
        link.symlink_to('/unrelated-command')
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(os.readlink(link), '/unrelated-command')

    def test_source_symlink_copies_the_bundle_instead_of_installing_a_symlink(self):
        alias = self.home / 'Source Alias.app'
        alias.symlink_to(self.source, target_is_directory=True)
        self.source = alias
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.target.is_dir())
        self.assertFalse(self.target.is_symlink())
        self.assertEqual((self.home / 'setup-call').read_text(), '--setup-cli\n')

    def test_failed_cli_setup_keeps_the_installed_app_for_retry(self):
        self.make_app(self.target, 'previous app')
        result = self.install(MIDWING_TEST_SETUP_EXIT='1')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.target / 'Contents/MacOS/midwing').read_bytes(),
                         (self.source / 'Contents/MacOS/midwing').read_bytes())
        self.assertIn('Midwing is installed', result.stderr)
        self.assertIn('Retry setup', result.stderr)
        self.assertEqual(list(self.target.parent.glob('.midwing-install.*')), [])

    def test_invalid_source_does_not_replace_the_previous_app(self):
        self.make_app(self.target, 'previous app')
        (self.source / 'Contents/MacOS/midwing').unlink()
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.target / 'Contents/MacOS/midwing').read_text(), 'previous app')
        self.assertFalse((self.home / 'setup-call').exists())

    def test_unrelated_source_is_not_executed(self):
        with (self.source / 'Contents/Info.plist').open('wb') as stream:
            plistlib.dump({'CFBundleIdentifier': 'com.example.unrelated'}, stream)
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.target.exists())
        self.assertFalse((self.home / 'setup-call').exists())

    def test_failed_bundle_move_restores_the_previous_app(self):
        self.make_app(self.target, 'previous app')
        tools = self.home / 'tools'
        tools.mkdir()
        move = tools / 'mv'
        move.write_text('#!/bin/sh\n'
                        'if [ "$2" = "$HOME/Applications/Midwing.app" ] && '
                        '[ "${1##*/}" = "Midwing.app" ]; then exit 1; fi\n'
                        'exec /bin/mv "$@"\n')
        move.chmod(0o755)
        result = self.install(PATH=f'{tools}:{os.environ["PATH"]}')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.target / 'Contents/MacOS/midwing').read_text(), 'previous app')
        self.assertEqual(list(self.target.parent.glob('.midwing-install.*')), [])
        self.assertFalse((self.home / 'setup-call').exists())

    def test_existing_app_symlink_is_preserved(self):
        self.target.parent.mkdir()
        self.target.symlink_to(self.source)
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.target.readlink(), self.source)
        self.assertFalse((self.home / 'setup-call').exists())


if __name__ == '__main__':
    unittest.main()
