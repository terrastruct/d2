#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../.."
. "./ci/sub/lib.sh"

if [ -n "${PUBLISH-}" ]; then
  echo >&2 "the inherited PUBLISH flag is no longer supported"
  echo >&2 "create the draft, wait for the Windows MSI workflow, then merge and publish it on GitHub"
  exit 1
fi
unset PUBLISH

VERSION_ARG=${VERSION-}
SKIP_BUILD_ARG=
scan_arguments() {
  while flag_parse "$@"; do
    case "$FLAG" in
      h|help|rebuild|prerelease|dry-run)
        flag_noarg || return 1
        ;;
      skip-build)
        flag_noarg || return 1
        SKIP_BUILD_ARG=1
        ;;
      publish)
        flag_noarg || return 1
        echo >&2 "release.sh no longer publishes a release directly"
        echo >&2 "create the draft without --publish, wait for the Windows MSI workflow, then merge and publish the reviewed draft on GitHub"
        return 1
        ;;
      version)
        flag_nonemptyarg || return 1
        VERSION_ARG=$FLAGARG
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        return 1
        ;;
    esac
    shift "$FLAGSHIFT"
  done
  shift "$FLAGSHIFT"
  if [ $# -gt 0 ]; then
    flag_errusage "no arguments are accepted"
    return 1
  fi
}
scan_arguments "$@"

if [ -n "$SKIP_BUILD_ARG" ]; then
  echo >&2 "release.sh no longer supports --skip-build"
  echo >&2 "production releases must upload the exact verified Release archives workflow artifact"
  exit 1
fi

if [ -z "$VERSION_ARG" ]; then
  VERSION_ARG=$(git describe 2>/dev/null || true)
fi
case "$VERSION_ARG" in
  v*)
    MSI_ASSET=d2-$VERSION_ARG-windows-amd64.msi
    BUILD_DIRECTORY=ci/release/build/$VERSION_ARG
    if [ -d "$BUILD_DIRECTORY" ]; then
      LOCAL_MSI=$(find -L "$BUILD_DIRECTORY" -maxdepth 1 -type f -name '*.msi' -print -quit)
      if [ -n "$LOCAL_MSI" ]; then
        echo >&2 "legacy local MSI must not be uploaded: $LOCAL_MSI"
        echo >&2 "remove it and let the Windows MSI workflow build the installer from the uploaded archive"
        exit 1
      fi
    fi
    REPOSITORY=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
    REPOSITORY_JSON=$(gh api "repos/$REPOSITORY")
    if ! printf '%s' "$REPOSITORY_JSON" | jq -e \
      '.permissions.push == true' >/dev/null; then
      echo >&2 "the active GitHub API identity does not have push access to $REPOSITORY"
      echo >&2 "cannot safely distinguish a missing release from a hidden draft release"
      exit 1
    fi
    # GitHub's tag endpoint returns only published releases. Enumerate releases
    # after confirming push access so an existing draft cannot bypass this guard.
    RELEASE_PAGES=$(gh api --paginate --slurp \
      "repos/$REPOSITORY/releases?per_page=100")
    if ! printf '%s' "$RELEASE_PAGES" | jq -e \
      'type == "array" and all(.[]; type == "array")' >/dev/null; then
      echo >&2 "GitHub returned malformed release listings"
      exit 1
    fi
    RELEASE_MATCHES=$(printf '%s' "$RELEASE_PAGES" | jq -c \
      --arg version "$VERSION_ARG" \
      '[.[][] | select(.tag_name == $version)]')
    RELEASE_COUNT=$(printf '%s' "$RELEASE_MATCHES" | jq -r length)
    case "$RELEASE_COUNT" in
      0)
        MSI_COUNT=0
        ;;
      1)
        RELEASE_JSON=$(printf '%s' "$RELEASE_MATCHES" | jq -c '.[0]')
        if ! printf '%s' "$RELEASE_JSON" | jq -e --arg version "$VERSION_ARG" \
          'type == "object" and (.id | type == "number") and
            .tag_name == $version and (.assets | type == "array")' >/dev/null; then
          echo >&2 "GitHub returned malformed release metadata for $VERSION_ARG"
          exit 1
        fi
        MSI_COUNT=$(printf '%s' "$RELEASE_JSON" | jq -r --arg asset "$MSI_ASSET" \
          '[.assets[] | select(.name == $asset)] | length')
        ;;
      *)
        echo >&2 "GitHub returned multiple releases for $VERSION_ARG"
        exit 1
        ;;
    esac
    if [ -n "$MSI_COUNT" ] && [ "$MSI_COUNT" -gt 0 ]; then
      echo >&2 "$MSI_ASSET is already attached to the release"
      echo >&2 "do not rerun the release builder or move its tag; merge the release PR and publish the reviewed draft on GitHub"
      exit 1
    fi
    ;;
esac

exec ./ci/sub/release/release.sh "$@"
