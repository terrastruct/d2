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
make install
\`\`\`

Pass \`PREFIX=whatever\` to change the installation prefix.

## Uninstall

\`\`\`sh
make uninstall DRY_RUN=1
# If it looks right, run:
make uninstall
\`\`\`
EOF
