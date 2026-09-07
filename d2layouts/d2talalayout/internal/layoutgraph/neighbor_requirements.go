package layoutgraph

// LongDistanceNeighborRequirements records the space a node needs to expose
// its parallel edges to a distant neighbor.
type LongDistanceNeighborRequirements struct {
	EdgeCount int
	MaxWidth  int
	MaxHeight int
}
