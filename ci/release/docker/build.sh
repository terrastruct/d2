#!/bin/sh
set -eu

. "$(dirname "$0")/../../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/../../.."

help() {
      cat <<EOF
usage: $0 [--version=str]

Build and load a local, single-platform Docker image. Production publishing is handled by
the protected Publish Docker release GitHub workflow.
EOF
}

main() {
  while flag_parse "$@"; do
    case "$FLAG" in
      h|help)
        help
        return 0
        ;;
      p|push)
        flag_noarg && shift "$FLAGSHIFT"
        echo "--push is disabled; use the Publish Docker release GitHub workflow" >&2
        return 2
        ;;
      latest)
        flag_noarg && shift "$FLAGSHIFT"
        echo "--latest is disabled; use the Publish Docker release GitHub workflow" >&2
        return 2
        ;;
      version)
        flag_reqarg && shift "$FLAGSHIFT"
        VERSION=$FLAGARG
        ;;
      *)
        flag_errusage "unrecognized flag $FLAGRAW"
        ;;
    esac
  done
  shift "$FLAGSHIFT"

  if [ -n "${RELEASE-}" ]; then
    echo "RELEASE-driven Docker publishing is disabled; use the Publish Docker release GitHub workflow" >&2
    return 2
  fi

  if [ -z "${VERSION-}" ]; then
    VERSION=$(readlink ./ci/release/build/latest)
  fi
  D2_DOCKER_IMAGE=${D2_DOCKER_IMAGE:-terrastruct/d2}

  sh_c mkdir -p "./ci/release/build/$VERSION/docker"
  sh_c cp \
    "./ci/release/build/$VERSION/d2-$VERSION"-linux-*.tar.gz \
    "./ci/release/build/$VERSION/docker/"
  sh_c cp \
    ./ci/release/docker/entrypoint.sh \
    "./ci/release/build/$VERSION/docker/entrypoint.sh"

  sh_c docker buildx build --load \
    -t "$D2_DOCKER_IMAGE:$VERSION" \
    -f ./ci/release/docker/Dockerfile "./ci/release/build/$VERSION/docker"
}

main "$@"
