#!/usr/bin/env bash
set -euo pipefail
. "$(dirname "$0")/common.sh"

command=${1:?usage: image.sh build local|push context OR image.sh smoke image}
shift
validate_arch "$ARCH"
validate_version "$VERSION"
PLATFORM="linux/$ARCH"
WORK_DIR=${WORK_DIR:-${RUNNER_TEMP:?}/docker-image-$ARCH}
mkdir -p "$WORK_DIR"

case "$command" in
  build)
    mode=${1:?build mode is required}
    context=${2:?Docker context directory is required}
    builder="d2-docker-$ARCH"
    trap 'docker buildx rm "$builder" >/dev/null 2>&1 || true' EXIT
    docker buildx create --name "$builder" --driver docker-container --use
    docker buildx inspect --bootstrap
    args=(--platform "$PLATFORM" --file "$context/Dockerfile" --metadata-file "$WORK_DIR/build.json")
    case "$mode" in
      local)
        args+=(--provenance=false --load --tag "${IMAGE:?local image name is required}")
        ;;
      push)
        images="${PRIMARY_DOCKER_IMAGE:?},${LEGACY_DOCKER_IMAGE:?}"
        args+=(--provenance=mode=max --output "type=image,\"name=$images\",push-by-digest=true,name-canonical=true,push=true")
        ;;
      *) die "unknown image build mode: $mode" ;;
    esac
    docker buildx build "${args[@]}" "$context"
    if [[ $mode == push ]]; then
      digest=$(jq -er '."containerimage.digest"' "$WORK_DIR/build.json")
      validate_digest "$digest"
      docker buildx imagetools inspect --raw "$PRIMARY_DOCKER_IMAGE@$digest" >"$WORK_DIR/primary.json"
      docker buildx imagetools inspect --raw "$LEGACY_DOCKER_IMAGE@$digest" >"$WORK_DIR/legacy.json"
      cmp -s "$WORK_DIR/primary.json" "$WORK_DIR/legacy.json" || die "architecture images differ between repositories"
      IMAGE="$PRIMARY_DOCKER_IMAGE@$digest"
      printf 'digest=%s\n' "$digest" >>"$GITHUB_OUTPUT"
    else
      # Measure compressed layers using the same cached build as the loaded smoke image.
      archive="$WORK_DIR/image.oci.tar"
      docker buildx build --platform "$PLATFORM" --provenance=false \
        --output "type=oci,dest=$archive,compression=gzip,compression-level=6,force-compression=true" \
        --file "$context/Dockerfile" "$context"
      manifest=$(tar -xOf "$archive" index.json | jq -er '.manifests[0].digest')
      validate_digest "$manifest"
      compressed=$(tar -xOf "$archive" "blobs/sha256/${manifest#sha256:}" | jq -er '[.layers[].size] | add')
      rm "$archive"
      uncompressed=$(docker image inspect --format '{{.Size}}' "$IMAGE")
      {
        echo "### $PLATFORM"
        echo
        echo "- Compressed OCI layers: $compressed bytes"
        echo "- Uncompressed image: $uncompressed bytes"
      } >>"$GITHUB_STEP_SUMMARY"
    fi
    printf 'image=%s\n' "$IMAGE" >>"$GITHUB_OUTPUT"
    ;;
  smoke)
    image=${1:?image reference is required}
    smoke="$WORK_DIR/smoke"
    mkdir -p "$smoke"
    printf 'x -> y\n' >"$smoke/input.d2"
    version_output=$(docker run --rm --platform "$PLATFORM" "$image" --version)
    [[ ${version_output%$'\r'} == "$VERSION" ]] || die "image version does not match $VERSION"
    for format in svg png; do
      docker run --rm --platform "$PLATFORM" -u "$(id -u):$(id -g)" \
        -v "$smoke:/home/debian/src" "$image" input.d2 "output.$format"
      test -s "$smoke/output.$format"
    done
    grep -q '<svg' "$smoke/output.svg"
    if [[ ${REQUIRE_TALA:-true} == true ]]; then
      docker run --rm --platform "$PLATFORM" -u "$(id -u):$(id -g)" \
        -v "$smoke:/home/debian/src" "$image" --layout=tala input.d2 output-tala.svg
      test -s "$smoke/output-tala.svg"
      grep -q '<svg' "$smoke/output-tala.svg"
    fi
    [[ $(od -An -tx1 -N8 "$smoke/output.png" | tr -d ' \n') == 89504e470d0a1a0a ]] || die "invalid PNG signature"
    ;;
  *) die "unknown image command: $command" ;;
esac
