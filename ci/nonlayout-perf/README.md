# D2 E2E performance without layout

This diagnostic harness runs the **existing active E2E tests**, using Go source
overlays to time their phases and optionally capture inputs for a renderer-only
replay. It preserves their skip rules, assertions, expected errors, geometry
JSON goldens, SVG goldens, and ASCII goldens. No tests or golden files are edited.
Generated binaries, profiles, results, and captured diagrams belong outside the
repository. Python 3.9+, Git, and the repository's Go toolchain are required.

Use an explicit baseline commit, or a separate baseline worktree with `--current`.
There is no default baseline: committing the candidate must not silently change
the control. `--base-ref` restores changed Go sources with an overlay and excludes
Go files added after that ref. It rejects differing `go.mod`/`go.sum` and non-Go inputs outside this harness,
including embedded assets and txtar fixtures; use a separate baseline worktree
when those inputs differ. Instrumentation fails if its
source anchors change, so a new E2E/compiler structure needs an explicit review.

From the repository root, for example:

```sh
BASE=0515f60e985ad829d652456e07831170583011a0
python3 ci/nonlayout-perf/run.py phases --base-ref "$BASE" --out /tmp/d2-base-1 --capture
python3 ci/nonlayout-perf/run.py phases --current --out /tmp/d2-head-1
python3 ci/nonlayout-perf/run.py phases --current --out /tmp/d2-head-2
python3 ci/nonlayout-perf/run.py phases --base-ref "$BASE" --out /tmp/d2-base-2
python3 ci/nonlayout-perf/run.py compare \
  --baseline /tmp/d2-base-1/run-0.json /tmp/d2-base-2/run-0.json \
  --candidate /tmp/d2-head-1/run-0.json /tmp/d2-head-2/run-0.json
```

Each invocation builds its source variant, then starts a fresh test process with
`CI=1`, error-level logging, `GOMAXPROCS=1`, and `-test.parallel 1`.
Inherited golden acceptance (`TESTDATA_ACCEPT`/`TA`), SVG skipping, and diff
suppression settings are cleared so verification cannot silently accept output. It runs from the
E2E directory so relative fixtures resolve correctly. Build time is excluded from
reported process wall/user/system time. `--runs N` repeats fresh processes;
alternating variants helps control system noise. `--gomaxprocs N` changes the CPU
limit. Use a fresh output directory for each invocation; previous results are
never overwritten. Provenance records the source ref, working-tree diff and new
Go source hashes, toolchain, harness files, and binary hash.

The product non-layout metric is:

```
compile_total - layout_excluded + svg_render + animation
```

`layout_excluded` wraps **all of `d2layouts.LayoutNested`**, including nested
placement, routing, and layout-internal sizing. Layout signature/preparation work
outside that call remains included. Compiler, theme, dimensions, and export are
reported as a decomposition of compilation; they are not added a second time.
Setup/serde separately includes ruler construction, its compiler and dimension
passes, graph serialization/deserialization, and JSON equality checking. XML
verification and ASCII rendering are also separate. The combined timed metric
excludes test registration, feature checks, assertions, golden I/O/diffing, capture
and hash/output overhead. It is **not total E2E wall time**.

`compare` requires identical complete output fingerprint sets and per-case phase
invocation counts. Counts are derived from the running suite, not fixed in the
harness. `--profile` adds CPU profiles for diagnosis. Profiling has substantial
cost for this allocation-heavy workload: use unprofiled runs for performance
claims. The labels describe the innermost timed phase; an enclosing compilation
CPU total must not be inferred from these labels.

## Renderer replay with zero layout calls

The capture contains actual pre-render diagrams and finalized options, including
multiboard master IDs. It does not reconstruct inputs or options from goldens.
Use this same corpus for both variants:

```sh
python3 ci/nonlayout-perf/run.py render --base-ref "$BASE" \
  --corpus /tmp/d2-base-1/corpus --oracle /tmp/d2-base-1/run-0.json --out /tmp/d2-render-base-1
python3 ci/nonlayout-perf/run.py render --current \
  --corpus /tmp/d2-base-1/corpus --oracle /tmp/d2-base-1/run-0.json --out /tmp/d2-render-head-1
python3 ci/nonlayout-perf/run.py compare \
  --baseline /tmp/d2-render-base-1/run-0.json \
  --candidate /tmp/d2-render-head-1/run-0.json --threshold 10
```

Use the same alternating order and multiple samples as the phase comparison for
final measurements. Every replay verifies byte-for-byte SHA256/length equality
against the SVGs produced by the original E2E run and checks that rendering did
not mutate the captured input. It reports individual diagram timings, aggregate
render/animation time, allocations, process CPU/wall time, and a distribution of
per-diagram speedups. `--threshold` optionally counts diagrams meeting any chosen
ratio; it does not change the aggregate metric or its interpretation.

Fixture loading and package initialization are outside individual render timers
but inside process time. SHA/input-mutation checks are outside render timers but
inside pass wall/alloc totals. GC is forced before each pass; GC during rendering
is included. `--passes 2` runs first use and then reuse of the same in-memory
inputs in one process; there is no output cache in the harness. Keep first-use and
reuse results separate when interpreting workload behavior.
