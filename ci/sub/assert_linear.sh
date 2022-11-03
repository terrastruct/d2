#!/bin/sh
set -eu
. "$(dirname "$0")/lib.sh"
cd -- "$(dirname "$0")/.."

# assert_linear.sh ensures that the current commit does not contain any PR merge commits
# compared to master as if it does, then that means our diffing mechanisms will be
# incorrect. We want all changes compared to master to be checked, not all changed
# relative to the previous PR into this branch.

if [ "$(git rev-parse --is-shallow-repository)" = true ]; then
  git fetch --unshallow origin master
fi

merge_base="$(git merge-base HEAD origin/master)"
merges="$(git --no-pager log --merges --grep="Merge pull request" --grep="\[ci-base\]" --format=%h "$merge_base"..HEAD)"

if [ -n "$merges" ]; then
  echoerr <<EOF
Found merge pull request commit(s) in PR: $(_echo "$merges" | tr '\n' ' ')
  Each pull request must be merged separately for CI to run correctly.
EOF
  exit 1
fi
