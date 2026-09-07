#### Features 🚀

- exports: render PNG, GIF, PDF, and PPTX with the built-in renderer

#### Improvements 🧹

- performance: reduce repeated work in Dagre ranking, text measurement, SVG preparation, and PNG encoding
- performance: reduce repeated work in selective imports, dynamic grid layout, and PNG connection rendering
- exports: reduce PNG memory usage and support images above the previous 67-megapixel limit
- tala: replace the Dagre-derived hierarchy ranker with an independently written deterministic network-simplex implementation based on the published algorithm, remove the fixed iteration cap, and remove TALA's Dagre license notice
- tala: consume D2's canonical sequence-lifeline ID API and remove the private signed-ID compatibility path
- tala: make D2 the only supported integration boundary: keep only options, layout, and routing public; remove the standalone SeedGraph transport and worker protocol, unreachable prearranged and fixed-size modes, the vestigial containment stage, and test-only topology transactions
- tala: remove per-stage layout JSON goldens and their acceptance and inventory harness; retain input/final layout goldens and focused stage-invariant tests
- tala: make internal graph fixtures current-only: reject unknown and trailing JSON, require unambiguous edge IDs, and remove obsolete arrowhead spellings and redundant adjacency, hierarchy, label-position, and vessel fields
- tala: require explicit non-negative work limits: reject negative limits and treat zero as a zero-work budget instead of an unbounded compatibility mode
- tala: enforce the shared placement work budget across disconnected-cluster discovery, overlap checks, and subtree movement, restoring exact geometry when the first over-limit unit follows an accepted move
- tala: restore exact gap-normalization geometry, cell size, and placement-cost state when a work-limit error, cancellation, or panic interrupts the stage after an earlier accepted move
- tala: restore exact cluster geometry, policy, cell size, and placement-cost state when an error, cancellation, or panic interrupts cluster optimization
- tala: restore exact stage-entry geometry when a work-limit error, cancellation, or panic interrupts symmetry balancing after an earlier node move
- tala: reuse bounded per-level scratch while counting graph edge crossings without changing crossing results or cancellation checks. On Apple M4, `big_many_subgraphs` uses 40.4 MB and 107,675 fewer allocations per layout, while `us_map` uses 12.5 MB and 46,236 fewer; four representative layout medians show no regression
- tala: restore exact node geometry and edge routes when cancellation or a panic interrupts the Dejitter placement stage
- tala: restore exact node geometry when cancellation or a panic interrupts disconnected-cluster joining during placement
- tala: move a single-node edge-port-swap adapter used only by routing unit tests into the test file; the whole-graph production stage and guarded swap core are unchanged
- tala: move an even-distribution helper used only by routing unit tests into the test file; guarded production balancing is unchanged
- tala: move an OVG port-orientation helper used only by one routing unit test into the test file; guarded production port filtering is unchanged
- tala: move two sizeless-optimizer work-guard adapters used only by their unit tests into the test file; guarded production paths are unchanged
- tala: remove a stale unused routing box-overlap helper reintroduced after callers moved to the shared `geo.Box` implementation; live routing overlap checks are unchanged
- tala: remove an unused node-level unrounded-bounding-box delegate orphaned by the mechanical bounding-box hot-path rewrite; live graph and node-list bounding-box paths are unchanged
- tala: move a node-to-point distance convenience method used only by its layoutgraph unit test into the test file; production distance helpers are unchanged
- tala: move a transaction constructor convenience method used only by layoutgraph resource tests into the test file; options-aware production paths are unchanged
- tala: move two unguarded reachability methods used only by layoutgraph unit tests into the test file; context-aware production paths are unchanged
- tala: move two unguarded label-placement convenience functions used only by their unit tests into the test file; checked production paths are unchanged
- tala: move two unguarded OVG edge-set convenience methods used only by their unit tests into the test file; guarded production paths are unchanged
- tala: remove three unused layoutgraph copies of placement math helpers left by engine domain modularization
- tala: move two route-blocker convenience wrappers used only by the BinPack cancellation regression into its test file; the production delta-aware guarded route blocker is unchanged
- tala: bypass node-order sorting for empty and singleton lists. In Apple M4 `Cleanup` microbenchmarks, zero- and one-group layouts use 48 fewer bytes and two fewer allocations per operation, while eight-cluster timing remains neutral
- tala: remove test-only routing constructors from production and make explicit-limit port swapping private
- tala: remove orphaned layoutgraph angle helpers and their self-test left behind by engine modularization
- tala: remove two unused private edge-routing angle helpers
- tala: move test-only edge-routing convenience wrappers out of the production routing implementation
- tala: require caller-owned context-derived OVG build guards in the remaining private routing helpers, keeping cancellation and resource accounting explicit
- tala: require live OVG build guards in contextless routing helpers instead of silently creating uncancelable fallback guards
- tala: remove unused internal node-overlap, inclusive-containment, route-formatting, and Fibonacci-heap allocation helpers
- tala: filter retired cluster vessels from graph and container lists once per reset instead of rescanning every list for every cluster. On Apple M4, `ResetClusters` is 2.54x faster for an 8-cluster relayout and 44.0x faster for a 512-cluster stress case, with unchanged allocations and no single-cluster regression
- tala: remove the unused full transaction-clone API and its snapshot-cloning machinery, leaving geometry-only speculative cloning as the single supported path
- tala: avoid allocating and sorting temporary slices when optimizer median scoring has one neighbor. On Apple M4, representative placement uses 8,414 fewer allocations per operation (3.99%) and 1.02% fewer bytes with neutral runtime
- tala: reuse one shared read-only layout-stage plan, time stages only when benchmark instrumentation requests it, and avoid allocating route-snapshot observers when snapshots are disabled. On Apple M4, pipeline construction falls from 42 to 5 allocations (12,800 to 10,976 B/op)
- tala: make raw graph serialization and deserialization consistently context-first and remove duplicate non-cancelable wrappers
- tala: move sized-optimizer testing conveniences out of the production placement implementation
- tala: remove the test-only contextless sequence-identification facade from the production grouping API
- tala: make deliberately unmetered internal graph traversals use a shared no-op work stepper instead of allocating and polling an uncancelable guard
- tala: adopt test-owned contexts across TALA facade and engine tests, and use Go 1.27 benchmark loops in engine and placement benchmarks
- tala: replace reflection-based slice sorting and manual slice copies with typed standard-library helpers while preserving deterministic tie ordering and layout output. In an Apple M4 1,024-identity sort microbenchmark, median time falls 37.3% (40.5µs to 25.4µs) and reflection overhead falls from 104 B/3 allocations to zero
- tala: restore exact node graph ownership when placement fails after temporarily splitting Near-connected subgraphs
- tala: accelerate transaction validation, node placement, and orthogonal edge routing while preserving existing layout output and resource-limit behavior. Median Apple M4 benchmarks improve `label_positions` E2E by 7.2×, `nesting_power` resource rejection by 14.6×, and `tables_slow_edge_routing` by 5.5× while reducing allocated bytes by 95.5% and allocations by 96.1%
- tala: reduce allocation overhead in placement scoring, table-column geometry, bounding boxes, topology validation, and empty-route checks while preserving existing layout output. Median Apple M4 single-CPU benchmarks improve `tables_slow_edge_routing`, `18tables`, and `big_connected` by 16.6%, 15.5%, and 10.1%; allocated bytes fall by 27.3–56.5% and allocations by 57.8–70.5%
- performance: SVG rendering is approximately 10× faster across the E2E corpus. Real-world diagrams compile, lay out, and render approximately 3× faster at the median.
- renders: SVG exports are approximately 24% smaller across the E2E corpus and 18% smaller across real-world fixtures, with unchanged appearance.
- renders: render Markdown labels as native SVG instead of HTML `foreignObject` content

#### Bugfixes ⛑️

- tala: count actual routing index work instead of eliminated comparisons, allowing large diagrams such as TPMJS and `nesting_power` to render within the existing resource limits
- tala: select the lowest-penalty seed independently of completion order, using area and then the later configured seed only for exact ties
- tala: preserve SQL table row connections for reverse arrows between tables and ordinary shapes
- install: reject the obsolete `--tala` installer flag with migration guidance before installing, instead of incorrectly claiming that older D2 releases bundle TALA
- sequence diagrams: keep synthetic lifeline endpoint IDs stable across architectures
- exports: honor `D2_TIMEOUT` during PNG and GIF rendering
- compiler: keep recursive globs out of class and variable definitions and report class reference cycles instead of overflowing the stack
- renders: decode gzip, Brotli, and deflate remote images before embedding them
- tala: restore straight-edge fallback routes and routing-cost caches when cancellation or panic interrupts the stage
- tala: restore duplicate-edge routes when duplicate reordering panics after a tentative swap
- tala: propagate request cancellation while snapshotting subgraph positions during compaction
- tala: restore exact external node ownership after hierarchy discovery and placement traverse temporary edge- or Near-connected subgraphs

- tala: reconcile overlapping sibling placement groups before choosing their shared side, fixing the Lion Reader layout failure

---

For the latest d2.js changes, see separate [changelog](https://github.com/d2lang/d2/blob/master/d2js/js/CHANGELOG.md).
