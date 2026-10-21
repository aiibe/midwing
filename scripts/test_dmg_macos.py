"""Inspect a real read-only UDIF image without Apple credentials or installation."""
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile
import unittest


@unittest.skipUnless(sys.platform == 'darwin', 'requires native macOS disk images')
class NativeDMGTests(unittest.TestCase):
    def run_tool(self, *args):
        result = subprocess.run(args, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout

    def test_image_preserves_universal_signed_app_and_shortcut(self):
        with tempfile.TemporaryDirectory(prefix='midwing-dmg-test-') as directory:
            root = Path(directory)
            payload = root / 'payload'
            app = payload / 'Midwing.app'
            binary = app / 'Contents/MacOS/midwing'
            binary.parent.mkdir(parents=True)
            source = root / 'main.c'
            source.write_text('int main(void) { return 0; }\n')
            self.run_tool('clang', '-arch', 'arm64', '-arch', 'x86_64',
                          '-mmacosx-version-min=13.0', str(source), '-o', str(binary))
            (app / 'Contents/Info.plist').write_bytes(plistlib.dumps({
                'CFBundleIdentifier': 'com.wails.midwing', 'CFBundleExecutable': 'midwing',
                'CFBundlePackageType': 'APPL', 'LSMinimumSystemVersion': '13.0',
            }))
            (app / 'Contents/resource-link').symlink_to('MacOS/midwing')
            (payload / 'Applications').symlink_to('/Applications')
            # Ad hoc signing tests preservation; release trust is verified by packaging.
            self.run_tool('codesign', '--force', '--sign', '-', str(app))
            image = root / 'midwing.dmg'
            self.run_tool('hdiutil', 'create', '-volname', 'Midwing', '-srcfolder', str(payload),
                          '-format', 'UDZO', str(image))
            self.run_tool('hdiutil', 'verify', str(image))
            mount = root / 'mounted'
            self.run_tool('hdiutil', 'attach', '-readonly', '-nobrowse', '-mountpoint', str(mount), str(image))
            try:
                self.assertEqual({p.name for p in mount.iterdir() if not p.name.startswith('.')},
                                 {'Midwing.app', 'Applications'})
                self.assertEqual((mount / 'Applications').readlink(), Path('/Applications'))
                copied = root / 'copied/Midwing.app'
                copied.parent.mkdir()
                self.run_tool('ditto', str(mount / 'Midwing.app'), str(copied))
                self.run_tool('codesign', '--verify', '--deep', '--strict', str(copied))
                self.run_tool('lipo', str(copied / 'Contents/MacOS/midwing'), '-verify_arch', 'arm64', 'x86_64')
                self.assertEqual((copied / 'Contents/resource-link').readlink(), Path('MacOS/midwing'))
                with (copied / 'Contents/Info.plist').open('rb') as stream:
                    self.assertEqual(plistlib.load(stream)['LSMinimumSystemVersion'], '13.0')
            finally:
                self.run_tool('hdiutil', 'detach', str(mount))


if __name__ == '__main__':
    unittest.main()
