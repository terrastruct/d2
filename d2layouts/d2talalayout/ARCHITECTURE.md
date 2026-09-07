# TALA architecture

TALA is a D2 layout and edge-routing engine. Its public API follows the other
packages under `d2layouts`: options plus layout and routing entry points. TALA
keeps seed coordination, scoring, its mutable layout graph, and its algorithms
private so they can evolve without becoming part of D2's API.

## Boundaries

The runtime is divided by concrete layout-engine responsibilities:

1. `d2talalayout` validates options and translates a D2 graph into TALA's graph
   representation.
2. Each seed runs the ordered TALA placement, routing, and labeling stages on
   an independent graph clone.
3. Completed graphs are validated and scored. The adapter applies the selected
   graph's geometry and routes to the original D2 graph.
4. `nodeshape` owns TALA's shape-specific port and label-position policy on top
   of D2's shared geometric shape implementations.

The source tree enforces that direction with Go's `internal` mechanism:

```text
d2layouts/d2talalayout/
  *.go                   D2 translation and public layout adapter
  internal/engine/       ordered layout pipeline and stage orchestration
  internal/layoutgraph/  graph, node, edge, topology, and shared geometry
  internal/graphbounds/  work-accounted node-set and fixed-origin bounds
  internal/grouping/     cluster and sequence discovery and lifecycle
  internal/proximity/    near, hub, and herd relationships
  internal/trees/        tree discovery, preprocessing, and placement
  internal/hierarchy/    DAG discovery, ranking, and layered placement
  internal/placement/    general node placement and refinement
  internal/placementcost/ candidate-geometry objective functions
  internal/packing/      disconnected-component and container packing
  internal/loops/        self-edge routes and their reserved extents
  internal/routing/      visibility graphs, path search, and route cleanup
  internal/labeling/     node, icon, edge, and arrowhead-label placement
  internal/quality/      completed-layout scoring
  internal/graphjson/    graph fixture representation and reconstruction
  internal/nodeshape/    TALA port and label-position behavior by shape
  internal/labelgeom/    renderer-exact arrowhead-label geometry
  internal/limits/       shared work budgets and checked arithmetic
  internal/invariant/    shared invariant-error classification
  internal/typedpool/    typed object-pool helper
  internal/testdata/     shared layout, routing, fuzz, and benchmark fixtures
```

The layout package may import internal TALA packages. Packages outside
`d2layouts/d2talalayout` use only the public layout adapter; the Go compiler
rejects direct access to internal types.

```text
D2 graph -> adapter -> TALA graph -> direct in-memory clone per seed
                                     -> discover layout structures
                                     -> place + pack nodes
                                     -> route edges + place labels
                                     -> validate + score -> selected graph

selected graph -> adapter validation -> patch original D2 graph
```

## Package rules

- TALA's `Graph`, `Node`, and `Edge` are layout-engine records, not aliases or
  wrappers around `d2graph` types. TALA temporarily removes and reconnects
  entities, creates synthetic placement nodes, and runs seeds independently;
  those operations must not mutate D2's compiler graph.
- The graph records and their invariant-preserving topology mutations stay
  together. Splitting node and edge types into separate packages would create
  cycles or ID-based indirection without adding a useful boundary.
- Runtime cloning is an explicit `layoutgraph.Clone` operation. It copies graph
  ownership and topology directly, without routing ordinary layout work
  through a serialized representation. Raw graph conversion remains an
  explicit `graphjson` operation used by fixtures and developer tooling.
- An algorithm moves into its own package only when that package owns a
  recognizable layout domain and has a one-way dependency on lower-level
  state. Do not introduce generic `model`, `common`, or callback-heavy adapter
  packages merely to move files.
- Packages name their actual dependencies. The engine does not re-export graph
  or algorithm symbols, and graph transactions share the concrete
  `limits.WorkGuard` instead of forwarding through package-local guard layers.
- Reuse D2 leaf value types such as geometry, label positions, and base shapes
  when their semantics are identical. Do not reuse broader D2 graph or style
  records when TALA needs different ownership or only a small subset of them.

The dependency direction is from shared geometry and graph state toward layout
algorithms, then toward the engine pipeline. `graphbounds` owns the exact
work-accounted node-set bounds shared by packing and routing; it depends on
`layoutgraph`, while neither shared package depends on a layout algorithm.
Algorithm packages do not read or write serialized `graphjson` records.
Packing owns disconnected-subgraph arrangement. The few sibling
dependencies describe concrete layout work: placement uses grouping, labeling,
loops, packing, proximity, and trees; routing uses hierarchy, labeling, and
loops; quality uses labeling. Other algorithm packages depend only on lower
graph, geometry, resource, and shape packages.

`placementcost` owns the objective functions used to compare candidate node
geometry. It depends only on graph state and lower geometry, invariant, and
pooling leaves; only `placement` imports it. Placement keeps ownership of
mutations and search strategy, while the graph retains ownership of the score
cache.

`engine` exports only the internal layout operation and its layout-specific
options; the pipeline, stages, snapshots, and timings remain private
implementation.
Layout-algorithm packages never import `engine`. The package-boundary test
enforces this direction and also prevents D2 compiler and renderer records from
leaking into the TALA graph. The narrow `labelgeom` leaf is the deliberate
exception for renderer-exact arrowhead-label geometry; it accepts geometric
values rather than graph records.

## Ownership and mutation

- A caller's D2 graph is mutated only after a completed graph has been
  validated and selected.
- Every seed exclusively owns its mutable TALA graph. Graphs are not shared by
  concurrent layout attempts.
- Cancellation or failure before the final commit leaves the caller's D2 graph
  unchanged.
- Internal seed results own their memory and cannot mutate another attempt
  through aliases.
- Speculative geometry changes restore their complete prior state unless they
  are explicitly accepted. Topology changes are built and validated before
  they replace live state.

## Context and errors

- Public operations and expensive internal operations accept a context.
- Cancellation is cooperative. An engine attempt must stop promptly after
  `ctx.Done()` is closed.
- Context errors remain standard `context.Canceled` or
  `context.DeadlineExceeded` causes wrapped with operation context.
- Speculative candidate rejections are owned by `layoutgraph`; invariant
  failures wrap `invariant.ErrViolation`.
- Errors wrap their causes, so callers can classify them with `errors.Is`.
- Invalid input returns an error. Panics are reserved for impossible programmer
  invariants and are recovered only at a public or goroutine trust boundary.
- The engine does not log diagram data or include stack traces in ordinary
  returned errors.

## Determinism

A graph, options, and seed define a layout attempt. Any map iteration that
affects a decision must be converted to a stable ordering.
Concurrency may change execution time, but it must not change the result.
Completed attempts are ordered by exact penalty, then area; exact ties favor
the later configured seed. Approximate pairwise ties would make that ordering
nontransitive and allow completion order to affect selection.

The number of requested seeds and the number executed concurrently are
separate policies. The public adapter coordinates attempts locally, while one
internal engine run always represents one seed.

Hierarchy ranking is deterministic and does not consume the placement random
stream. `hierarchy/rank_graph.go` validates and indexes a connected simple DAG,
`hierarchy/rank_simplex.go` solves its ranking objective, and `hierarchy/rank.go`
owns the contract and final primal/dual comparison. Keeping those
responsibilities separate makes the mathematical solver replaceable without
coupling graph validation or hierarchy placement to a particular algorithm.

The ranker minimizes the weighted sum of directed edge spans, with every edge
spanning at least one level. It uses the graphical network-simplex method:
longest-path feasible ranks, a tight spanning tree, and minimum-slack exchanges
for negative-cut tree edges. Stable node and edge IDs define every tie. Bland's
smallest-index pivot rule makes degenerate zero-shift exchanges finite without
a topology-independent pivot cap. The shared optimization guard provides the
work bound and cancellation polling instead.

At termination, the nonnegative final tree cuts define the unique tree-supported
dual flow for the authored edge-weight balances. The ranker checks those node
balances and compares the dual value with an independently computed primal
weighted-span objective. Checked integer arithmetic, compact adjacency storage,
stable entity ordering, context polling, and aggregate work accounting keep the
result portable to D2's native and WASM targets.

When several normalized rankings have the same minimum cost, the stable
feasible-tree and Bland pivot choices select one deterministic optimum. The
optional crowd-balancing pass described by Gansner et al. is deliberately not
part of the ranker; normalization only subtracts the minimum returned rank.

The formulation follows Gansner, Koutsofios, North, and Vo's
[DAG ranking model](https://graphviz.org/documentation/TSE93.pdf). Degenerate
pivots use Bland's finite rule from
[New Finite Pivoting Rules for the Simplex Method](https://doi.org/10.1287/moor.2.2.103).
These are algorithmic references; the Go implementation is original to TALA.

## Compatibility

The supported Go boundary is `Options`, `DefaultOptions`, `DefaultLayout`,
`Layout`, and `RouteEdges`. Internal seed, scoring, and engine types are
implementation details. Layout geometry may improve between D2
releases; deterministic seeds make those changes reviewable rather than a
promise that coordinates never change.

## Tests

Public facade tests cover D2 mutation, cancellation, options, and bounded local
coordination. Internal tests cover algorithm invariants and focused stages.
Synthetic final layout goldens are the regression contract; focused stage tests
assert invariants without freezing every intermediate pipeline state.

The neutral `internal/testdata` tree is shared by the engine pipeline, routing,
quality, hierarchy, and adapter validation tests. The engine integration tests
own layout and OVG golden acceptance without making the graph-state package own
algorithm output.
