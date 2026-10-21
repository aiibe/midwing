"""Exercise release gating without needing an Apple account or private key."""
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parent.parent


class PackageTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        for name in ['scripts/package-macos.sh', 'scripts/prepare-macos-signing.sh']:
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(ROOT / name, target)
        binary = self.root / 'build/bin/midwing.app/Contents/MacOS/midwing'
        binary.parent.mkdir(parents=True)
        binary.write_text('universal app')
        (binary.parent / 'resource-link').symlink_to('midwing')
        tools = self.root / 'tools'
        tools.mkdir()
        for name in ['security', 'lipo', 'codesign', 'ditto', 'hdiutil', 'xcrun', 'spctl']:
            target = tools / name
            shutil.copy2(ROOT / 'scripts/mock_macos_tool.py', target)
            target.chmod(0o755)
        self.log = self.root / 'tools.jsonl'
        self.env = dict(os.environ, PATH=f'{tools}:{os.environ["PATH"]}', TOOL_LOG=str(self.log))
        for name in ['SIGNING_IDENTITY', 'NOTARY_PROFILE', 'NOTARY_KEYCHAIN']:
            self.env.pop(name, None)
        self.archive = self.root / 'build/release/midwing-v0.6.0-macos-universal.dmg'

    def run_package(self, tag='v0.6.0', **env):
        return subprocess.run(['sh', str(self.root / 'scripts/package-macos.sh'), tag],
                              env=dict(self.env, **env), capture_output=True, text=True)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_sign_notarize_and_staple_app_and_dmg(self):
        result = self.run_package(NOTARY_PROFILE='existing-profile', NOTARY_KEYCHAIN='/tmp/test.keychain-db')
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        self.assertIn(['lipo', 'build/bin/midwing.app/Contents/MacOS/midwing',
                       '-verify_arch', 'arm64', 'x86_64'], calls)
        signs = [call for call in calls if call[:2] == ['codesign', '--force']]
        self.assertEqual(len(signs), 2)
        for sign in signs:
            self.assertIn('Developer ID Application: Test (TEAM)', sign)
            self.assertIn('--timestamp', sign)
            self.assertIn('/tmp/test.keychain-db', sign)
        self.assertEqual(signs[0][signs[0].index('--options') + 1], 'runtime')
        self.assertIn('com.wails.midwing.dmg', signs[1])
        submissions = [call for call in calls if call[:3] == ['xcrun', 'notarytool', 'submit']]
        self.assertEqual(len(submissions), 2)
        for submission in submissions:
            self.assertIn('existing-profile', submission)
            self.assertIn('/tmp/test.keychain-db', submission)
            self.assertIn('--wait', submission)
            self.assertFalse(Path(submission[3]).exists(), 'staging was not cleaned up')
        self.assertEqual([call[:3] for call in calls if call[0] == 'xcrun'], 2 * [
            ['xcrun', 'notarytool', 'submit'], ['xcrun', 'stapler', 'staple'], ['xcrun', 'stapler', 'validate']])
        self.assertEqual(calls[-1][:4], ['spctl', '--assess', '--type', 'open'])
        self.assertIn('context:primary-signature', calls[-1])
        create = next(call for call in calls if call[:2] == ['hdiutil', 'create'])
        self.assertIn('UDZO', create)
        self.assertTrue(any(call[:2] == ['hdiutil', 'verify'] for call in calls))
        # The mock image stores its payload as ZIP, including symlink targets.
        with zipfile.ZipFile(self.archive) as archive:
            self.assertEqual(archive.comment, b'stapled')
            self.assertIn('Midwing.app/ticket', archive.namelist())
            self.assertEqual(archive.read('Applications'), b'/Applications')
            self.assertEqual(archive.read('Midwing.app/Contents/MacOS/resource-link'), b'midwing')
            self.assertNotIn('install.sh', archive.namelist())
        checksum = subprocess.run(['shasum', '-a', '256', '-c', self.archive.name + '.sha256'],
                                  cwd=self.archive.parent, capture_output=True, text=True)
        self.assertEqual(checksum.returncode, 0, checksum.stderr)
        self.assertEqual({p.name for p in self.archive.parent.iterdir()},
                         {self.archive.name, self.archive.name + '.sha256'})

    def test_failures_never_publish_and_clean_staging(self):
        failures = [{'FAIL_ARCH': '1'}, {'FAIL_DMG_CREATE': '1'}, {'FAIL_DMG_VERIFY': '1'}]
        for stage in ['APP', 'DMG']:
            failures.extend({f'FAIL_{stage}_{step}': '1'} for step in
                            ['SIGN', 'VERIFY', 'NOTARY', 'STAPLE', 'VALIDATE', 'ASSESS'])
            failures.append({f'{stage}_NOTARY_STATUS': 'Invalid'})
        for failure in failures:
            with self.subTest(failure=failure):
                result = self.run_package(**failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.archive.exists())
                self.assertFalse(Path(str(self.archive) + '.sha256').exists())
                for call in self.calls():
                    if call[:2] == ['codesign', '--force']:
                        self.assertFalse(Path(call[-1]).exists(), 'staging survived failure')
                self.log.unlink()

    def test_ambiguous_or_unavailable_identity_requires_explicit_choice(self):
        for identities in [[], ['Developer ID Application: A', 'Developer ID Application: B']]:
            with self.subTest(identities=identities):
                result = self.run_package(IDENTITIES=json.dumps(identities))
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('Set SIGNING_IDENTITY explicitly', result.stderr)
                self.assertFalse(self.archive.exists())

    def test_explicit_identity_and_default_profile(self):
        result = self.run_package(SIGNING_IDENTITY='Developer ID Application: Chosen', IDENTITIES='[]')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(call[0] == 'security' for call in self.calls()))
        self.assertTrue(any('midwing-notary' in call for call in self.calls()))

    def test_reject_development_and_ad_hoc_signatures(self):
        for identity in ['-', 'Apple Development: Test (TEAM)', 'Developer ID Installer: Test (TEAM)']:
            result = self.run_package(SIGNING_IDENTITY=identity)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Developer ID', result.stderr)
            self.assertEqual(self.calls(), [])

    def run_prepare(self, **env):
        values = dict(self.env, RUNNER_TEMP=str(self.root),
                      MACOS_CERTIFICATE_P12=base64.b64encode(b'test certificate').decode(),
                      MACOS_CERTIFICATE_PASSWORD='test export password',
                      SIGNING_IDENTITY='Developer ID Application: Test (TEAM)',
                      APPLE_ID='test@example.com', APPLE_TEAM_ID='TEAM',
                      APPLE_APP_PASSWORD='test app password')
        return subprocess.run(['sh', str(self.root / 'scripts/prepare-macos-signing.sh')],
                              env=dict(values, **env), capture_output=True, text=True)

    def test_prepare_uses_temporary_keychain_and_removes_certificate(self):
        result = self.run_prepare()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(list(self.root.glob('midwing-certificates.*')), [])
        calls = self.calls()
        self.assertTrue(any(call[:2] == ['security', 'create-keychain'] for call in calls))
        self.assertTrue(any(call[:2] == ['security', 'set-key-partition-list'] for call in calls))
        imports = [call for call in calls if call[:2] == ['security', 'import']]
        self.assertEqual(len(imports), 1)
        self.assertIn('/usr/bin/codesign', imports[0])
        self.assertIn('pkcs12', imports[0])
        self.assertTrue(imports[0][2].endswith('.p12'))
        store = next(call for call in calls if call[:3] == ['xcrun', 'notarytool', 'store-credentials'])
        self.assertEqual(store[3], 'midwing-notary')
        self.assertEqual(store[store.index('--keychain') + 1], str(self.root / 'midwing-signing.keychain-db'))

    def test_prepare_rejects_missing_credentials_and_invalid_certificate(self):
        for values in [{'APPLE_APP_PASSWORD': ''}, {'MACOS_CERTIFICATE_P12': 'invalid base64'},
                       {'MACOS_CERTIFICATE_PASSWORD': ''}]:
            with self.subTest(values=values):
                result = self.run_prepare(**values)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.calls(), [])
                self.assertEqual(list(self.root.glob('midwing-certificates.*')), [])

    def test_invalid_tag_fails_before_signing(self):
        result = self.run_package(tag='v0.6.0;false')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls(), [])


if __name__ == '__main__':
    unittest.main()
