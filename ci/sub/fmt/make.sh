#!/bin/sh
set -eu
. "$(dirname "$0")/../lib.sh"
PATH="$(cd -- "$(dirname "$0")" && pwd)/../bin:$PATH"

set_changed_files
gomod_path="$(search_up go.mod || true)"
if [ "$gomod_path" ]; then
  export CI_FMT_GO_MODULE=1
  module_name="$(cat "$gomod_path" | head -n1 | cut -d' ' -f2 )"
  if [ "${CI_GOIMPORTS_LOCAL:-}" ]; then
    export CI_GOIMPORTS_LOCAL="$CI_GOIMPORTS_LOCAL,$module_name"
  else
    export CI_GOIMPORTS_LOCAL="$module_name"
  fi
fi
if search_up package.json > /dev/null; then
  export CI_FMT_NODE_MODULE=1
fi
if < "$CHANGED_FILES" grep -qm1 '\.go$'; then
  export CI_FMT_GO=1
fi
if < "$CHANGED_FILES" grep -qm1 '\.md$'; then
  if [ -z "${CI:-}" ]; then
    # Only locally for now.
    export CI_FMT_MARKDOWN=1
  fi
fi
if < "$CHANGED_FILES" grep -qm1 '\.\(js\|jsx\|ts\|tsx\|scss\|css\|html\)$'; then
  export CI_FMT_PRETTIER=1
fi
_make -f "$(dirname "$0")/Makefile" "$@"
