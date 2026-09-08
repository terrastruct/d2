# TALA corpus performance and equivalence

This standalone command reads a caller-supplied corpus without changing E2E
tests or goldens. Keep private diagram sources and licensed fonts outside this
repository. The corpus is a JSON array:

```json
[
  {"name": "example", "source": "a -> b: hello", "seed": 0},
  {"name": "example/sketch", "source": "a -> b", "seed": 3, "sketch": true, "theme": 0}
]
```

Each case must have a unique nonempty name. `skip: true` explicitly excludes a
case. Preserve original source, seed, theme, sketch settings and font bytes when
extracting an existing suite. The runner invokes `d2lib.Compile` with UTF-16
positions, SourceCodePro as the monospace family, the supplied font as the
regular family, scale 1, and one explicit seed with concurrency 1. Sketch cases
use D2's sketch font. Diagram configuration retains its normal precedence.

Build the identical harness source against an untouched baseline and the
candidate revision. Use the same Go toolchain, hardware, `GOMAXPROCS`, corpus,
font files and arguments, and keep other CPU-heavy work stopped during timing.
Alternate baseline/candidate runs to reduce thermal and scheduling bias. For
example, from each checkout:

```sh
go build -o /tmp/tala-baseline ./ci/tala-performance
GOMAXPROCS=1 /tmp/tala-baseline \
  -corpus /private/corpus.json -count 3 \
  -output /private/baseline.jsonl
```

An external font can be loaded with `-font-regular /private/regular.ttf
-font-bold /private/bold.ttf -font-name d2Font`. Without a font, D2's default is
used. Record the SHA-256 of those font files alongside the corpus and source
revision; the command prints the corpus hash, toolchain and `GOMAXPROCS`.

Every repetition reparses, measures and lays out a fresh graph. The command
reports nanoseconds per diagram for:

- `layout_ns`: time inside TALA `Layout`, including layout's own edge routing,
  adapter work and complete deterministic seed selection.
- `routing_ns`: additional `RouteEdges` callbacks requested by D2's nested
  layout integration during compilation.
- `compile_ns`: the whole compile operation, including parser, text measurement,
  nested-layout orchestration, TALA and target export.
- `render_ns`: SVG rendering after compilation.

`layout_ns + routing_ns` measures TALA's contribution to the compile path.
Summed per-diagram times describe a sequential corpus run, not the wall time of
the parallel Go E2E test runner. Setup and fingerprint serialization are outside
these timings. Compiler and renderer work limits the speedup of the full suite
even when TALA becomes faster.

The default matches a compile-and-render E2E path. `-reroute` adds a separate
rerouting pass over the root board's edges after compilation and records its
time and output separately; this extra pass is not included in the TALA compile
total. It does not reroute the child boards.

Results include SHA-256 hashes of the input case, recursively serialized D2
graphs, complete exported diagram JSON, and rendered SVG bytes. These cover
shape geometry, route points, labels, styles, ordering and nested boards. Use
`-artifacts /private/baseline-artifacts` to retain complete outputs for inspecting
a mismatch. Artifact filenames are hashes of case names. These outputs can
contain private content; they should remain outside the repository. If a
downstream application converts D2 into another model, that application-specific
conversion is outside this harness.

Compare runs with:

```sh
python3 ci/tala-performance/compare.py \
  /private/baseline.jsonl /private/candidate.jsonl \
  --csv /private/per-diagram.csv
```

The comparison rejects missing cases, incomplete repetitions, duplicate records,
changed inputs/outputs, nondeterministic repeated outputs and errors. It reports
each repetition's total and the ratio of the median totals. Per-case CSV timings
use each case's median. Preserve all failures, including pre-existing baseline
failures, in the report; matching errors do not prove a diagram's equivalence.
To acknowledge a known baseline rejection, supply `--expected-errors
/private/expected-errors.json`, a mapping from case name to its exact error
message. The case remains in the corpus and the failed-case report. Both sides
must produce exactly that error; a different error or unexpected success fails
the comparison. This never counts a rejected diagram as a successful layout.

For attribution, run a separate profile pass:

```sh
GOMAXPROCS=1 /tmp/tala-baseline -corpus /private/corpus.json \
  -output /private/profile.jsonl -cpuprofile /private/cpu.pprof \
  -memprofile /private/alloc.pprof
go tool pprof -top -tagfocus=phase=layout /tmp/tala-baseline /private/cpu.pprof
go tool pprof -top -alloc_space /tmp/tala-baseline /private/alloc.pprof
```

CPU samples are labeled `phase=layout`, `phase=routing` or `phase=reroute` for
attribution; a profile without a phase filter includes compilation, rendering
and fingerprint work. The allocation profile covers the complete process.
Do not use a profile pass as the final unprofiled speedup measurement.
