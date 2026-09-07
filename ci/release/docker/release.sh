#!/usr/bin/env bash
set -euo pipefail
. "$(dirname "$0")/common.sh"

command=${1:?usage: release.sh validate|preflight|download|publish-version|publish-latest}
shift
WORK_DIR=${WORK_DIR:-${RUNNER_TEMP:?}/docker-release}
mkdir -p "$WORK_DIR"

if [[ $command == validate ]]; then
  [[ $GITHUB_REF == refs/heads/master && $REF_PROTECTED == true ]] ||
    die "Run this workflow from the protected master branch"
  validate_version "$VERSION"
  RELEASE=$(release_snapshot)
  if [[ $PUBLISH_LATEST == true ]]; then require_stable; fi
  printf 'release=%s\n' "$RELEASE" >>"$GITHUB_OUTPUT"
  exit 0
fi

VERSION=$(jq -er '.version' <<<"${RELEASE:?validated release snapshot is required}")
validate_version "$VERSION"
images=("${PRIMARY_DOCKER_IMAGE:?}" "${LEGACY_DOCKER_IMAGE:?}")

case "$command" in
  preflight)
    expected_rule='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$'
    for image in "${images[@]}"; do
      namespace=${image%%/*}
      repository=${image#*/}
      [[ -n $namespace && -n $repository && $repository != "$image" ]] || die "invalid image: $image"
      curl -fsSL "https://hub.docker.com/v2/namespaces/$namespace/repositories/$repository" |
        jq -e --arg rule "$expected_rule" \
          '.immutable_tags_settings.enabled == true and .immutable_tags_settings.rules == [$rule]' >/dev/null
      registry_head "$image" "$VERSION"
      [[ $REGISTRY_STATUS == 404 ]] || die "$image:$VERSION already exists; version tags are immutable"
      echo "$image:$VERSION is available for this release"
    done
    ;;
  download)
    arch=${1:?architecture is required}
    context=${2:?Docker context directory is required}
    source_dir=${3:?release Dockerfile directory is required}
    validate_arch "$arch"
    asset_id=$(jq -er --arg arch "$arch" '.assets[$arch].id' <<<"$RELEASE")
    asset_digest=$(jq -er --arg arch "$arch" '.assets[$arch].digest' <<<"$RELEASE")
    validate_digest "$asset_digest"
    [[ $asset_id =~ ^[0-9]+$ ]] || die "invalid release asset ID"
    mkdir -p "$context"
    archive="$context/d2-$VERSION-linux-$arch.tar.gz"
    gh api --method GET -H 'Accept: application/octet-stream' \
      "repos/$GITHUB_REPOSITORY/releases/assets/$asset_id" >"$archive"
    printf '%s  %s\n' "${asset_digest#sha256:}" "$archive" | sha256sum -c -
    tar -tzf "$archive" >/dev/null
    cp "$source_dir/entrypoint.sh" "$source_dir/Dockerfile" "$context/"
    ;;
  publish-version)
    digest_dir=${1:?digest artifact directory is required}
    # Fail closed if either matrix leg did not produce exactly its expected artifact.
    [[ $(find "$digest_dir" -mindepth 1 -maxdepth 1 | wc -l | tr -d ' ') == 2 ]] ||
      die "expected exactly two architecture digest artifacts"
    amd64_digest=$(cat "$digest_dir/amd64.txt")
    arm64_digest=$(cat "$digest_dir/arm64.txt")
    validate_digest "$amd64_digest"
    validate_digest "$arm64_digest"
    existing=()
    candidate_digest=
    for i in "${!images[@]}"; do
      image=${images[$i]}
      candidate="$WORK_DIR/candidate-$i.json"
      docker buildx imagetools create --dry-run --progress=none \
        "$image@$amd64_digest" "$image@$arm64_digest" >"$candidate"
      validate_candidate "$candidate"
      digest=$(manifest_digest "$candidate")
      if [[ -n $candidate_digest ]]; then
        [[ $digest == "$candidate_digest" ]] || die "primary and legacy candidate manifest digests differ"
        compare_descriptors "$WORK_DIR/candidate-0.json" "$candidate"
      fi
      candidate_digest=$digest
      registry_head "$image" "$VERSION"
      existing[$i]=$REGISTRY_STATUS
      if [[ $REGISTRY_STATUS == 200 ]]; then
        verify_ref "$image" "$VERSION" "$candidate_digest" "$candidate"
      fi
    done

    # Recheck all mutable GitHub inputs immediately before creating either tag.
    revalidate_release
    for i in "${!images[@]}"; do
      image=${images[$i]}
      metadata="$WORK_DIR/version-$i.json"
      if [[ ${existing[$i]} == 404 ]]; then
        docker buildx imagetools create --tag "$image:$VERSION" --metadata-file "$metadata" \
          "$image@$amd64_digest" "$image@$arm64_digest"
        [[ $(jq -er '."containerimage.descriptor".digest' "$metadata") == "$candidate_digest" ]] ||
          die "$image:$VERSION digest does not match the publication result"
      fi
      verify_ref "$image" "$VERSION" "$candidate_digest" "$WORK_DIR/candidate-$i.json"
    done
    printf 'digest=%s\n' "$candidate_digest" >>"$GITHUB_OUTPUT"
    {
      echo "Published and verified both Docker Hub release images."
      echo
      echo "- Primary: \`$PRIMARY_DOCKER_IMAGE:$VERSION\`"
      echo "- Legacy: \`$LEGACY_DOCKER_IMAGE:$VERSION\`"
      echo "- Release commit: \`$(jq -r '.commit' <<<"$RELEASE")\`"
      for arch in amd64 arm64; do
        echo "- $arch release asset: $(jq -c --arg arch "$arch" '.assets[$arch]' <<<"$RELEASE")"
      done
      echo "- amd64 image: \`$amd64_digest\`"
      echo "- arm64 image: \`$arm64_digest\`"
      echo "- Version manifest: \`$candidate_digest\`"
    } >>"$GITHUB_STEP_SUMMARY"
    ;;
  publish-latest)
    [[ ${PUBLISH_LATEST:-false} == true ]] || die "latest promotion must be explicitly requested"
    require_stable
    validate_digest "$VERSION_DIGEST"
    revalidate_release
    # Verify both immutable source manifests before moving either latest tag.
    for i in "${!images[@]}"; do
      docker buildx imagetools inspect --raw "${images[$i]}@$VERSION_DIGEST" >"$WORK_DIR/source-$i.json"
      validate_candidate "$WORK_DIR/source-$i.json"
    done
    compare_descriptors "$WORK_DIR/source-0.json" "$WORK_DIR/source-1.json"
    for i in "${!images[@]}"; do
      image=${images[$i]}
      docker buildx imagetools create --tag "$image:latest" "$image@$VERSION_DIGEST"
      verify_ref "$image" latest "$VERSION_DIGEST" "$WORK_DIR/source-$i.json"
    done
    {
      echo "Updated and verified both latest tags from the immutable version digest."
      echo
      for image in "${images[@]}"; do
        echo "- \`$image:latest\` from \`$image@$VERSION_DIGEST\`"
      done
    } >>"$GITHUB_STEP_SUMMARY"
    ;;
  *) die "unknown Docker release command: $command" ;;
esac
