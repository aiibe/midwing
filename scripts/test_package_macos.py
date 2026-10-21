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
MOCK_TOOL = r'''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import shutil
import sys
import zipfile

name = Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ['TOOL_LOG'], 'a') as log:
    log.write(json.dumps([name, *args]) + '\n')
if name == 'security' and args[:1] == ['find-identity']:
    identities = json.loads(os.environ.get('IDENTITIES',
        '["Developer ID Application: Test (TEAM)", "Developer ID Installer: Test (TEAM)"]'))
    for i, identity in enumerate(identities):
        print(f'  {i + 1}) ABCDEF "{identity}"')
if name == 'codesign' and '-R' in args and not args[args.index('-R') + 1].startswith('='):
    sys.exit('A literal code requirement must start with =')
package = args[-1].endswith('.pkg')
if name == 'codesign' and '--force' in args and os.environ.get('FAIL_SIGN'):
    sys.exit('Signing failed')
if name == 'pkgbuild' and os.environ.get('FAIL_BUILD'):
    sys.exit('Package build failed')
if name == 'productbuild':
    if os.environ.get('FAIL_PACKAGE_SIGN'):
        sys.exit('Package signing failed')
    shutil.copyfile(Path(args[args.index('--package-path') + 1]) / 'midwing-component.pkg', args[-1])
if name == 'pkgutil' and os.environ.get('FAIL_PACKAGE_VERIFY'):
    sys.exit('Package signature invalid')
if name == 'xcrun' and args[:2] == ['notarytool', 'submit']:
    package = args[2].endswith('.pkg')
    if os.environ.get('FAIL_PACKAGE_NOTARY' if package else 'FAIL_NOTARY'):
        sys.exit('Notary service unavailable')
    status = os.environ.get('PACKAGE_NOTARY_STATUS' if package else 'NOTARY_STATUS', 'Accepted')
    print(json.dumps({'status': status, 'id': 'test-submission'}))
if name == 'xcrun' and args[:2] == ['stapler', 'staple']:
    if os.environ.get('FAIL_PACKAGE_STAPLE' if package else 'FAIL_STAPLE'):
        sys.exit('Stapling failed')
    if package:
        with zipfile.ZipFile(args[2], 'a') as archive:
            archive.comment = b'stapled'
    else:
        (Path(args[2]) / 'ticket').write_text('stapled')
if name == 'spctl' and os.environ.get('FAIL_PACKAGE_ASSESS' if package else 'FAIL_ASSESS'):
    sys.exit('Gatekeeper rejected the app')
if name in ['ditto', 'pkgbuild']:
    source = Path(args[args.index('--root') + 1] if name == 'pkgbuild' else args[-2])
    target = Path(args[-1])
    with zipfile.ZipFile(target, 'w') as archive:
        base = source.parent if '--keepParent' in args else source
        for path in source.rglob('*'):
            if path.is_symlink():
                archive.writestr(str(path.relative_to(base)), os.readlink(path))
            elif path.is_file():
                archive.write(path, path.relative_to(base))
'''


class PackageTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        for name in ['scripts/package-macos.sh', 'scripts/prepare-macos-signing.sh']:
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(ROOT / name, target)
        shutil.copytree(ROOT / 'scripts/macos-installer', self.root / 'scripts/macos-installer')
        binary = self.root / 'build/bin/midwing.app/Contents/MacOS/midwing'
        binary.parent.mkdir(parents=True)
        binary.write_text('universal app')
        tools = self.root / 'tools'
        tools.mkdir()
        for name in ['security', 'lipo', 'codesign', 'ditto', 'pkgbuild', 'productbuild', 'pkgutil', 'xcrun', 'spctl']:
            target = tools / name
            target.write_text(MOCK_TOOL)
            target.chmod(0o755)
        self.log = self.root / 'tools.jsonl'
        self.env = dict(os.environ, PATH=f'{tools}:{os.environ["PATH"]}', TOOL_LOG=str(self.log))
        for name in ['SIGNING_IDENTITY', 'INSTALLER_SIGNING_IDENTITY', 'NOTARY_PROFILE', 'NOTARY_KEYCHAIN']:
            self.env.pop(name, None)
        self.archive = self.root / 'build/release/midwing-v0.5.0-macos-universal.pkg'

    def run_package(self, tag='v0.5.0', **env):
        return subprocess.run(['sh', str(self.root / 'scripts/package-macos.sh'), tag],
                              env=dict(self.env, **env), capture_output=True, text=True)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_sign_notarize_and_staple_both_app_and_package(self):
        result = self.run_package(NOTARY_PROFILE='existing-profile', NOTARY_KEYCHAIN='/tmp/test.keychain-db')
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        signs = [call for call in calls if call[:2] == ['codesign', '--force']]
        self.assertEqual(len(signs), 1)
        sign = signs[0]
        self.assertIn('Developer ID Application: Test (TEAM)', sign)
        self.assertIn('--timestamp', sign)
        self.assertEqual(sign[sign.index('--options') + 1], 'runtime')
        self.assertEqual(sign[sign.index('--keychain') + 1], '/tmp/test.keychain-db')
        submissions = [call for call in calls if call[:3] == ['xcrun', 'notarytool', 'submit']]
        self.assertEqual(len(submissions), 2)
        submission = submissions[0]
        self.assertEqual(submission[submission.index('--keychain-profile') + 1], 'existing-profile')
        self.assertIn('--wait', submission)
        self.assertIn('/tmp/test.keychain-db', submission)
        self.assertEqual([call[:3] for call in calls if call[0] == 'xcrun'], 2 * [
            ['xcrun', 'notarytool', 'submit'], ['xcrun', 'stapler', 'staple'], ['xcrun', 'stapler', 'validate']])
        package_sign = next(call for call in calls if call[0] == 'productbuild')
        self.assertIn('Developer ID Installer: Test (TEAM)', package_sign)
        self.assertIn('--timestamp', package_sign)
        self.assertIn('/tmp/test.keychain-db', package_sign)
        self.assertEqual(submissions[1][3], package_sign[-1])
        self.assertIn('existing-profile', submissions[1])
        self.assertTrue(any(call[:2] == ['pkgutil', '--check-signature'] for call in calls))
        self.assertEqual(calls[-1][:4], ['spctl', '--assess', '--type', 'install'])
        build = next(call for call in calls if call[0] == 'pkgbuild')
        self.assertEqual(build[build.index('--version') + 1], '0.5.0')
        self.assertEqual(build[build.index('--install-location') + 1], '/')
        # The fake pkgbuild stores the payload in ZIP format for inspection.
        with zipfile.ZipFile(self.archive) as archive:
            self.assertEqual(archive.comment, b'stapled')
            self.assertIn('Applications/Midwing.app/ticket', archive.namelist())
            self.assertEqual(archive.read('usr/local/bin/midwing'),
                             b'/Applications/Midwing.app/Contents/MacOS/midwing')
            self.assertNotIn('install.sh', archive.namelist())
        checksum = subprocess.run(['shasum', '-a', '256', '-c', self.archive.name + '.sha256'],
                                  cwd=self.archive.parent, capture_output=True, text=True)
        self.assertEqual(checksum.returncode, 0, checksum.stderr)
        self.assertFalse(Path(submission[3]).exists(), 'temporary submission was not cleaned up')

    def test_failure_never_produces_a_release_archive(self):
        for failure in [{'FAIL_SIGN': '1'}, {'FAIL_NOTARY': '1'}, {'NOTARY_STATUS': 'Invalid'},
                        {'FAIL_STAPLE': '1'}, {'FAIL_ASSESS': '1'}, {'FAIL_BUILD': '1'},
                        {'FAIL_PACKAGE_SIGN': '1'}, {'FAIL_PACKAGE_VERIFY': '1'},
                        {'FAIL_PACKAGE_NOTARY': '1'}, {'PACKAGE_NOTARY_STATUS': 'Invalid'},
                        {'FAIL_PACKAGE_STAPLE': '1'}, {'FAIL_PACKAGE_ASSESS': '1'}]:
            with self.subTest(failure=failure):
                result = self.run_package(**failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.archive.exists())
                self.assertFalse(self.archive.with_suffix('.pkg.sha256').exists())
                self.log.unlink()

    def test_ambiguous_or_unavailable_identity_requires_explicit_choice(self):
        for identities, variable in [([], 'SIGNING_IDENTITY'),
                (['Developer ID Application: A', 'Developer ID Application: B'], 'SIGNING_IDENTITY'),
                (['Developer ID Application: A'], 'INSTALLER_SIGNING_IDENTITY'),
                (['Developer ID Application: A', 'Developer ID Installer: A', 'Developer ID Installer: B'],
                 'INSTALLER_SIGNING_IDENTITY')]:
            with self.subTest(identities=identities):
                result = self.run_package(IDENTITIES=json.dumps(identities))
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(f'Set {variable} explicitly', result.stderr)
                self.assertFalse(self.archive.exists())

    def test_explicit_identity_and_default_profile(self):
        result = self.run_package(SIGNING_IDENTITY='Developer ID Application: Chosen',
                                  INSTALLER_SIGNING_IDENTITY='Developer ID Installer: Chosen', IDENTITIES='[]')
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        self.assertFalse(any(call[0] == 'security' for call in calls))
        self.assertTrue(any('Developer ID Application: Chosen' in call for call in calls))
        self.assertTrue(any('midwing-notary' in call for call in calls))

    def test_reject_development_and_ad_hoc_signatures(self):
        for variable, identity in [('SIGNING_IDENTITY', '-'), ('SIGNING_IDENTITY', 'Apple Development: Test (TEAM)'),
                                   ('INSTALLER_SIGNING_IDENTITY', 'Developer ID Application: Test (TEAM)')]:
            with self.subTest(variable=variable, identity=identity):
                values = dict(SIGNING_IDENTITY='Developer ID Application: Test (TEAM)',
                              INSTALLER_SIGNING_IDENTITY='Developer ID Installer: Test (TEAM)')
                values[variable] = identity
                result = self.run_package(**values)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('Developer ID', result.stderr)
                self.assertEqual(self.calls(), [])

    def run_prepare(self, **env):
        values = dict(self.env, RUNNER_TEMP=str(self.root),
                      MACOS_CERTIFICATE_P12=base64.b64encode(b'test certificate').decode(),
                      MACOS_CERTIFICATE_PASSWORD='test export password',
                      MACOS_INSTALLER_CERTIFICATE_P12=base64.b64encode(b'test installer certificate').decode(),
                      MACOS_INSTALLER_CERTIFICATE_PASSWORD='test installer export password',
                      SIGNING_IDENTITY='Developer ID Application: Test (TEAM)',
                      INSTALLER_SIGNING_IDENTITY='Developer ID Installer: Test (TEAM)',
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
        self.assertEqual(len(imports), 2)
        self.assertIn('/usr/bin/codesign', imports[0])
        self.assertIn('/usr/bin/productbuild', imports[1])
        store = next(call for call in calls if call[:3] == ['xcrun', 'notarytool', 'store-credentials'])
        self.assertEqual(store[3], 'midwing-notary')
        self.assertEqual(store[store.index('--keychain') + 1], str(self.root / 'midwing-signing.keychain-db'))

    def test_prepare_rejects_missing_credentials_and_invalid_certificate(self):
        for values in [{'APPLE_APP_PASSWORD': ''}, {'MACOS_CERTIFICATE_P12': 'invalid base64'},
                       {'MACOS_INSTALLER_CERTIFICATE_P12': 'invalid base64'},
                       {'MACOS_INSTALLER_CERTIFICATE_PASSWORD': ''}]:
            with self.subTest(values=values):
                result = self.run_prepare(**values)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.calls(), [])
                self.assertEqual(list(self.root.glob('midwing-certificates.*')), [])

    def test_invalid_tag_fails_before_signing(self):
        result = self.run_package(tag='v0.5.0;false')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls(), [])


if __name__ == '__main__':
    unittest.main()
