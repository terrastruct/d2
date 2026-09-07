# TALA test data

This directory contains TALA's synthetic and public-data fixture corpus. Graph
labels and geometry are synthetic placeholders unless a factual public source
is identified below.

- `layout` contains synthetic layout cases and their deterministic final JSON
  goldens.
- `ovg` contains synthetic orthogonal-visibility-graph cases and exact JSON
  goldens.
- `regression` contains generic structural cases for previously fixed
  algorithm failures.
- `benchmark` contains synthetic, generated, or factual public data. The
  `big_many_subgraphs.json`, `hierarchy_41nodes_50edges.json`, and
  `hierarchy_nested.json` family was deliberately constructed for trees,
  clusters, and hierarchy coverage. Files named
  `nodes*_edges*_containers*_*.json` are generated structural graphs, and
  `us_map.json` contains factual US-state adjacency.

Go fuzz corpus entries live under the owning package, currently
`../engine/testdata/fuzz/FuzzAutolayout`, so ordinary package tests replay them
automatically.

From the D2 repository root, run the ordinary fixture contracts with:

```sh
go test ./d2layouts/d2talalayout/internal/engine ./d2layouts/d2talalayout/internal/routing
```

Accept changed layout goldens only after reviewing the algorithm change:

```sh
TESTDATA_ACCEPT=1 go test ./d2layouts/d2talalayout/internal/engine -run '^TestGraphs/<case>$'
```

Accept changed OVG snapshot goldens only after reviewing the route change:

```sh
TESTDATA_ACCEPT=1 go test ./d2layouts/d2talalayout/internal/engine -run '^TestBuildOVGFromGraph/<case>$'
```
