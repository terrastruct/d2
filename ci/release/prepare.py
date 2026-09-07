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
        parser.error("this release is published or already has its MSI; retry failed Actions jobs instead of preparing it again")

    def run(*command):
        if args.dry_run:
            print("+ " + shlex.join(command))
        else:
            subprocess.run(command, check=True)

    branch_exists = subprocess.run(["git", "show-ref", "--verify", "--quiet", f"refs/heads/{version}"]).returncode == 0
    if not branch_exists:
        run("git", "branch", version, "master")
    run("git", "checkout", version)
    changelog = Path("ci/release/changelogs") / f"{version}.md"
    if not changelog.exists():
        run("cp", "ci/release/changelogs/next.md", str(changelog))
        if "-" not in version:
            run("cp", "ci/release/changelogs/template.md", "ci/release/changelogs/next.md")
    run("git", "add", "--", "ci/release/changelogs")
    if output("git", "show", "--no-patch", "--format=%s") == version:
        run("git", "commit", "--allow-empty", "--amend", "--no-edit")
    else:
        run("git", "commit", "--allow-empty", "-m", version)
    run("git", "push", "-f", "origin", f"refs/heads/{version}")

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
    # Retain the existing release-tag policy. Tag immutability is a separate change.
    run("git", "tag", "--force", "-a", version, "-m", version)
    run("git", "push", "-f", "origin", f"refs/tags/{version}")
    if args.dry_run:
        print("Dry run complete. No files, Git refs, or GitHub records were changed.")
        return
    print(f"Prepared {version}. Follow https://github.com/{repository}/actions/workflows/release-archives.yml")
    print("Actions owns archive/MSI builds and draft upload. After it succeeds, review and merge the release PR, then publish the draft manually.")


if __name__ == "__main__":
    main()
