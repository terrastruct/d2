#!/bin/sh
set -eu

export REPORT_OUTPUT="out/e2e_report.html"
rm -f $REPORT_OUTPUT
export E2E_REPORT=1

FORCE_COLOR=1 DEBUG=1 go run ./e2etests/report/main.go "$@";

if [ -z "${NO_OPEN:-}" ]; then
  if [ -s "$REPORT_OUTPUT" ]; then
    open "$REPORT_OUTPUT" 2>/dev/null || \
    xdg-open "$REPORT_OUTPUT" 2>/dev/null || \
    echo "could not open $REPORT_OUTPUT with commands 'open' or 'xdg-open'"
  else
    echo "The report is empty"
  fi
fi
