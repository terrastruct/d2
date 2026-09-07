#!/usr/bin/env bash
# Shared validation for Docker release scripts. Sourcing this file has no side effects.

die() { echo "$*" >&2; exit 1; }

validate_version() {
  [[ ${#1} -le 128 && $1 =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]] ||
    die "version must be a v-prefixed semantic version usable as a Docker tag"
}

validate_digest() {
  [[ $1 =~ ^sha256:[0-9a-f]{64}$ ]] || die "invalid SHA-256 digest: $1"
}

validate_arch() {
  [[ $1 == amd64 || $1 == arm64 ]] || die "unsupported architecture: $1"
}

release_metadata() {
  local version=$1
  jq -ceS --arg version "$version" '
    def asset($arch):
      [.assets[] | select(.name == "d2-" + $version + "-linux-" + $arch + ".tar.gz")] |
      if length != 1 then error("expected exactly one release archive for " + $arch)
      else .[0] end |
      if .state != "uploaded" or .size <= 0 or
         (.id | type != "number") or (.id | floor != .) or .id <= 0 or
         (.digest | type != "string") or (.digest | test("^sha256:[0-9a-f]{64}$") | not)
      then error("invalid release archive for " + $arch)
      else {id, digest} end;
    if .tag_name != $version or .draft != false or .published_at == null or
       (.id | type != "number") or (.id | floor != .) or .id <= 0 or
       (.prerelease | type != "boolean")
    then error("release identity or publication state is invalid")
    else {version: $version, id, prerelease, assets: {amd64: asset("amd64"), arm64: asset("arm64")}} end
  '
}

require_stable() {
  [[ $VERSION != *-* && $(jq -r '.prerelease' <<<"$RELEASE") == false ]] ||
    die "publish_latest cannot be used for a prerelease"
}

resolve_tag_commit() {
  local object sha type depth=0
  object=$(gh api "repos/$GITHUB_REPOSITORY/git/ref/tags/$VERSION")
  sha=$(jq -r '.object.sha' <<<"$object")
  type=$(jq -r '.object.type' <<<"$object")
  while [[ $type == tag ]]; do
    depth=$((depth + 1))
    [[ $depth -le 8 && $sha =~ ^[0-9a-f]{40}$ ]] || die "could not safely peel tag $VERSION"
    object=$(gh api "repos/$GITHUB_REPOSITORY/git/tags/$sha")
    sha=$(jq -r '.object.sha' <<<"$object")
    type=$(jq -r '.object.type' <<<"$object")
  done
  [[ $type == commit && $sha =~ ^[0-9a-f]{40}$ ]] || die "$VERSION does not peel to a commit"
  printf '%s\n' "$sha"
}

release_snapshot() {
  local metadata commit status
  metadata=$(gh api "repos/$GITHUB_REPOSITORY/releases/tags/$VERSION" | release_metadata "$VERSION")
  commit=$(resolve_tag_commit)
  status=$(gh api "repos/$GITHUB_REPOSITORY/compare/$commit...$GITHUB_SHA" --jq '.status')
  [[ $status == ahead || $status == identical ]] ||
    die "$VERSION is not an ancestor of the protected master dispatch"
  jq -cS --arg commit "$commit" '. + {commit: $commit}' <<<"$metadata"
}

revalidate_release() {
  [[ $(release_snapshot) == "$RELEASE" ]] ||
    die "$VERSION release identity, tag, publication state, or archive IDs/digests changed after validation"
}

validate_candidate() {
  jq -e '
    def runtime:
      .platform.os == "linux" and
      (.platform.architecture == "amd64" or .platform.architecture == "arm64");
    def attestation:
      .platform.os == "unknown" and .platform.architecture == "unknown" and
      .annotations["vnd.docker.reference.type"] == "attestation-manifest";
    .schemaVersion == 2 and (.manifests | type == "array") and
    ([.manifests[] | select(runtime and .platform.architecture == "amd64")] | length == 1) and
    ([.manifests[] | select(runtime and .platform.architecture == "arm64")] | length == 1) and
    ([.manifests[] | select(attestation)] | length == 2) and (.manifests | length == 4) and
    (([.manifests[] | select(runtime) | .digest] | sort) ==
     ([.manifests[] | select(attestation) | .annotations["vnd.docker.reference.digest"]] | sort))
  ' "$1" >/dev/null || die "candidate must contain exactly both Linux architectures and their provenance"
}

manifest_digest() {
  local size
  [[ $(tail -c 1 "$1" | od -An -tx1 | tr -d ' \n') == 0a ]] ||
    die "candidate manifest output must end in a newline"
  size=$(wc -c <"$1" | tr -d ' ')
  # buildx dry-run appends one newline that is not part of the registry manifest.
  printf 'sha256:%s\n' "$(dd if="$1" bs=1 count="$((size - 1))" 2>/dev/null | sha256sum | awk '{print $1}')"
}

compare_descriptors() {
  jq -S '.manifests | sort_by(.digest)' "$1" >"$WORK_DIR/expected-descriptors.json"
  jq -S '.manifests | sort_by(.digest)' "$2" >"$WORK_DIR/actual-descriptors.json"
  cmp -s "$WORK_DIR/expected-descriptors.json" "$WORK_DIR/actual-descriptors.json" ||
    die "manifest descriptors differ: $1 and $2"
}

registry_head() {
  local image=$1 ref=$2 token headers="$WORK_DIR/registry-headers"
  token=$(curl -fsSL --get 'https://auth.docker.io/token' \
    --data-urlencode 'service=registry.docker.io' --data-urlencode "scope=repository:$image:pull" | jq -er '.token')
  REGISTRY_STATUS=$(curl -sS -o /dev/null -D "$headers" -w '%{http_code}' --head \
    -H "Authorization: Bearer $token" \
    -H 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json' \
    "https://registry-1.docker.io/v2/$image/manifests/$ref")
  REGISTRY_DIGEST=
  case "$REGISTRY_STATUS" in
    200)
      REGISTRY_DIGEST=$(tr -d '\r' <"$headers" | awk 'tolower($1) == "docker-content-digest:" {print $2}' | tail -n 1)
      validate_digest "$REGISTRY_DIGEST"
      ;;
    404) ;;
    *) die "Docker Hub $image:$ref returned HTTP $REGISTRY_STATUS" ;;
  esac
}

verify_ref() {
  local image=$1 ref=$2 expected_digest=$3 expected_manifest=$4
  registry_head "$image" "$ref"
  [[ $REGISTRY_STATUS == 200 && $REGISTRY_DIGEST == "$expected_digest" ]] ||
    die "$image:$ref does not resolve to the verified manifest digest"
  docker buildx imagetools inspect --raw "$image:$ref" >"$WORK_DIR/verified-manifest.json"
  compare_descriptors "$expected_manifest" "$WORK_DIR/verified-manifest.json"
  docker buildx imagetools inspect "$image:$ref"
}
