package routing

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type TreeEdgePath struct {
	SourcePortNode            *OVGNode
	TargetPortNode            *OVGNode
	SourceMidpoint            *geo.Point
	TargetMidpoint            *geo.Point
	SourceOrientationToTarget geo.Orientation
}

func isSentinelEdgeSource(tree *layoutgraph.Tree) bool {
	return tree != nil && tree.SentinelEdge != nil && tree.SentinelEdge.From == tree.Node
}

// get the S-shaped path information for the tree node's sentinelEdge
func treeEdgePath(t *layoutgraph.Tree, portNodes map[*layoutgraph.Node][]*OVGNode, portArrowheads map[*OVGNode]map[layoutgraph.Arrowhead]struct{}) TreeEdgePath {
	path, _ := treeEdgePathForBuild(t, portNodes, portArrowheads, nil)
	return path
}

func treeEdgePathForBuild(t *layoutgraph.Tree, portNodes map[*layoutgraph.Node][]*OVGNode, portArrowheads map[*OVGNode]map[layoutgraph.Arrowhead]struct{}, guard *ovgBuildGuard) (TreeEdgePath, error) {
	step := func() error {
		if guard == nil {
			return nil
		}
		return guard.step()
	}
	if err := step(); err != nil {
		return TreeEdgePath{}, err
	}
	parent := t.SentinelNode()
	child := t.Node

	parentPortIndex := parent.CenterPortIndex(t.Orientation)
	childPortIndex := child.CenterPortIndex(t.Orientation.GetOpposite())
	parentPortNode := portNodes[parent][parentPortIndex]
	childPortNode := portNodes[child][childPortIndex]

	if arrowheads, has := portArrowheads[parentPortNode]; has {
		// check the arrowhead on the parent
		arrowhead := t.SentinelEdge.TargetArrowhead
		if !isSentinelEdgeSource(t) {
			arrowhead = t.SentinelEdge.SourceArrowhead
		}
		if len(arrowheads) > 0 {
			if _, in := arrowheads[arrowhead]; !in {
				var closestPort *OVGNode
				closestDistance := math.Inf(1)
				// try to find a different port since we have a mismatched arrowhead
				for _, portIndex := range parent.PortIndices(t.Orientation) {
					if err := step(); err != nil {
						return TreeEdgePath{}, err
					}
					if portIndex == parentPortIndex {
						continue
					}
					usePort := false
					port := portNodes[parent][portIndex]
					_, hasArrowheads := portArrowheads[port]
					if !hasArrowheads {
						usePort = true
					} else if _, matches := portArrowheads[port][arrowhead]; matches {
						usePort = true
					}
					if usePort {
						distance := geo.EuclideanDistance(
							port.Point.X,
							port.Point.Y,
							childPortNode.X,
							childPortNode.Y,
						)
						if distance < closestDistance {
							closestPort = port
							closestDistance = distance
						}
					}
				}
				if closestPort != nil {
					parentPortNode = closestPort
				}
			}
		}
	}

	childPortNode, err := alignedPortNodeForBuild(t, parentPortNode, childPortNode, guard)
	if err != nil {
		return TreeEdgePath{}, err
	}
	// alignedPortNodeForBuild may synthesize a port so that a nearly aligned tree
	// edge stays straight. addTreeNodes canonicalizes that coordinate into the
	// OVG and records it in the child's port list. Resolve it here on subsequent
	// route lookups; otherwise child-as-source routing starts from a fresh node
	// with no OVG edges, and shared points lose their canonical owner metadata.
	for _, portNode := range portNodes[child] {
		if err := step(); err != nil {
			return TreeEdgePath{}, err
		}
		if nonNilEquals(portNode.Point, childPortNode.Point) {
			childPortNode = portNode
			break
		}
	}

	parentMidpoint, childMidpoint := treeEdgeMidpoints(parent, t.Orientation, parentPortNode.Point, childPortNode.Point)

	// if we are the target, swap these around
	if isSentinelEdgeSource(t) {
		return TreeEdgePath{
			SourcePortNode:            childPortNode,
			TargetPortNode:            parentPortNode,
			SourceMidpoint:            childMidpoint,
			TargetMidpoint:            parentMidpoint,
			SourceOrientationToTarget: t.Orientation,
		}, nil
	} else {
		return TreeEdgePath{
			SourcePortNode:            parentPortNode,
			TargetPortNode:            childPortNode,
			SourceMidpoint:            parentMidpoint,
			TargetMidpoint:            childMidpoint,
			SourceOrientationToTarget: t.Orientation.GetOpposite(),
		}, nil
	}
}

func alignedPortNodeForBuild(tree *layoutgraph.Tree, parentPort, childPort *OVGNode, guard *ovgBuildGuard) (*OVGNode, error) {
	isWithinThreshold := func(a, b, t float64) bool {
		return math.Abs(a-b) <= t
	}
	parent := tree.SentinelNode()
	child := tree.Node

	endOfNodeBuffer := 5.

	// in some cases, parent.center = (100.5, 100) and child.center = (100, 150)
	// however, this is just a rounding error and then we compare equality by upto 1 in difference
	nodesAreVerticallyAligned := isWithinThreshold(parent.Center().X, child.Center().X, treeChildAlignmentTolerance)
	portsAreVerticallyMisaligned := parentPort.X != childPort.X
	if tree.Orientation.IsVertical() && nodesAreVerticallyAligned && portsAreVerticallyMisaligned {
		if child.TopLeft.X+endOfNodeBuffer < parentPort.X && parentPort.X < child.TopLeft.X+child.Width-endOfNodeBuffer {
			if guard != nil {
				return guard.newDerivedNode(geo.NewPoint(parentPort.X, childPort.Y))
			}
			return NewOVGNode(geo.NewPoint(parentPort.X, childPort.Y)), nil
		}
	}

	nodesAreHorizontallyAligned := isWithinThreshold(parent.Center().Y, child.Center().Y, treeChildAlignmentTolerance)
	portsAreHorizontallyMisaligned := parentPort.Y != childPort.Y
	if tree.Orientation.IsHorizontal() && nodesAreHorizontallyAligned && portsAreHorizontallyMisaligned {
		if child.TopLeft.Y+endOfNodeBuffer < parentPort.Y && parentPort.Y < child.TopLeft.Y+child.Height-endOfNodeBuffer {
			if guard != nil {
				return guard.newDerivedNode(geo.NewPoint(childPort.X, parentPort.Y))
			}
			return NewOVGNode(geo.NewPoint(childPort.X, parentPort.Y)), nil
		}
	}

	return childPort, nil
}

// treeEdgeMidpoints computes the two bends in a tree edge's S-shaped route.
// The bend distance is relative to the parent, even when the child is farther
// away to accommodate an edge label.
func treeEdgeMidpoints(parent *layoutgraph.Node, treeOrientation geo.Orientation, parentPortPosition, childPortPosition *geo.Point) (*geo.Point, *geo.Point) {
	parentMidpoint := new(geo.Point)
	childMidpoint := new(geo.Point)
	switch treeOrientation {
	case geo.Left:
		parentMidpoint.Y = parentPortPosition.Y
		childMidpoint.Y = childPortPosition.Y
		centerX := parent.TopLeft.X - layoutgraph.TreeParentSpacing/2
		parentMidpoint.X = centerX
		childMidpoint.X = centerX
	case geo.Right:
		parentMidpoint.Y = parentPortPosition.Y
		childMidpoint.Y = childPortPosition.Y
		centerX := parent.TopLeft.X + parent.Width + layoutgraph.TreeParentSpacing/2
		parentMidpoint.X = centerX
		childMidpoint.X = centerX
	case geo.Top:
		parentMidpoint.X = parentPortPosition.X
		childMidpoint.X = childPortPosition.X
		centerY := parent.TopLeft.Y - layoutgraph.TreeParentSpacing/2
		parentMidpoint.Y = centerY
		childMidpoint.Y = centerY
	case geo.Bottom:
		parentMidpoint.X = parentPortPosition.X
		childMidpoint.X = childPortPosition.X
		centerY := parent.TopLeft.Y + parent.Height + layoutgraph.TreeParentSpacing/2
		parentMidpoint.Y = centerY
		childMidpoint.Y = centerY
	}
	return parentMidpoint, childMidpoint
}
