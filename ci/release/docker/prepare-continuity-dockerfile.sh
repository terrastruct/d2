#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 VERSION SOURCE_DOCKERFILE OUTPUT_DOCKERFILE" >&2
  exit 2
fi

version=$1
source_dockerfile=$2
output_dockerfile=$3

if [[ ! -f "$source_dockerfile" ]]; then
  echo "source Dockerfile does not exist: $source_dockerfile" >&2
  exit 1
fi

case "$version" in
  v0.7.1)
    # playwright-go v0.4702.0 expects a driver ZIP from the retired
    # playwright.azureedge.net endpoints. Recreate the same 1.47.2 driver
    # layout from the immutable official playwright-core npm tarball instead.
    playwright_version=1.47.2
    playwright_sha256=4f14eac4a244fada8e072d9932a1f9a4752a161b6726f227223ef33fd5dddbf4
    ;;
  *)
    install -m 0644 "$source_dockerfile" "$output_dockerfile"
    exit 0
    ;;
esac

if [[ $(grep -Fxc 'USER debian:debian' "$source_dockerfile") -ne 1 ]]; then
  echo "expected exactly one debian USER instruction in $source_dockerfile" >&2
  exit 1
fi

awk \
  -v playwright_version="$playwright_version" \
  -v playwright_sha256="$playwright_sha256" \
  '
    $0 == "USER debian:debian" {
      print "# Restore the Playwright driver without the retired Playwright CDN."
      print "ARG PLAYWRIGHT_GO_DRIVER_VERSION=" playwright_version
      print "ARG PLAYWRIGHT_GO_DRIVER_SHA256=" playwright_sha256
      print "RUN set -eux; \\"
      print "  driver_dir=\"/home/debian/.cache/ms-playwright-go/$PLAYWRIGHT_GO_DRIVER_VERSION\"; \\"
      print "  mkdir -p \"$driver_dir\"; \\"
      print "  curl -fsSL \"https://registry.npmjs.org/playwright-core/-/playwright-core-$PLAYWRIGHT_GO_DRIVER_VERSION.tgz\" -o /tmp/playwright-core.tgz; \\"
      print "  echo \"$PLAYWRIGHT_GO_DRIVER_SHA256  /tmp/playwright-core.tgz\" | sha256sum -c -; \\"
      print "  tar -xzf /tmp/playwright-core.tgz -C \"$driver_dir\"; \\"
      print "  rm /tmp/playwright-core.tgz; \\"
      print "  chown -R debian:debian /home/debian/.cache"
      print "ENV PLAYWRIGHT_NODEJS_PATH=/usr/bin/node"
      print "ENV PLAYWRIGHT_DOWNLOAD_HOST=https://cdn.playwright.dev"
      print ""
    }
    { print }
  ' "$source_dockerfile" >"$output_dockerfile"
