#!/usr/bin/env python3
"""Exercise release preparation against local Git repositories and a fake GitHub CLI."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("prepare.py").resolve()
FAKE_GH = '''#!/usr/bin/env python3
import json, os, pathlib, sys
args = sys.argv[1:]
state = json.loads(pathlib.Path(os.environ['FAKE_GH_STATE']).read_text())
record = {'args': args}
if '--body-file' in args:
    record['body'] = pathlib.Path(args[args.index('--body-file') + 1]).read_text()
with open(os.environ['FAKE_GH_LOG'], 'a') as log:
    log.write(json.dumps(record) + '\\n')
if args[:2] == ['repo', 'view']:
    print('test/repo')
elif args == ['api', 'repos/test/repo']:
    print(json.dumps({'permissions': {'push': True}}))
elif args[:3] == ['api', '--paginate', '--slurp']:
    print(json.dumps([state.get('releases', [])]))
elif args[:2] == ['pr', 'list']:
    print(json.dumps(state.get('prs', [])))
elif args[:2] == ['pr', 'create']:
    print('https://github.com/test/repo/pull/1')
else:
    raise SystemExit('unexpected gh command: ' + str(args))
'''


class PreparationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'repo'
        self.remote = self.root / 'remote.git'
        subprocess.run(['git', 'init', '--bare', '-q', str(self.remote)], check=True)
        subprocess.run(['git', 'init', '-q', '-b', 'master', str(self.repo)], check=True)
        for key, value in (('user.name', 'Test'), ('user.email', 'test@example.com'), ('commit.gpgsign', 'false'), ('tag.gpgSign', 'false'), ('push.gpgSign', 'false'), ('core.hooksPath', '/dev/null')):
            self.git('config', key, value)
        notes = self.repo / 'ci/release/changelogs'
        notes.mkdir(parents=True)
        (notes / 'next.md').write_text('Upcoming changes\n')
        (notes / 'template.md').write_text('Next release\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'initial')
        self.git('remote', 'add', 'origin', str(self.remote))
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        fake = self.bin / 'gh'
        fake.write_text(FAKE_GH)
        fake.chmod(0o755)
        self.state = self.root / 'state.json'
        self.state.write_text('{}')
        self.log = self.root / 'calls.jsonl'
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'], FAKE_GH_STATE=str(self.state), FAKE_GH_LOG=str(self.log))
        self.env.pop('GITHUB_ACTIONS', None)
        self.env.pop('PUBLISH', None)

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.repo, text=True, stderr=subprocess.DEVNULL).strip()

    def prepare(self, *flags, success=True):
        result = subprocess.run([sys.executable, str(SCRIPT), '--version=v1.2.3', *flags], cwd=self.repo, env=self.env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
        return result.stdout

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_prepares_tag_and_pr_without_building_or_uploading(self):
        self.prepare()
        self.assertEqual(self.git('branch', '--show-current'), 'v1.2.3')
        self.assertEqual(self.git('rev-parse', 'HEAD'), self.git('rev-parse', 'refs/tags/v1.2.3^{commit}'))
        remote_tag = subprocess.check_output(['git', '--git-dir', str(self.remote), 'rev-parse', 'refs/tags/v1.2.3^{commit}'], text=True).strip()
        self.assertEqual(remote_tag, self.git('rev-parse', 'HEAD'))
        created = [call for call in self.calls() if call['args'][:2] == ['pr', 'create']]
        self.assertEqual(len(created), 1)
        self.assertTrue(created[0]['body'].startswith('## Human\n\n---\n\n## AI\n'))
        self.assertFalse(any(call['args'][0] in ('release', 'workflow', 'run') for call in self.calls()))

    def test_existing_pr_description_is_untouched(self):
        self.state.write_text(json.dumps({'prs': [{'url': 'https://github.com/test/repo/pull/9', 'state': 'OPEN'}]}))
        self.prepare()
        self.assertFalse(any(call['args'][:2] in (['pr', 'edit'], ['pr', 'create']) for call in self.calls()))

    def test_completed_release_stops_before_git_mutation(self):
        self.state.write_text(json.dumps({'releases': [{'tag_name': 'v1.2.3', 'draft': True, 'assets': [{'name': 'd2-v1.2.3-windows-amd64.msi'}]}]}))
        before = self.git('rev-parse', 'HEAD')
        self.assertIn('already has its MSI', self.prepare(success=False))
        self.assertEqual(self.git('rev-parse', 'HEAD'), before)
        self.assertEqual(self.git('tag'), '')

    def test_dry_run_does_not_change_git_or_github(self):
        before = self.git('rev-parse', 'HEAD')
        self.prepare('--dry-run')
        self.assertEqual(self.git('rev-parse', 'HEAD'), before)
        self.assertEqual(self.git('status', '--porcelain'), '')
        self.assertEqual(self.git('tag'), '')
        self.assertFalse(any(call['args'][:2] == ['pr', 'create'] for call in self.calls()))

    def test_removed_flags_are_rejected_before_any_action(self):
        for flag in ('--skip-build', '--publish', '--rebuild'):
            with self.subTest(flag=flag):
                self.prepare(flag, success=False)
                self.assertEqual(self.calls(), [])

    def test_stable_prerelease_flag_is_rejected(self):
        self.assertIn('v0.9.0-rc.1', self.prepare('--prerelease', success=False))
        self.assertEqual(self.calls(), [])


if __name__ == '__main__':
    unittest.main()
