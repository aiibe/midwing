"""Check installer conflicts and real macOS package payloads without installing."""
from pathlib import Path
import plistlib
import shutil
import subprocess
import tempfile
import unittest
import xml.etree.ElementTree as ET


INSTALLER = Path(__file__).resolve().parent / 'macos-installer'
CLI_TARGET = '/Applications/Midwing.app/Contents/MacOS/midwing'


class InstallerTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name) / 'volume'
        self.app = self.root / 'Applications/Midwing.app'
        self.command = self.root / 'usr/local/bin/midwing'
        self.command.parent.mkdir(parents=True)

    def preinstall(self):
        return subprocess.run(['sh', str(INSTALLER / 'hooks/preinstall'), 'package.pkg', '/', str(self.root)],
                              capture_output=True, text=True)

    def make_app(self, identifier='com.wails.midwing'):
        contents = self.app / 'Contents'
        (contents / 'MacOS').mkdir(parents=True)
        executable = contents / 'MacOS/midwing'
        executable.write_text('#!/bin/sh\nexit 0\n')
        executable.chmod(0o755)
        (contents / 'Info.plist').write_bytes(plistlib.dumps({
            'CFBundleIdentifier': identifier, 'CFBundleExecutable': 'midwing',
            'CFBundlePackageType': 'APPL', 'CFBundleVersion': '0.5.0',
            'CFBundleShortVersionString': '0.5.0', 'LSMinimumSystemVersion': '13.0',
        }))

    def test_fresh_install_and_existing_cli_link(self):
        self.assertEqual(self.preinstall().returncode, 0)
        self.command.symlink_to(CLI_TARGET)
        self.assertEqual(self.preinstall().returncode, 0)
        self.assertEqual(self.command.readlink(), Path(CLI_TARGET))

    @unittest.skipUnless(Path('/usr/libexec/PlistBuddy').exists(), 'Requires macOS')
    def test_existing_app_can_be_updated(self):
        self.make_app()
        self.command.symlink_to(CLI_TARGET)
        result = self.preinstall()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(self.app.exists())

    def test_unrelated_command_or_symlink_is_preserved(self):
        for target in [None, '/unrelated', '../unrelated']:
            with self.subTest(target=target):
                if target is None:
                    self.command.write_text('unrelated command')
                else:
                    self.command.symlink_to(target)
                result = self.preinstall()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('unrelated command', result.stderr)
                if target is None:
                    self.assertEqual(self.command.read_text(), 'unrelated command')
                else:
                    self.assertEqual(str(self.command.readlink()), target)
                self.command.unlink()

    def test_unrelated_app_is_preserved(self):
        self.make_app('org.example.unrelated')
        result = self.preinstall()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('unrelated app', result.stderr)
        self.assertTrue(self.app.exists())

    def test_app_symlink_is_preserved(self):
        self.app.parent.mkdir()
        self.app.symlink_to('/unrelated')
        result = self.preinstall()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('symlink', result.stderr)
        self.assertEqual(str(self.app.readlink()), '/unrelated')

    @unittest.skipUnless(all(shutil.which(tool) for tool in ['pkgbuild', 'productbuild', 'pkgutil']),
                         'Requires macOS packaging tools')
    def test_native_package_contains_app_cli_and_preinstall(self):
        self.make_app()
        self.command.symlink_to(CLI_TARGET)
        output = self.root.parent / 'packages'
        output.mkdir()
        component = output / 'midwing-component.pkg'
        package = output / 'Midwing.pkg'
        commands = [
            ['pkgbuild', '--root', str(self.root),
             '--identifier', 'com.wails.midwing.pkg', '--version', '0.5.0',
             '--install-location', '/', '--ownership', 'recommended',
             '--component-plist', str(INSTALLER / 'components.plist'),
             '--scripts', str(INSTALLER / 'hooks'), str(component)],
            ['productbuild', '--distribution', str(INSTALLER / 'Distribution.xml'),
             '--package-path', str(output), str(package)],
            ['pkgutil', '--expand-full', str(package), str(output / 'expanded')],
        ]
        for command in commands:
            result = subprocess.run(command, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        expanded = output / 'expanded/midwing-component.pkg'
        self.assertTrue((expanded / 'Payload/Applications/Midwing.app/Contents/MacOS/midwing').exists())
        self.assertEqual(str((expanded / 'Payload/usr/local/bin/midwing').readlink()), CLI_TARGET)
        hook = expanded / 'Scripts/preinstall'
        self.assertEqual(hook.read_bytes(), (INSTALLER / 'hooks/preinstall').read_bytes())
        self.assertTrue(hook.stat().st_mode & 0o111)
        info = ET.parse(expanded / 'PackageInfo').getroot()
        self.assertEqual(info.get('version'), '0.5.0')
        bundle = info.find('bundle')
        self.assertEqual(bundle.get('path'), './Applications/Midwing.app')
        self.assertEqual(info.get('relocatable'), 'false')
        self.assertEqual(info.find('upgrade-bundle/bundle').get('id'), 'com.wails.midwing')
        self.assertEqual(info.find('strict-identifier/bundle').get('id'), 'com.wails.midwing')
        distribution = ET.parse(output / 'expanded/Distribution').getroot()
        self.assertEqual(distribution.find('volume-check/allowed-os-versions/os-version').get('min'), '13.0')
        self.assertEqual(distribution.find('domains').get('enable_currentUserHome'), 'false')
        self.assertEqual(distribution.find('pkg-ref/must-close/app').get('id'), 'com.wails.midwing')


if __name__ == '__main__':
    unittest.main()
