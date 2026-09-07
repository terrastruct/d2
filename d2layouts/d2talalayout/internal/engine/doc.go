// Package engine composes TALA's layout stages into one deterministic layout.
//
// Graph ownership belongs to layoutgraph, and individual algorithms live in
// their domain packages. Engine owns only stage ordering and the internal
// layout entry point.
package engine
