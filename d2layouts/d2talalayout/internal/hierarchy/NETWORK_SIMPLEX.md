# Hierarchy rank assignment

`rankDAG` solves the integer program

```text
minimize  sum(weight(e) * (rank(head(e)) - rank(tail(e))))
subject to rank(head(e)) - rank(tail(e)) >= 1
```

for a connected simple DAG. Authored hierarchy weights are retained as the
objective coefficients.

## Algorithm

The implementation follows the graphical network-simplex ranker in section
2.3 of Gansner, Koutsofios, North, and Vo, [A Technique for Drawing Directed
Graphs](https://doi.org/10.1109/32.221135), IEEE Transactions on Software
Engineering 19(3), 1993:

1. Stable Kahn traversal produces a longest-path feasible ranking.
2. A tight spanning tree is constructed by repeatedly shifting the current
   tight tree by the smallest boundary slack (figure 2-2 of the paper).
3. A negative-cut tree edge leaves the basis. The minimum-slack edge directed
   from its head component to its tail component enters, and the head
   component is shifted until that edge is tight (figure 2-1).
4. When every tree cut value is nonnegative, the ranks are normalized so the
   minimum is zero.

The optional crowd-balancing pass in the paper is not part of this ranker. It
changes which optimum is selected without improving the weighted-span
objective.

## Determinism and termination

Nodes are ordered by stable node ID and pivot candidates by stable edge ID.
For degenerate zero-shift exchanges, the smallest-ID negative-cut tree edge is
chosen first and minimum-slack entering ties use the smallest edge ID. This is
the finite pivot ordering from Robert G. Bland, [New Finite Pivoting Rules for
the Simplex Method](https://doi.org/10.1287/moor.2.2.103), Mathematics of
Operations Research 2(2), 1977. There is no topology-independent pivot cap;
the shared optimization work guard bounds work and polls cancellation.

## Exact optimality certificate

At termination, each tree cut value is the unique flow on that tight tree edge
that satisfies the node supplies induced by all authored edge weights. The
ranker verifies that these flows are nonnegative, recomputes every node
balance, and compares the resulting dual value with an independently computed
primal weighted-span value. Equality certifies exact optimality.

All rank, slack, cut, flow, and objective arithmetic uses checked `int64`
operations. Validation and result construction preserve the public error and
normalization contracts used by the hierarchy pipeline.
