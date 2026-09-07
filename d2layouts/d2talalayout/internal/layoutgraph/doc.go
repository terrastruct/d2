// Package layoutgraph defines the mutable state used by TALA's layout
// algorithms.
//
// The package is deliberately independent of the algorithms that mutate this
// state. That dependency direction lets placement, routing, and other layout
// domains share one graph model without depending on the engine pipeline.
package layoutgraph
