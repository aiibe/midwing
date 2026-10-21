#!/usr/bin/env python3
"""Native-tool stand-ins used only by the packaging failure tests."""
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
image = args[-1].endswith('.dmg') if args else False
stage = 'DMG' if image else 'APP'
if name == 'security' and args[:1] == ['find-identity']:
    for i, identity in enumerate(json.loads(os.environ.get('IDENTITIES',
            '["Developer ID Application: Test (TEAM)"]'))):
        print(f'  {i + 1}) ABCDEF "{identity}"')
if name == 'codesign':
    if '-R' in args and not args[args.index('-R') + 1].startswith('='):
        sys.exit('A literal code requirement must start with =')
    operation = 'SIGN' if '--force' in args else 'VERIFY'
    if os.environ.get(f'FAIL_{stage}_{operation}'):
        sys.exit(f'{stage} {operation} failed')
if name == 'lipo' and os.environ.get('FAIL_ARCH'):
    sys.exit('Missing universal architecture')
if name == 'xcrun' and args[:2] == ['notarytool', 'submit']:
    stage = 'DMG' if args[2].endswith('.dmg') else 'APP'
    if os.environ.get(f'FAIL_{stage}_NOTARY'):
        sys.exit('Notary service unavailable')
    print(json.dumps({'status': os.environ.get(f'{stage}_NOTARY_STATUS', 'Accepted'), 'id': 'test-submission'}))
if name == 'xcrun' and args[:1] == ['stapler']:
    operation = args[1].upper()
    if os.environ.get(f'FAIL_{stage}_{operation}'):
        sys.exit('Stapling failed')
    if operation == 'STAPLE':
        if image:
            with zipfile.ZipFile(args[2], 'a') as archive:
                archive.comment = b'stapled'
        else:
            (Path(args[2]) / 'ticket').write_text('stapled')
if name == 'spctl' and os.environ.get(f'FAIL_{stage}_ASSESS'):
    sys.exit('Gatekeeper rejected the artifact')
if name == 'hdiutil' and os.environ.get(f'FAIL_DMG_{args[0].upper()}'):
    sys.exit('Disk image operation failed')
if name == 'ditto' and len(args) == 2:
    shutil.copytree(args[0], args[1], symlinks=True)
elif name == 'ditto' or (name == 'hdiutil' and args[0] == 'create'):
    source = Path(args[args.index('-srcfolder') + 1] if name == 'hdiutil' else args[-2])
    with zipfile.ZipFile(args[-1], 'w') as archive:
        base = source.parent if '--keepParent' in args else source
        for path in source.rglob('*'):
            if path.is_symlink():
                archive.writestr(str(path.relative_to(base)), os.readlink(path))
            elif path.is_file():
                archive.write(path, path.relative_to(base))
