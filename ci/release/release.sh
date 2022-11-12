#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. ./ci/sub/lib.sh

help() {
  cat <<EOF
usage: $0 [--rebuild] <version>

$0 implements the D2 release process.

Flags:

--rebuild: Normally the release script will avoid rebuilding release assets if they
           already exist but if you changed something and need to force rebuild, use
           this flag.
--pre-release: Pass to mark the release on GitHub as a pre-release. For pre-releases the
               version format should include a suffix like v0.0.99-alpha.1
               As well, for pre-releases the script will not overwrite changelogs/next.md
               with changelogs/template.md and instead keep it the same as
               changelogs/v0.0.99-alpha.1.md. This is because you want to maintain the
               changelog entries for the eventual final release.

Process:

Let's say you passed in v0.0.99 as the version:

1. It creates branch v0.0.99 based on master if one does not already exist.
   - It then checks it out.
2. It moves changelogs/next.md to changelogs/v0.0.99.md if there isn't already a
   changelogs/v0.0.99.md.
   - If the move occured, changelogs/next.md is replaced with changelogs/template.md.
3. If the current commit does not have a title of v0.0.99 then a new commit with said
   title will be created with all uncommitted changes.
   - If the current commit does, then the uncommitted changes will be amended to the commit.
4. It pushes branch v0.0.99 to origin.
5. It creates a v0.0.99 git tag if one does not already exist.
   If one does, it ensures the v0.0.99 tag points to the current commit.
   Then it pushes the tag to origin.
6. It creates a draft GitHub release for the tag if one does not already exist.
   - It will also set the release notes to match changelogs/v0.0.99.md even
     if the release already exists.
7. It creates a draft PR for branch v0.0.99 into master if one does not already exist.
8. It builds the release assets if they do not exist.
   Pass --rebuild to force rebuilding all release assets.
9. It uploads the release assets overwriting any existing assets on the release.

Only a draft release will be created so do not fret if something goes wrong.
You can just rerun the script again as it is fully idempotent.

To complete the release, merge the release PR and then publish the draft release.

Testing:

For testing, change the origin remote to a private throwaway repository and push master to
it. Then the PR, tag and draft release will be generated against said throwaway
repository.

Example:
  $0 v0.0.99
EOF
}

main() {
  unset FLAG \
    FLAGRAW \
    FLAGARG \
    FLAGSHIFT \
    VERSION \
    REBUILD \
    PRERELEASE
  while :; do
    flag_parse "$@"
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      rebuild)
        flag_noarg
        REBUILD=1
        shift "$FLAGSHIFT"
        ;;
      pre-release)
        flag_noarg
        PRERELEASE=1
        shift "$FLAGSHIFT"
        ;;
      '')
        shift "$FLAGSHIFT"
        break
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done

  if [ $# -ne 1 ]; then
    flag_errusage "first argument must be release version like v0.0.99"
  fi
  VERSION="$1"
  shift

  runjob 1_ensure_branch _1_ensure_branch
  runjob 2_ensure_changelog _2_ensure_changelog
  runjob 3_ensure_commit _3_ensure_commit
  runjob 4_push_branch _4_push_branch
  runjob 5_ensure_tag _5_ensure_tag
  runjob 6_ensure_release _6_ensure_release
  runjob 7_ensure_pr _7_ensure_pr
  runjob 8_ensure_assets _8_ensure_assets
  runjob 9_upload_assets _9_upload_assets
}

_1_ensure_branch() {
  if [ -z "$(git branch --list "$VERSION")" ]; then
    sh_c git branch "$VERSION" master
  fi
  sh_c git checkout -q "$VERSION"
}

_2_ensure_changelog() {
  if [ -f "./ci/release/changelogs/$VERSION.md" ]; then
    return 0
  fi

  sh_c cp "./ci/release/changelogs/next.md" "./ci/release/changelogs/$VERSION.md"
  if [ -z "${PRERELEASE-}" ]; then
    sh_c cp "./ci/release/changelogs/template.md" "./ci/release/changelogs/next.md"
  fi
}

_3_ensure_commit() {
  sh_c git add --all
  if [ "$(git show --no-patch --format=%s)" = "$VERSION" ]; then
    sh_c git commit --amend --no-edit
  else
    sh_c git commit --allow-empty -m "$VERSION"
  fi
}

_4_push_branch() {
  if git rev-parse @{u} >/dev/null 2>&1; then
    sh_c git push -f origin "refs/heads/$VERSION"
  else
    sh_c git push -fu origin "refs/heads/$VERSION"
  fi
}

_5_ensure_tag() {
  sh_c git tag --force -a "$VERSION" -m "$VERSION"
  sh_c git push -f origin "refs/tags/$VERSION"
}

_6_ensure_release() {
  if gh release view "$VERSION" >/dev/null 2>&1; then
    sh_c gh release edit \
      --draft \
      --notes-file "./ci/release/changelogs/$VERSION.md" \
      ${PRERELEASE:+--prerelease} \
      "--title=$VERSION" \
      "$VERSION"
    return 0
  fi
  sh_c gh release create \
    --draft \
    --notes-file "./ci/release/changelogs/$VERSION.md" \
    ${PRERELEASE:+--prerelease} \
    "--title=$VERSION" \
    "$VERSION"
}

_7_ensure_pr() {
  pr_url=$(gh pr view "$VERSION" --json=url '--template={{ .url }}' 2>/dev/null || true)
  if [ -n "$pr_url" ]; then
    _echo "PR already exists: $pr_url"
    return 0
  fi
  sh_c gh pr create --draft --fill
}

_8_ensure_assets() {
  # ./ci/release/build.sh ${REBUILD:+--rebuild} $VERSION
  _echo TODO
}

_9_upload_assets() {
  sh_c gh release upload --clobber "$VERSION" "./ci/release/build/$VERSION"/*.tar.gz
}

main "$@"
