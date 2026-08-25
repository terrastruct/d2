#!/bin/sh

verify_ci_release_asset_directory() {
  D2_RELEASE_ASSET_DIRECTORY=$1
  D2_RELEASE_ASSET_VERSION=$2
  D2_RELEASE_ASSET_REQUIRE_ALL=${3:-1}

  if [ ! -d "$D2_RELEASE_ASSET_DIRECTORY" ]; then
    echo >&2 "release asset directory does not exist: $D2_RELEASE_ASSET_DIRECTORY"
    return 1
  fi
  if [ -L "$D2_RELEASE_ASSET_DIRECTORY" ]; then
    echo >&2 "release asset directory must not be a symlink: $D2_RELEASE_ASSET_DIRECTORY"
    return 1
  fi

  D2_RELEASE_UNEXPECTED_ASSET=$(find "$D2_RELEASE_ASSET_DIRECTORY" \
    -mindepth 1 -maxdepth 1 \( -type f -o -type l \) \
    ! -name "d2-$D2_RELEASE_ASSET_VERSION-linux-amd64.tar.gz" \
    ! -name "d2-$D2_RELEASE_ASSET_VERSION-linux-arm64.tar.gz" \
    ! -name "d2-$D2_RELEASE_ASSET_VERSION-macos-amd64.tar.gz" \
    ! -name "d2-$D2_RELEASE_ASSET_VERSION-macos-arm64.tar.gz" \
    ! -name "d2-$D2_RELEASE_ASSET_VERSION-windows-amd64.tar.gz" \
    ! -name "d2-$D2_RELEASE_ASSET_VERSION-windows-arm64.tar.gz" \
    ! -name SHA256SUMS \
    -print -quit)
  if [ -n "$D2_RELEASE_UNEXPECTED_ASSET" ]; then
    echo >&2 "release asset directory contains an unexpected file: $D2_RELEASE_UNEXPECTED_ASSET"
    return 1
  fi

  D2_RELEASE_SYMLINKED_ASSET=$(find "$D2_RELEASE_ASSET_DIRECTORY" \
    -mindepth 1 -maxdepth 1 -type l -print -quit)
  if [ -n "$D2_RELEASE_SYMLINKED_ASSET" ]; then
    echo >&2 "release asset must not be a symlink: $D2_RELEASE_SYMLINKED_ASSET"
    return 1
  fi

  if [ "$D2_RELEASE_ASSET_REQUIRE_ALL" -eq 0 ]; then
    return 0
  fi

  for D2_RELEASE_TARGET in \
    linux-amd64 linux-arm64 \
    macos-amd64 macos-arm64 \
    windows-amd64 windows-arm64; do
    D2_RELEASE_EXPECTED_ASSET=$D2_RELEASE_ASSET_DIRECTORY/d2-$D2_RELEASE_ASSET_VERSION-$D2_RELEASE_TARGET.tar.gz
    if [ ! -f "$D2_RELEASE_EXPECTED_ASSET" ]; then
      echo >&2 "release asset directory is missing $D2_RELEASE_EXPECTED_ASSET"
      return 1
    fi
  done
  if [ ! -f "$D2_RELEASE_ASSET_DIRECTORY/SHA256SUMS" ]; then
    echo >&2 "release asset directory is missing $D2_RELEASE_ASSET_DIRECTORY/SHA256SUMS"
    return 1
  fi
}
