package layoutgraph

// Shared geometry invariants used by multiple layout stages.
const (
	// AxisAlignmentTolerance is the maximum coordinate difference still treated
	// as axis-aligned.
	AxisAlignmentTolerance = 1.0
	// NodeGap is the minimum gap between unconnected nodes.
	NodeGap = 20
	// ConnectedNodeGap is the minimum gap between connected nodes.
	ConnectedNodeGap = 60
	// TableNodeGap leaves enough room for table-edge routing.
	TableNodeGap = 2 * ConnectedNodeGap
	// ContainerPadding is the ordinary padding on each side of a container.
	ContainerPadding = 60

	// MaxIconSize caps positioned icon geometry.
	MaxIconSize = 64

	// MinArrowheadClearance is the minimum route length needed to fit an arrowhead.
	MinArrowheadClearance = 20.0
	// MinPortClearance keeps the first route bend away from its port.
	MinPortClearance = 20.0
	// MinRouteNodeClearance keeps visibility-graph points away from node bounds.
	MinRouteNodeClearance = 20.0
	// TreeParentSpacing is the fixed gap used to place and route tree levels.
	TreeParentSpacing = 100.0

	// CrossingCostWeight is the shared penalty for one edge crossing.
	CrossingCostWeight = 0.48 * 0.48 * 0.48
)
