#!/usr/bin/env python3
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("upload_draft", Path(__file__).with_name("upload-draft.py"))
uploader = importlib.util.module_from_spec(spec)
spec.loader.exec_module(uploader)

VERSION = "v1.2.3"
COMMIT = "a" * 40


class FakeGitHub:
    def __init__(self, assets=(), published=False):
        self.release = {"id": 123, "tag_name": VERSION, "draft": not published, "prerelease": False, "assets": [], "html_url": "https://github.com/test/repo/releases/123"}
        self.commit = COMMIT
        self.writes = []
        self.change_after_upload = None
        self.change_before_release_read = None
        self.change_before_asset_read = None
        self.fail_upload_once = False
        self.next_asset_id = 1000
        for asset in assets:
            self.add_asset(*asset)

    def add_asset(self, path, digest, size):
        asset = {"id": self.next_asset_id, "name": path.name, "state": "uploaded", "digest": digest, "size": size}
        self.next_asset_id += 1
        self.release["assets"].append(asset)
        return asset

    def add_starter(self, path):
        asset = self.add_asset(path, None, 0)
        asset["state"] = "starter"
        return asset

    def api(self, path):
        if "/git/ref/tags/" in path:
            return {"object": {"type": "commit", "sha": self.commit}}
        if path.endswith("/releases/123"):
            if self.change_before_release_read:
                self.change_before_release_read(self)
                self.change_before_release_read = None
            return copy.deepcopy(self.release)
        if "/releases/assets/" in path:
            asset = next(asset for asset in self.release["assets"] if asset["id"] == int(path.rsplit("/", 1)[1]))
            if self.change_before_asset_read:
                self.change_before_asset_read(asset)
                self.change_before_asset_read = None
            return copy.deepcopy(asset)
        raise AssertionError(path)

    def gh(self, *args):
        if args[:3] == ("api", "--paginate", "--slurp"):
            return json.dumps([[self.release] if self.release else []])
        if args[:2] == ("release", "create"):
            self.assert_create(args)
            self.release = FakeGitHub().release
        elif args[:2] == ("release", "upload"):
            path = Path(args[3])
            assert "--clobber" not in args
            assert not any(asset["name"] == path.name for asset in self.release["assets"])
            if self.fail_upload_once:
                self.fail_upload_once = False
                self.add_starter(path)
                self.writes.append(args)
                raise RuntimeError("502 upload failure left an empty starter")
            self.add_asset(path, uploader.digest(path), path.stat().st_size)
            if self.change_after_upload:
                self.change_after_upload(self)
        elif args[:3] == ("api", "--method", "DELETE"):
            asset_id = int(args[3].rsplit("/", 1)[1])
            self.release["assets"] = [asset for asset in self.release["assets"] if asset["id"] != asset_id]
        else:
            raise AssertionError(args)
        self.writes.append(args)
        return ""

    @staticmethod
    def assert_create(args):
        assert "--draft" in args and "--verify-tag" in args
        assert "--notes-file" in args


class DraftUploadTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.archives = self.root / "archives"
        self.archives.mkdir()
        lines = []
        for target in uploader.TARGETS:
            path = self.archives / f"d2-{VERSION}-{target}.tar.gz"
            path.write_bytes(target.encode())
            lines.append(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n")
        (self.archives / "SHA256SUMS").write_text("".join(lines))
        self.msi = self.root / f"d2-{VERSION}-windows-amd64.msi"
        self.msi.write_bytes(b"verified installer")
        self.sbom = self.root / "d2.spdx.json"
        self.sbom.write_text('{"spdxVersion":"SPDX-2.3"}')
        self.assets = self.read_assets()

    def read_assets(self):
        return uploader.local_assets(VERSION, self.archives, self.msi, self.sbom)

    def run_upload(self, fake):
        with patch.object(uploader, "gh", fake.gh), patch.object(uploader, "api", fake.api), patch.object(uploader.time, "sleep"), patch.dict(os.environ, {"GITHUB_STEP_SUMMARY": ""}):
            uploader.upload("test/repo", VERSION, COMMIT, self.assets)

    def test_manifest_rejects_changed_archive(self):
        self.assets[0][0].write_bytes(b"changed")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            self.read_assets()

    def test_manifest_rejects_duplicate_entry(self):
        manifest = self.archives / "SHA256SUMS"
        manifest.write_text(manifest.read_text() + manifest.read_text().splitlines()[0] + "\n")
        with self.assertRaisesRegex(ValueError, "duplicate"):
            self.read_assets()

    def test_manifest_rejects_extra_asset(self):
        (self.archives / "unexpected").write_bytes(b"extra")
        with self.assertRaisesRegex(ValueError, "exactly six"):
            self.read_assets()

    def test_rejects_symlinked_installer(self):
        replacement = self.root / "outside.msi"
        self.msi.rename(replacement)
        self.msi.symlink_to(replacement)
        with self.assertRaisesRegex(ValueError, "regular file"):
            self.read_assets()

    def test_upload_and_retry_preserve_exact_bytes(self):
        fake = FakeGitHub()
        self.run_upload(fake)
        self.assertEqual(len(fake.writes), 9)
        self.run_upload(fake)
        self.assertEqual(len(fake.writes), 9)

    def test_conflict_preflight_prevents_partial_upload(self):
        fake = FakeGitHub([self.assets[-1]])
        fake.release["assets"][0]["digest"] = "sha256:" + "b" * 64
        with self.assertRaisesRegex(ValueError, "different bytes"):
            self.run_upload(fake)
        self.assertEqual(fake.writes, [])

    def test_starter_cleanup_recovers_repeated_upload_failure(self):
        fake = FakeGitHub(self.assets[:2])
        completed = copy.deepcopy(fake.release["assets"])
        first_id = fake.add_starter(self.assets[2][0])["id"]
        fake.fail_upload_once = True
        with self.assertRaisesRegex(RuntimeError, "502"):
            self.run_upload(fake)
        second_id = fake.release["assets"][-1]["id"]
        self.assertNotEqual(first_id, second_id)
        self.run_upload(fake)
        writes = copy.deepcopy(fake.writes)
        self.run_upload(fake)
        self.assertEqual(fake.writes, writes)
        self.assertEqual(fake.release["assets"][:2], completed)
        self.assertEqual(len(fake.release["assets"]), 9)
        self.assertEqual([call[3] for call in fake.writes if call[:3] == ("api", "--method", "DELETE")],
                         [f"repos/test/repo/releases/assets/{first_id}", f"repos/test/repo/releases/assets/{second_id}"])

    def test_unsafe_starter_metadata_is_never_deleted(self):
        for field, value in (("state", "uploading"), ("state", "uploaded"), ("size", 1), ("size", False),
                             ("digest", "sha256:" + "b" * 64), ("id", "1000"), ("id", -1), ("id", True), ("id", None)):
            with self.subTest(field=field, value=value):
                fake = FakeGitHub()
                fake.add_starter(self.assets[0][0])[field] = value
                with self.assertRaisesRegex(ValueError, "incomplete metadata"):
                    self.run_upload(fake)
                self.assertEqual(fake.writes, [])

    def test_starter_change_before_cleanup_prevents_deletion(self):
        path, digest, size = self.assets[0]
        changes = ({"id": 456}, {"state": "uploading"}, {"size": 1}, {"digest": digest},
                   {"state": "uploaded", "size": size, "digest": digest})
        for phase in ("release", "asset"):
            for change in changes:
                with self.subTest(phase=phase, change=change):
                    fake = FakeGitHub()
                    starter = fake.add_starter(path)
                    if phase == "release":
                        fake.change_before_release_read = lambda state: starter.update(change)
                    else:
                        fake.change_before_asset_read = lambda asset: asset.update(change)
                    with self.assertRaises(ValueError):
                        self.run_upload(fake)
                    self.assertEqual(fake.writes, [])

    def test_starter_name_change_at_exact_asset_read_prevents_deletion(self):
        fake = FakeGitHub()
        fake.add_starter(self.assets[0][0])
        fake.change_before_asset_read = lambda asset: asset.update(name="unrelated.tar.gz")
        with self.assertRaisesRegex(ValueError, "starter asset changed"):
            self.run_upload(fake)
        self.assertEqual(fake.writes, [])

    def test_conflict_elsewhere_prevents_starter_cleanup(self):
        for phase in ("preflight", "cleanup"):
            with self.subTest(phase=phase):
                fake = FakeGitHub([self.assets[-1]])
                fake.add_starter(self.assets[0][0])
                def conflict(state):
                    state.release["assets"][0]["digest"] = "sha256:" + "b" * 64
                if phase == "preflight":
                    conflict(fake)
                else:
                    fake.change_before_release_read = conflict
                with self.assertRaisesRegex(ValueError, "different bytes"):
                    self.run_upload(fake)
                self.assertEqual(fake.writes, [])

    def test_duplicate_starters_are_never_deleted(self):
        fake = FakeGitHub()
        fake.add_starter(self.assets[0][0])
        fake.add_starter(self.assets[0][0])
        with self.assertRaisesRegex(ValueError, "incomplete metadata"):
            self.run_upload(fake)
        self.assertEqual(fake.writes, [])

    def test_release_or_tag_change_before_cleanup_prevents_deletion(self):
        for field, value in (("id", 456), ("prerelease", True), ("draft", False), ("tag_name", "v9.9.9"), ("commit", "b" * 40)):
            with self.subTest(field=field):
                fake = FakeGitHub()
                fake.add_starter(self.assets[0][0])
                def change(state):
                    if field == "commit":
                        state.commit = value
                    else:
                        state.release[field] = value
                fake.change_before_release_read = change
                with self.assertRaises(ValueError):
                    self.run_upload(fake)
                self.assertEqual(fake.writes, [])

    def test_published_release_is_never_modified(self):
        fake = FakeGitHub(published=True)
        with self.assertRaisesRegex(ValueError, "remain a draft"):
            self.run_upload(fake)
        self.assertEqual(fake.writes, [])

    def test_moved_tag_is_never_modified(self):
        fake = FakeGitHub()
        fake.commit = "b" * 40
        with self.assertRaisesRegex(ValueError, "tag changed"):
            self.run_upload(fake)
        self.assertEqual(fake.writes, [])

    def test_tag_change_during_upload_stops_following_writes(self):
        fake = FakeGitHub()
        fake.change_after_upload = lambda state: setattr(state, "commit", "b" * 40)
        with self.assertRaisesRegex(ValueError, "tag changed"):
            self.run_upload(fake)
        self.assertEqual(len(fake.writes), 1)

    def test_draft_change_during_upload_stops_following_writes(self):
        for field, value in (("id", 456), ("prerelease", True), ("draft", False), ("tag_name", "v9.9.9")):
            with self.subTest(field=field):
                fake = FakeGitHub()
                fake.change_after_upload = lambda state: state.release.update({field: value})
                with self.assertRaises(ValueError):
                    self.run_upload(fake)
                self.assertEqual(len(fake.writes), 1)

    def test_create_requires_notes_and_stays_draft(self):
        fake = FakeGitHub()
        fake.release = None
        previous = os.getcwd()
        os.chdir(self.root)
        self.addCleanup(os.chdir, previous)
        notes = self.root / "ci/release/changelogs" / f"{VERSION}.md"
        notes.parent.mkdir(parents=True)
        notes.write_text("Release notes")
        self.run_upload(fake)
        self.assertEqual(fake.writes[0][:2], ("release", "create"))
        self.assertTrue(fake.release["draft"])


if __name__ == "__main__":
    unittest.main()
