#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

# Install the packed artifact outside the checkout so imports exercise package
# exports and shipped files, without resolving dependencies from the source tree.
TEST_DIR=$(mktemp -d "${TMPDIR:-/tmp}/d2-js-integration.XXXXXX")
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM

npm pack --json --pack-destination "$TEST_DIR" > "$TEST_DIR/pack.json"
TARBALL=$(node -e '
  const fs = require("node:fs");
  const packages = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
  const [metadata] = Object.values(packages);
  process.stdout.write(metadata.filename);
' "$TEST_DIR/pack.json")

mkdir "$TEST_DIR/consumer"
cp test/integration/*.test.* "$TEST_DIR/consumer/"
cd "$TEST_DIR/consumer"
printf '%s\n' '{"private":true}' > package.json
npm install --ignore-scripts --no-audit --no-fund --package-lock=false \
  "$TEST_DIR/$TARBALL"
node --test --test-timeout=30000 ./*.test.*
