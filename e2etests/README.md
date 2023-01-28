# e2etests

`e2etests` test the end-to-end flow of turning D2 scripts into a rendered diagram

Tests fall under 1 of 3 categories:

1. **Stable**. Scripts which produce diagrams that never had issues this major release.
2. **Regressions**. Scripts which used to have issues but no longer do. Each one should be
   linked to the PR which fixed it.
3. **Todos**. Scripts which have an issue. If the issue prevents compile, `skip: true` can
   be set, otherwise the issue is visual. Each one should be linked to a Github Issue
   which describes it.

Upon a major release, Regressions are carried over to Stable.

If a change results in test diffs, you can run this script to generate a visual HTML
report with the old vs new renders.

```
go run ./e2etests/report/main.go -delta
open ./e2etests/out/e2e_report.html
```
