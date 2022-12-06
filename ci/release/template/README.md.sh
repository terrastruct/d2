#!/bin/sh
set -eu

cat <<EOF
# d2

For docs, more installation options and the source code see https://oss.terrastruct.com/d2

version: $VERSION

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
