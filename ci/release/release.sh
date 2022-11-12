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
   - If the move occured, changelogs/next.md is replaced with changelogs/template.md. As
     well, a git commit with title v0.0.99 will be created.
3. It pushes branch v0.0.99 to origin.
4. It creates a v0.0.99 git tag if one does not already exist.
   If one does, it ensures the v0.0.99 tag points to the current commit.
   Then it pushes the tag to origin.
5. It creates a draft GitHub release for the tag if one does not already exist.
6. It updates the GitHub release notes to match changelogs/v0.0.99.md.
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

  runjob ensure_branch
  runjob ensure_changelog
  runjob ensure_commit
  # runjob push_branch
  # 3_commit
  # 4_tag
  # 5_draft_release
  # 6_draft_pr
  # 7_build_assets
  # 9_upload_assets

  # if [ "$(git_describe_ref)" != "$TAG" ]; then
  #   git tag -am "$TAG" "$TAG"
  # fi
  # hide git push origin "$TAG"
}

ensure_branch() {
  if [ -z "$(git branch --list "$VERSION")" ]; then
    sh_c git branch "$VERSION" master
  fi
  sh_c git checkout -q "$VERSION"
}

ensure_changelog() {
  if [ -f "./ci/release/changelogs/$VERSION.md" ]; then
    return 0
  fi

  sh_c cp "./ci/release/changelogs/next.md" "./ci/release/changelogs/$VERSION.md"
  if [ -z "${PRERELEASE-}" ]; then
    sh_c cp "./ci/release/changelogs/template.md" "./ci/release/changelogs/next.md"
  fi
}

ensure_commit() {
  sh_c git add --all
  if ! git commit --dry-run >/dev/null; then
    return 0
  fi
  if [ "$(git show --no-patch --format=%s)" = "$VERSION" ]; then
    sh_c git commit --amend --no-edit
  else
    sh_c git commit -m "$VERSION"
  fi
}

push_branch() {
  sh_c git push -fu origin "$VERSION"
}

ensure_built_assets() {
  ./ci/release/build.sh ${REBUILD:+--rebuild} $VERSION
}

main "$@"
