package layoutgraph

// ClusterGeometryWork is the mutation contract used while synchronizing
// cluster geometry. Layoutgraph owns the geometry; callers that need guarded
// descendant movement provide the operations on top of the shared work budget.
type ClusterGeometryWork interface {
	SharedWorkStepper
	MoveNodeWithChildren(node *Node, dx, dy float64) error
	PositionContainerChildren(node *Node) error
}

type unmeteredGroupGeometryWork struct{}

func (unmeteredGroupGeometryWork) Step() error   { return nil }
func (unmeteredGroupGeometryWork) Finish() error { return nil }

func (unmeteredGroupGeometryWork) MoveNodeWithChildren(node *Node, dx, dy float64) error {
	node.moveNodeWithChildren(dx, dy)
	return nil
}

func (unmeteredGroupGeometryWork) PositionContainerChildren(node *Node) error {
	node.positionContainerChildren(false)
	return nil
}

var unmeteredGroupGeometry = unmeteredGroupGeometryWork{}
