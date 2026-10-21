"""Verify CI certificate preparation against the native macOS importer."""
import base64
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent.parent


@unittest.skipUnless(sys.platform == 'darwin', 'requires the macOS Keychain')
class NativeSigningTests(unittest.TestCase):
    def test_prepare_imports_real_pkcs12_identities(self):
        with tempfile.TemporaryDirectory(prefix='midwing-signing-test-') as directory:
            root = Path(directory)
            keychain = root / 'midwing-signing.keychain-db'
            tools = root / 'tools'
            tools.mkdir()
            # Only notarization needs an Apple account; exercise the real importer.
            notary = tools / 'xcrun'
            notary.write_text('#!/bin/sh\nexit 0\n')
            notary.chmod(0o755)
            self.run_tool([
                'openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes',
                '-keyout', str(root / 'key.pem'), '-out', str(root / 'cert.pem'),
                '-days', '1', '-subj', '/CN=Midwing signing regression test',
            ])
            self.run_tool([
                'openssl', 'pkcs12', '-export', '-inkey', str(root / 'key.pem'),
                '-in', str(root / 'cert.pem'), '-out', str(root / 'identity.p12'),
                '-keypbe', 'PBE-SHA1-3DES', '-certpbe', 'PBE-SHA1-3DES',
                '-macalg', 'sha1', '-passout', 'pass:test-export-password',
            ])
            certificate = base64.b64encode((root / 'identity.p12').read_bytes()).decode()
            env = dict(
                os.environ, PATH=f'{tools}:{os.environ["PATH"]}',
                RUNNER_TEMP=str(root), MACOS_CERTIFICATE_P12=certificate,
                MACOS_INSTALLER_CERTIFICATE_P12=certificate,
                MACOS_CERTIFICATE_PASSWORD='test-export-password',
                MACOS_INSTALLER_CERTIFICATE_PASSWORD='test-export-password',
                SIGNING_IDENTITY='Developer ID Application: Test',
                INSTALLER_SIGNING_IDENTITY='Developer ID Installer: Test',
                APPLE_ID='test@example.com', APPLE_TEAM_ID='TEST',
                APPLE_APP_PASSWORD='test-notary-password',
            )
            try:
                self.run_tool(['sh', str(ROOT / 'scripts/prepare-macos-signing.sh')], env=env)
                identities = self.run_tool(['security', 'find-identity', str(keychain)])
                self.assertIn('Midwing signing regression test', identities.stdout)
                self.assertEqual(list(root.glob('midwing-certificates.*')), [])
            finally:
                if keychain.exists():
                    self.run_tool(['security', 'delete-keychain', str(keychain)])

    def run_tool(self, args, **kwargs):
        result = subprocess.run(args, capture_output=True, text=True, **kwargs)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result


if __name__ == '__main__':
    unittest.main()
