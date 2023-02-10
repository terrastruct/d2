#!/bin/sh
set -eu

. "$(dirname "$0")/../../../ci/sub/lib.sh"
cd -- "$(dirname "$0")/../../.."

help() {
      cat <<EOF
usage: $0 [-p|--push] [--latest] [--version=str]
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
        PUSH=1
        ;;
      latest)
        flag_noarg && shift "$FLAGSHIFT"
        LATEST=1
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

  flags='--load'
  if [ -n "${PUSH-}" -o -n "${RELEASE-}" ]; then
    flags='--push --platform linux/amd64,linux/arm64'
  fi
  if [ -n "${LATEST-}" -o -n "${RELEASE-}" ]; then
    flags="$flags -t $D2_DOCKER_IMAGE:latest"
  fi
  sh_c docker buildx build $flags \
    -t "$D2_DOCKER_IMAGE:$VERSION" \
    --build-arg "VERSION=$VERSION" \
    -f ./ci/release/docker/Dockerfile "./ci/release/build/$VERSION/docker"
}

main "$@"
