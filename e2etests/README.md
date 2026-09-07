# e2etests

`e2etests` test the end-to-end flow of turning D2 scripts into a rendered diagram

Tests fall under 1 of 4 categories:

1. **Stable**. Scripts which produce diagrams that never had issues this major release.
2. **Regressions**. Scripts which used to have issues but no longer do. Each one should be
   linked to the PR which fixed it.
3. **Todos**. Scripts which have an issue. If the issue prevents compile, `skip: true` can
   be set, otherwise the issue is visual. Each one should be linked to a Github Issue
   which describes it.
4. **Real-world**. Substantial diagrams from other projects, preserved with source
   revisions, adaptations, and licenses in
   [`testdata/files/REAL_WORLD.md`](testdata/files/REAL_WORLD.md). Each fixture runs
   through both Dagre and ELK using the same checks as the other e2e groups.

Upon a major release, Regressions are carried over to Stable.

Diagram fixtures have a sketch SVG and, when their resources are bundled with
D2, a native isometric SVG golden for each layout. The isometric render uses the
compiled source layout, container hierarchy and a fitted canvas up to 1200 × 1200
at time zero. Root boards use
`isometric.exp.svg`; additional boards use source-order paths such as
`isometric.layers.0.steps.1.exp.svg`. Their names remain in `board.exp.json`.
SVG comparisons check the encoded bytes for deterministic output.
The `asciitxtar` fixtures check SVG, standard ASCII and Unicode text only; they
do not produce isometric snapshots.

The isometric suite has 584 SVG snapshots. It uses D2's existing bundled fonts
and embedded image data without downloading artwork or discovering system fonts.
For 92 boards that require external artwork or additional glyphs, the suite
checks explicit resource errors listed in `isometric_test.go` instead. Unexpected
errors still fail, and a fixture that becomes self-contained must regain its SVG
snapshot. Regular/sketch coverage is unchanged. Dedicated renderer and CLI tests
cover image loading and supplied font fallback separately.

Run a focused case, then inspect its changed images:

```sh
go test ./e2etests -run '^TestE2E/stable/all_shapes$' -count=1
go run ./e2etests/report -skip-tests -delta -variant isometric -test-set '^stable$' -test-case '^all_shapes$'
open ./e2etests/out/e2e_report.html
```

After reviewing the changes, accept the selected case's goldens and rerun without
the acceptance flag. Acceptance also updates any changed SVG/board goldens:

```sh
TESTDATA_ACCEPT=1 go test ./e2etests -run '^TestE2E/stable/all_shapes$' -count=1
go test ./e2etests -run '^TestE2E/stable/all_shapes$' -count=1
```

`TA=1` is an alias for `TESTDATA_ACCEPT=1`. `SKIP_ISOMETRIC_CHECK=1` opts out of the
native isometric SVG check; `SKIP_SVG_CHECK=1` skips SVG comparisons, including
isometric SVGs.

Native PNG coverage reuses the compiled layouts from 11 representative fixtures,
with 20 snapshots in the separate `png/testdata` bundle:

```sh
go test ./e2etests/png -count=1
```

PNG comparisons use decoded pixels. Dimensions and alpha must match exactly;
to allow cross-architecture antialiasing rounding, at most 0.01% of pixels may
differ by one RGB value. Smaller images below 10,000 pixels remain exact.
After reviewing PNG changes, use `TESTDATA_ACCEPT=1 go test ./e2etests/png -count=1`
to accept them, then rerun without the acceptance flag.

The main report displays only SVG snapshots and ignores legacy PNG files.
It defaults to `-variant all`; `sketch` shows ordinary/sketch SVGs and `isometric`
shows native isometric SVGs, including every multiboard image. `-delta` includes
changed `.got` files and new images without an expected golden, and ignores
byte-identical stale `.got` files. These options filter the report, not which
snapshots tests generate.
Omit `-skip-tests` to run tests first; `-timeout 10m` controls that run. `TEST_DIR`
and `REPORT_OUTPUT` override the test directory and report path.
