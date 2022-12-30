#!/bin/sh
set -eu
cd -- "$(dirname "$0")/../../.."
. ./ci/sub/lib.sh

ensure_os
ensure_arch
cat <<EOF
# d2

For docs, more installation options and the source code see https://oss.terrastruct.com/d2

version: $VERSION
os: $OS
arch: $ARCH

Built with $(go version | grep -o 'go[^ ]\+').
EOF

if [ "$OS" = windows ]; then
  cat <<EOF

This release is structured the same as our Unix releases for use with MSYS2.

You may find our \`.msi\` installer more convenient as it handles putting \`d2.exe\` into
your \`\$PATH\` for you.

See https://github.com/terrastruct/d2/blob/master/docs/INSTALL.md#windows
EOF
fi

cat <<EOF

## Install

\`\`\`sh
make install DRY_RUN=1
# If it looks right, run:
# make install
\`\`\`

Pass \`PREFIX=somepath\` to change the installation prefix from the default which is
\`/usr/local\` or \`~/.local\` if \`/usr/local\` is not writable with the current user.

## Uninstall

\`\`\`sh
make uninstall DRY_RUN=1
# If it looks right, run:
# make uninstall
\`\`\`
EOF
