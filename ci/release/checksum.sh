#!/bin/sh
if [ -n "${D2_RELEASE_CHECKSUM-}" ]; then
  return 0
fi
D2_RELEASE_CHECKSUM=1

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{ print $1 }'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$1" | awk '{ print $NF }'
  else
    echo >&2 "sha256sum, shasum, or openssl is required to verify release downloads"
    return 1
  fi
}

release_asset_digest_from_json() {
  ASSET_NAME=$1
  awk -v asset="$ASSET_NAME" '
    /^[[:space:]]*"name":[[:space:]]*/ {
      value = $0
      sub(/^[^:]*:[[:space:]]*"/, "", value)
      sub(/"[,[:space:]]*$/, "", value)
      found = value == asset
      next
    }
    found && /^[[:space:]]*"digest":[[:space:]]*/ {
      value = $0
      original = value
      sub(/^[^:]*:[[:space:]]*"sha256:/, "", value)
      if (value == original) {
        exit
      }
      sub(/"[,[:space:]]*$/, "", value)
      print value
      exit
    }
  '
}

release_asset_sha256() {
  REPOSITORY=$1
  RELEASE_VERSION=$2
  ASSET_NAME=$3
  RELEASE_INFO=$(mktempd)/release.json
  RELEASE_INFO_URL="https://api.github.com/repos/$REPOSITORY/releases/tags/$RELEASE_VERSION"
  DRY_RUN= fetch_gh "$RELEASE_INFO_URL" "$RELEASE_INFO" 'application/vnd.github+json'
  EXPECTED_SHA256=$(release_asset_digest_from_json "$ASSET_NAME" <"$RELEASE_INFO")
  EXPECTED_SHA256=$(printf '%s' "$EXPECTED_SHA256" | tr '[:upper:]' '[:lower:]')
  if ! printf '%s\n' "$EXPECTED_SHA256" | grep -Eq '^[0-9a-f]{64}$'; then
    echo >&2 "$ASSET_NAME does not have a valid GitHub SHA-256 digest"
    return 1
  fi
  printf '%s\n' "$EXPECTED_SHA256"
}

verify_sha256() {
  FILE=$1
  EXPECTED_SHA256=$2
  ACTUAL_SHA256=$(sha256_file "$FILE")
  ACTUAL_SHA256=$(printf '%s' "$ACTUAL_SHA256" | tr '[:upper:]' '[:lower:]')
  if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
    echo >&2 "$FILE has SHA-256 $ACTUAL_SHA256, expected $EXPECTED_SHA256"
    return 1
  fi
}

fetch_verified_release_asset() {
  REPOSITORY=$1
  RELEASE_VERSION=$2
  ASSET_NAME=$3
  ASSET_URL=$4
  DESTINATION=$5

  if [ -n "${DRY_RUN-}" ]; then
    sh_c "download $ASSET_NAME and verify its GitHub SHA-256 digest"
    return
  fi

  EXPECTED_SHA256=$(release_asset_sha256 "$REPOSITORY" "$RELEASE_VERSION" "$ASSET_NAME")
  if [ -e "$DESTINATION" ]; then
    if verify_sha256 "$DESTINATION" "$EXPECTED_SHA256"; then
      log "reusing verified $DESTINATION"
      return
    fi
    warn "discarding cached $ASSET_NAME after checksum verification failed"
    rm -f "$DESTINATION"
  fi
  fetch_gh "$ASSET_URL" "$DESTINATION" 'application/octet-stream'
  verify_sha256 "$DESTINATION" "$EXPECTED_SHA256"
  log "verified $ASSET_NAME ($EXPECTED_SHA256)"
}
