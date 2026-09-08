package routing

import (
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

type OVGNode struct {
	*geo.Point                                // 8 bytes
	Edges      []*OVGEdge                     // 24 bytes
	IsNearPort map[*layoutgraph.Node]struct{} // 8 bytes
	// portOwnersByNode keeps port metadata per graph node. More than one graph node can
	// legitimately have a port at the same coordinate (for example, two shapes
	// whose borders touch). The OVG canonicalizes nodes by coordinate, so a
	// single owner and direction cannot describe that point correctly.
	portOwnersByNode map[*layoutgraph.Node]ovgPortMetadata
	Container        *layoutgraph. // 8 bytes
				Node
	Index        int  // 8 bytes
	IsNodeCenter bool // 1 byte
	IsTunnel     bool // 1 byte
}

type ovgPortMetadata struct {
	directions   portDirectionSet
	isCenterPort bool
}

// A rounded OVG coordinate can represent more than one side of the same node.
// A 1x1 square, for example, has top and left snap points at the same point.
// Keep every valid role instead of letting the last side overwrite the first.
type portDirectionSet uint16

func newPortDirectionSet(direction geo.Orientation) portDirectionSet {
	if direction < geo.TopLeft || direction > geo.NONE {
		return 0
	}
	return 1 << uint(direction)
}

func (directions portDirectionSet) has(direction geo.Orientation) bool {
	return directions&newPortDirectionSet(direction) != 0
}

// any reports whether predicate accepts at least one represented direction.
// Empty metadata is unrestricted and therefore behaves like NONE.
func (directions portDirectionSet) any(predicate func(geo.Orientation) bool) bool {
	if directions == 0 {
		return predicate(geo.NONE)
	}
	for direction := geo.TopLeft; direction <= geo.NONE; direction++ {
		if directions.has(direction) && predicate(direction) {
			return true
		}
	}
	return false
}

func (directions portDirectionSet) transformed(transform func(geo.Orientation) geo.Orientation) portDirectionSet {
	var transformed portDirectionSet
	directions.any(func(direction geo.Orientation) bool {
		transformed |= newPortDirectionSet(transform(direction))
		return false
	})
	return transformed
}

func NewOVGNode(p *geo.Point) *OVGNode {
	// Edges grows lazily. A large share of candidate visibility nodes are
	// rejected before connection, so eagerly allocating four slots here creates
	// substantial garbage without helping surviving nodes.
	return &OVGNode{Point: p}
}

func (n *OVGNode) ensurePortOwners() {
	if n.portOwnersByNode == nil {
		n.portOwnersByNode = make(map[*layoutgraph.Node]ovgPortMetadata, 2)
	}
}

func (n *OVGNode) addPortMetadata(owner *layoutgraph.Node, added ovgPortMetadata) {
	if owner == nil {
		return
	}
	n.ensurePortOwners()

	metadata := n.portOwnersByNode[owner]
	metadata.directions |= added.directions
	metadata.isCenterPort = metadata.isCenterPort || added.isCenterPort
	n.portOwnersByNode[owner] = metadata
}

func (n *OVGNode) addPortOwner(owner *layoutgraph.Node, direction geo.Orientation, isCenterPort bool) {
	n.addPortMetadata(owner, ovgPortMetadata{
		directions:   newPortDirectionSet(direction),
		isCenterPort: isCenterPort,
	})
}

func (n *OVGNode) setCenterPort(owner *layoutgraph.Node) {
	metadata, ok := n.portMetadataFor(owner)
	if !ok {
		return
	}
	metadata.isCenterPort = true
	n.addPortMetadata(owner, metadata)
}

func (n *OVGNode) portMetadataFor(owner *layoutgraph.Node) (ovgPortMetadata, bool) {
	metadata, ok := n.portOwnersByNode[owner]
	return metadata, ok
}

func (n *OVGNode) isPort() bool {
	return len(n.portOwnersByNode) > 0
}

func (n *OVGNode) isPortOf(owner *layoutgraph.Node) bool {
	_, ok := n.portMetadataFor(owner)
	return ok
}

func (n *OVGNode) portDirectionsFor(owner *layoutgraph.Node) (portDirectionSet, bool) {
	metadata, ok := n.portMetadataFor(owner)
	return metadata.directions, ok
}

func (n *OVGNode) hasPortDirection(owner *layoutgraph.Node, direction geo.Orientation) bool {
	directions, ok := n.portDirectionsFor(owner)
	return ok && directions.has(direction)
}

func (n *OVGNode) portDirectionsForObstacle(owner *layoutgraph.Node) portDirectionSet {
	directions, _ := n.portDirectionsFor(owner)
	return directions
}

func (n *OVGNode) setPortDirections(owner *layoutgraph.Node, directions portDirectionSet) {
	metadata, ok := n.portMetadataFor(owner)
	if !ok {
		return
	}
	n.ensurePortOwners()
	metadata.directions = directions
	n.portOwnersByNode[owner] = metadata
}

func (n *OVGNode) isCenterPortOf(owner *layoutgraph.Node) bool {
	metadata, ok := n.portMetadataFor(owner)
	return ok && metadata.isCenterPort
}

func (n *OVGNode) portOwners() map[*layoutgraph.Node]ovgPortMetadata {
	return n.portOwnersByNode
}

func (n *OVGNode) sharesPortOwner(other *OVGNode) bool {
	if n == nil || other == nil {
		return false
	}
	for owner := range n.portOwners() {
		if other.isPortOf(owner) {
			return true
		}
	}
	return false
}

func (node *OVGNode) addEdge(e *OVGEdge) {
	node.Edges = append(node.Edges, e)
}

func (n *OVGNode) adjacent(e *OVGEdge) *OVGNode {
	if n == e.From {
		return e.To
	}
	return e.From
}

func (n *OVGNode) Adjacent(edge *OVGEdge) *OVGNode { return n.adjacent(edge) }

// hasUnobstructedLineToPorts checks if an OVG node has unobstructed lines to non-container ports
// numNodes indicates how many ports belonging to different nodes it must have lines to
func (ovgNode *OVGNode) hasUnobstructedLineToPorts(ovg *OVG, portIndex *ovgPortIndex, numNodes int, guard *ovgBuildGuard) (bool, error) {
	if err := guard.check(); err != nil {
		return false, err
	}
	// Only care about ovg nodes which are port aligned
	distinctPortNodeCount := 0

	aligned := portIndex.alignedOwners(ovgNode.X, ovgNode.Y)
	hasAlignedPort := len(aligned.byX) > 0 || len(aligned.byY) > 0
	for {
		ownerIndex, ok, err := aligned.next(guard)
		if err != nil {
			return false, err
		}
		if !ok {
			break
		}
		gNode := portIndex.owners[ownerIndex]
		portNodes := ovg.Ports[gNode]
		for _, portNode := range portNodes {
			if err := guard.step(); err != nil {
				return false, err
			}
			if portNode.X != ovgNode.X && portNode.Y != ovgNode.Y {
				continue
			}
			// Port nodes which can make a straight line between it, its shape's edge, and the point, are NOT candidates for unobstructed path
			// e.g. this is bad
			// ┌──────┐
			// │      │
			// x      │
			// ├──────┘
			// │
			// │
			// x
			//
			// this is fine
			//        ┌──────┐
			//        │      │
			// x──────x      │
			//        └──────┘
			if portNode.X == ovgNode.X && ((portNode.X == gNode.TopLeft.X) || (portNode.X == gNode.TopLeft.X+gNode.Width)) {
				continue
			}
			if portNode.Y == ovgNode.Y && ((portNode.Y == gNode.TopLeft.Y) || (portNode.Y == gNode.TopLeft.Y+gNode.Height)) {
				continue
			}

			// Remove nodes which are obstructed from a port by another node
			hasDirectLineToThisPort := true
			blockers := portIndex.horizontalBlockers[portNode.Y]
			if portNode.X == ovgNode.X {
				blockers = portIndex.verticalBlockers[portNode.X]
			}
			for _, otherGNode := range blockers {
				if err := guard.step(); err != nil {
					return false, err
				}
				if gNode == otherGNode {
					continue
				}
				if gNode.IsInvisible {
					continue
				}
				// don't count nodes of the same sequence as blocking each other
				if gNode.Sequence != nil && otherGNode.Sequence != nil && gNode.Sequence == otherGNode.Sequence {
					continue
				}
				if otherGNode.PassesThrough(portNode.Point, ovgNode.Point) {
					hasDirectLineToThisPort = false
					break
				}
			}

			if hasDirectLineToThisPort {
				distinctPortNodeCount++
				if distinctPortNodeCount == numNodes {
					return true, nil
				}
				break
			}
		}
	}

	return !hasAlignedPort, guard.check()
}

func (nodeA *OVGNode) distanceToBoundary(nodeB *layoutgraph.Node) float64 {
	x1 := nodeA.Point.X
	y1 := nodeA.Point.Y

	x2 := nodeB.TopLeft.X
	y2 := nodeB.TopLeft.Y
	x2b := nodeB.TopLeft.X + nodeB.Width
	y2b := nodeB.TopLeft.Y + nodeB.Height

	left := x2b < x1
	right := x1 < x2
	top := y2b < y1
	bottom := y1 < y2

	if top && left {
		return geo.EuclideanDistance(x1, y1, x2b, y2b)
	} else if left && bottom {
		return geo.EuclideanDistance(x1, y1, x2b, y2)
	} else if bottom && right {
		return geo.EuclideanDistance(x1, y1, x2, y2)
	} else if right && top {
		return geo.EuclideanDistance(x1, y1, x2, y2b)
	} else if left {
		return x1 - x2b
	} else if right {
		return x2 - x1
	} else if bottom {
		return y2 - y1
	} else if top {
		return y1 - y2b
	} else {
		return 0
	}
}
