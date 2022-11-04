#!/bin/sh
set -eu

export REPORT_OUTPUT="out/e2e_report.html"
rm -f $REPORT_OUTPUT

FORCE_COLOR=1 DEBUG=1 go run ./e2etests/report/main.go "$@";

if [ -z "${NO_OPEN:-}" ]; then
  if [ -s "$REPORT_OUTPUT" ]; then
    open "$REPORT_OUTPUT"
  else
    echo "The report is empty"
  fi
fi
