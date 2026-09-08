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
    if state.get('fail_create'):
        raise SystemExit('simulated PR creation failure')
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

    def test_existing_draft_without_remote_tag_is_not_reused(self):
        self.state.write_text(json.dumps({'releases': [{'tag_name': 'v1.2.3', 'draft': True, 'assets': []}]}))
        before = self.refs()
        self.assertIn('do not reuse this version', self.prepare(success=False))
        self.assertEqual(self.refs(), before)

    def test_published_tagged_release_is_rejected_without_changes(self):
        self.prepare()
        self.state.write_text(json.dumps({'releases': [{'tag_name': 'v1.2.3', 'draft': False, 'assets': []}]}))
        before = self.refs()
        self.assertIn('use a new version', self.prepare(success=False))
        self.assertEqual(self.refs(), before)

    def test_dry_run_does_not_change_git_or_github(self):
        before = self.git('rev-parse', 'HEAD')
        self.prepare('--dry-run')
        self.assertEqual(self.git('rev-parse', 'HEAD'), before)
        self.assertEqual(self.git('status', '--porcelain'), '')
        self.assertEqual(self.git('tag'), '')
        self.assertFalse(any(call['args'][:2] == ['pr', 'create'] for call in self.calls()))

    def refs(self):
        local = self.git('for-each-ref', '--format=%(refname) %(objectname)')
        remote = subprocess.check_output(['git', '--git-dir', str(self.remote), 'for-each-ref', '--format=%(refname) %(objectname)'], text=True).strip()
        return local, remote, self.git('branch', '--show-current')

    def remote_git(self, *args):
        return subprocess.check_output(['git', '--git-dir', str(self.remote), *args], text=True, stderr=subprocess.DEVNULL).strip()

    def add_commit(self, name):
        (self.repo / name).write_text(name)
        self.git('add', '--', name)
        self.git('commit', '-qm', name)
        return self.git('rev-parse', 'HEAD')

    def assert_rejected_without_ref_changes(self):
        before = self.refs()
        calls = len(self.calls())
        self.assertIn('choose a new version', self.prepare(success=False))
        self.assertEqual(self.refs(), before)
        self.assertFalse(any(call['args'][:2] in (['pr', 'create'], ['pr', 'edit']) for call in self.calls()[calls:]))

    def test_existing_matching_tag_is_a_noop_even_from_master(self):
        self.prepare()
        self.git('checkout', '-q', 'master')
        before = self.refs()
        calls = len(self.calls())
        self.assertIn('already tagged', self.prepare())
        self.assertEqual(self.refs(), before)
        self.assertFalse(any(call['args'][0] == 'pr' for call in self.calls()[calls:]))

    def test_existing_remote_tag_does_not_recreate_missing_local_tag(self):
        self.prepare()
        self.git('tag', '-d', 'v1.2.3')
        before = self.refs()
        self.assertIn('already tagged', self.prepare())
        self.assertEqual(self.refs(), before)

    def test_changed_source_after_tag_requires_new_version(self):
        self.prepare()
        self.add_commit('new-source')
        self.assert_rejected_without_ref_changes()

    def test_conflicting_tag_objects_at_same_commit_are_rejected(self):
        self.prepare()
        self.git('tag', '-f', '-a', 'v1.2.3', '-m', 'different annotation')
        self.assert_rejected_without_ref_changes()

    def test_conflicting_remote_tag_commit_is_rejected(self):
        self.prepare()
        self.remote_git('update-ref', 'refs/tags/v1.2.3', self.git('rev-parse', 'HEAD^'))
        self.assert_rejected_without_ref_changes()

    def test_partial_preparation_retry_does_not_create_another_commit(self):
        self.state.write_text(json.dumps({'fail_create': True}))
        self.prepare(success=False)
        before = self.git('rev-parse', 'HEAD')
        self.assertEqual(self.git('tag'), '')
        self.assertEqual(self.remote_git('rev-parse', 'refs/heads/v1.2.3'), before)
        self.state.write_text('{}')
        self.prepare()
        self.assertEqual(self.git('rev-parse', 'HEAD'), before)
        self.assertEqual(self.remote_git('rev-parse', 'refs/tags/v1.2.3^{commit}'), before)

    def test_partial_remote_branch_can_resume_without_new_commit(self):
        self.state.write_text(json.dumps({'fail_create': True}))
        self.prepare(success=False)
        commit = self.git('rev-parse', 'HEAD')
        self.git('checkout', '-q', 'master')
        self.git('branch', '-D', 'v1.2.3')
        self.state.write_text('{}')
        self.prepare()
        self.assertEqual(self.git('rev-parse', 'HEAD'), commit)
        self.assertEqual(self.remote_git('rev-parse', 'refs/tags/v1.2.3^{commit}'), commit)

    def test_local_tag_from_failed_push_is_pushed_without_recreation(self):
        self.prepare()
        self.remote_git('update-ref', '-d', 'refs/tags/v1.2.3')
        self.state.write_text(json.dumps({'prs': [{'url': 'https://github.com/test/repo/pull/1', 'state': 'OPEN'}]}))
        tag_object = self.git('rev-parse', 'refs/tags/v1.2.3')
        commit = self.git('rev-parse', 'HEAD')
        self.prepare()
        self.assertEqual(self.git('rev-parse', 'HEAD'), commit)
        self.assertEqual(self.git('rev-parse', 'refs/tags/v1.2.3'), tag_object)
        self.assertEqual(self.remote_git('rev-parse', 'refs/tags/v1.2.3'), tag_object)

    def test_follow_tags_config_cannot_push_tag_before_pr_success(self):
        self.prepare()
        self.remote_git('update-ref', '-d', 'refs/tags/v1.2.3')
        self.git('config', 'push.followTags', 'true')
        self.state.write_text(json.dumps({'fail_create': True}))
        before = self.refs()
        self.prepare(success=False)
        self.assertEqual(self.refs(), before)

    def test_conflicting_local_only_tag_is_rejected(self):
        self.prepare()
        self.remote_git('update-ref', '-d', 'refs/tags/v1.2.3')
        self.add_commit('different-source')
        self.assert_rejected_without_ref_changes()

    def test_divergent_untagged_branch_is_rejected_before_checkout(self):
        self.state.write_text(json.dumps({'fail_create': True}))
        self.prepare(success=False)
        self.git('checkout', '-q', 'master')
        other_commit = self.add_commit('divergent-source')
        self.git('push', 'origin', 'HEAD:refs/heads/divergent-fixture')
        self.remote_git('update-ref', 'refs/heads/v1.2.3', other_commit)
        self.state.write_text('{}')
        self.assert_rejected_without_ref_changes()

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
