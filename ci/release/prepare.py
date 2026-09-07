#!/usr/bin/env python3
"""Prepare release metadata, branch, tag, and PR; Actions builds the draft asynchronously."""

import argparse
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import tempfile


def output(*args):
    return subprocess.check_output(args, text=True).strip()


def local_ref(ref):
    result = subprocess.run(["git", "rev-parse", "--verify", "--quiet", ref], text=True, stdout=subprocess.PIPE)
    return result.stdout.strip() if result.returncode == 0 else None


def has_file(commit, path):
    return subprocess.run(["git", "cat-file", "-e", f"{commit}:{path}"], stderr=subprocess.DEVNULL).returncode == 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default=os.environ.get("VERSION"))
    parser.add_argument("--prerelease", action="store_true", help="requires a version such as v0.9.0-rc.1")
    parser.add_argument("--dry-run", action="store_true", help="print preparation commands without changing files or GitHub")
    # Keep old flags recognizable so callers receive an actionable migration error.
    for flag in ("publish", "rebuild", "skip-build"):
        parser.add_argument(f"--{flag}", action="store_true", help=argparse.SUPPRESS)
    args = parser.parse_args()
    if args.skip_build:
        parser.error("release.sh no longer supports --skip-build; Actions owns all release assets")
    if args.publish or os.environ.get("PUBLISH"):
        parser.error("review the successful Actions run and publish its complete draft manually on GitHub")
    if args.rebuild:
        parser.error("use Re-run failed jobs in the existing Release archives run; local --rebuild is unsupported")
    if os.environ.get("GITHUB_ACTIONS") == "true":
        parser.error("run release.sh locally; GITHUB_TOKEN tag pushes do not start the release workflow")
    version = args.version or output("git", "describe")
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?", version):
        parser.error("--version must be a v-prefixed semantic version")
    if args.prerelease and "-" not in version:
        parser.error("--prerelease requires a semantic-version suffix, for example --version=v0.9.0-rc.1")
    if output("git", "status", "--porcelain"):
        parser.error("commit or stash local changes before preparing a release")
    repository = output("gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
    info = json.loads(output("gh", "api", f"repos/{repository}"))
    if not info.get("permissions", {}).get("push"):
        parser.error("the active GitHub identity needs push access to check for existing draft releases")
    pages = json.loads(output("gh", "api", "--paginate", "--slurp", f"repos/{repository}/releases?per_page=100"))
    releases = [release for page in pages for release in page if release["tag_name"] == version]
    if len(releases) > 1:
        parser.error("multiple releases have the requested tag")
    if releases and (not releases[0]["draft"] or any(asset["name"].endswith(".msi") for asset in releases[0]["assets"])):
        parser.error("this release is published or already has its MSI; use a new version for changes, or retry failed Actions jobs for the existing draft")
    branch_ref = f"refs/heads/{version}"
    tag_ref = f"refs/tags/{version}"
    remote_refs = {}
    for line in output("git", "ls-remote", "origin", branch_ref, tag_ref, tag_ref + "^{}").splitlines():
        commit, ref = line.split()
        remote_refs[ref] = commit
    branch = local_ref(branch_ref)
    local_tag = local_ref(tag_ref)
    local_tag_commit = local_ref(tag_ref + "^{commit}") if local_tag else None
    remote_branch = remote_refs.get(branch_ref)
    remote_tag = remote_refs.get(tag_ref)
    remote_tag_commit = remote_refs.get(tag_ref + "^{}", remote_tag)
    if local_tag and not local_tag_commit:
        parser.error("the local release tag does not point to a commit; choose a new version")
    if remote_tag:
        if (local_tag and local_tag != remote_tag) or any(ref and ref != remote_tag_commit for ref in (branch, remote_branch)):
            parser.error("existing release tag and local/remote release refs disagree; they will not be changed; choose a new version")
        print(f"{version} is already tagged. Use its existing Actions run for retries; changed code requires a new version.")
        return
    if releases:
        parser.error("a release already exists without its remote tag; do not reuse this version")
    source = branch or remote_branch or local_ref("refs/heads/master")
    if not source or not local_ref(source + "^{commit}"):
        parser.error("release branch commit is unavailable locally; fetch origin before retrying")
    if remote_branch and subprocess.run(["git", "merge-base", "--is-ancestor", remote_branch, source]).returncode != 0:
        parser.error("local and remote release branches diverged; they will not be changed; choose a new version")
    changelog = Path("ci/release/changelogs") / f"{version}.md"
    prepared = has_file(source, changelog)
    if local_tag and (local_tag_commit != source or not prepared):
        parser.error("the local tag does not match a prepared release branch; it will not be changed; choose a new version")
    if not prepared and (not has_file(source, "ci/release/changelogs/next.md") or not has_file(source, "ci/release/changelogs/template.md")):
        parser.error("release changelog inputs are missing from the source commit")

    def run(*command):
        if args.dry_run:
            print("+ " + shlex.join(command))
        else:
            subprocess.run(command, check=True)

    if not branch:
        run("git", "branch", version, source)
    run("git", "checkout", version)
    if not prepared:
        run("cp", "ci/release/changelogs/next.md", str(changelog))
        if "-" not in version:
            run("cp", "ci/release/changelogs/template.md", "ci/release/changelogs/next.md")
        run("git", "add", "--", "ci/release/changelogs")
        run("git", "commit", "-m", version)
    run("git", "-c", "push.followTags=false", "push", "origin", branch_ref)

    prs = json.loads(output("gh", "pr", "list", "--repo", repository, "--state", "all", "--head", version, "--json", "url,state"))
    if not any(pr["state"] in ("OPEN", "MERGED") for pr in prs):
        body = "## Human\n\n---\n\n## AI\n\nPrepare " + version + ". The Release archives workflow builds and verifies all release assets, then creates a draft for manual publication.\n"
        if args.dry_run:
            print("+ create release PR with the Human/AI description template")
        else:
            with tempfile.NamedTemporaryFile(mode="w", suffix=".md") as body_file:
                body_file.write(body)
                body_file.flush()
                run("gh", "pr", "create", "--repo", repository, "--base", "master", "--head", version, "--title", version, "--body-file", body_file.name)
    if not local_tag:
        run("git", "tag", "-a", version, "-m", version)
    run("git", "-c", "push.followTags=false", "push", "origin", tag_ref)
    if args.dry_run:
        print("Dry run complete. No files, Git refs, or GitHub records were changed.")
        return
    print(f"Prepared {version}. Follow https://github.com/{repository}/actions/workflows/release-archives.yml")
    print("Actions owns archive/MSI builds and draft upload. After it succeeds, review and merge the release PR, then publish the draft manually.")


if __name__ == "__main__":
    main()
