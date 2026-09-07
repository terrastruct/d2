package routing

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

type TunnelEntry struct {
	Node    *layoutgraph.Node
	OVGNode *OVGNode
}

func NewTunnelEntry(node *layoutgraph.Node, ovgNode *OVGNode) *TunnelEntry {
	// Tunnel endpoints are unrestricted ports. Register their owner when the
	// entry is created so every production port uses the same per-owner metadata
	// representation before OVG canonicalization.
	ovgNode.addPortMetadata(node, ovgPortMetadata{})
	return &TunnelEntry{Node: node, OVGNode: ovgNode}
}

type Tunnel struct {
	EntryA *TunnelEntry
	EntryB *TunnelEntry
}

func NewTunnel(entryA, entryB *TunnelEntry) *Tunnel {
	return &Tunnel{EntryA: entryA, EntryB: entryB}
}

type Range struct {
	start float64
	end   float64
}

func (r *Range) length() float64 {
	return r.end - r.start
}

func tunnelRangesBetween(g *layoutgraph.Graph, nodeA, nodeB *layoutgraph.Node, filterOutShortTunnels bool, guard *ovgBuildGuard) (ranges []*Range, visibleHorizontally bool, err error) {
	if err := guard.check(); err != nil {
		return nil, false, err
	}
	return tunnelRangesBetweenChecked(g,
		nodeA,
		nodeB,
		filterOutShortTunnels,
		guard.step,
		guard.check,
		guard.isDescendantOf,
	)
}

func tunnelRangesBetweenGuarded(g *layoutgraph.Graph, nodeA, nodeB *layoutgraph.Node, filterOutShortTunnels bool, guard *routeWorkGuard) (ranges []*Range, visibleHorizontally bool, err error) {
	return tunnelRangesBetweenChecked(g,
		nodeA,
		nodeB,
		filterOutShortTunnels,
		guard.step,
		guard.check,
		func(descendant, ancestor *layoutgraph.Node) (bool, error) {
			return isDescendantOfWithRouteGuard(descendant, ancestor, guard)
		},
	)
}

func isDescendantOfWithRouteGuard(descendant, ancestor *layoutgraph.Node, guard *routeWorkGuard) (bool, error) {
	for current := descendant; ; {
		if err := guard.step(); err != nil {
			return false, err
		}
		if ancestor == current {
			return true, nil
		}
		if current == nil {
			return false, nil
		}
		switch {
		case current.Container != nil:
			current = current.Container
		case current.Cluster != nil:
			current = current.Cluster.Vessel
		case current.Sequence != nil:
			current = current.Sequence.Vessel
		default:
			return ancestor == nil, nil
		}
	}
}

func tunnelRangesBetweenChecked(g *layoutgraph.Graph,
	nodeA, nodeB *layoutgraph.Node,
	filterOutShortTunnels bool,
	step func() error,
	check func() error,
	isDescendant func(*layoutgraph.Node, *layoutgraph.Node) (bool, error),
) (ranges []*Range, visibleHorizontally bool, err error) {
	if err := step(); err != nil {
		return nil, false, err
	}
	visibleHorizontally = nodeA.VisibilityGraphCandidate(true, false, true, nodeB, 0)
	var visibleVertically bool
	if !visibleHorizontally {
		if err := step(); err != nil {
			return nil, false, err
		}
		visibleVertically = nodeA.VisibilityGraphCandidate(false, false, true, nodeB, 0)
		if !visibleVertically {
			return nil, false, nil
		}
	}

	{
		var start, end float64
		if visibleHorizontally {
			start = math.Max(nodeA.TopLeft.Y, nodeB.TopLeft.Y)
			end = math.Min(nodeA.TopLeft.Y+nodeA.Height, nodeB.TopLeft.Y+nodeB.Height)
		} else {
			nodeARight := nodeA.TopLeft.X + nodeA.Width
			nodeBRight := nodeB.TopLeft.X + nodeB.Width

			// sequence nodes have shorter ranges along Top/Bottom (except last node in sequence)
			if nodeA.Sequence != nil && nodeA.Sequence.Last() != nodeA {
				nodeARight -= shape.STEP_WEDGE_WIDTH
			}
			if nodeB.Sequence != nil && nodeB.Sequence.Last() != nodeB {
				nodeBRight -= shape.STEP_WEDGE_WIDTH
			}

			start = math.Max(nodeA.TopLeft.X, nodeB.TopLeft.X)
			end = math.Min(nodeARight, nodeBRight)
		}
		if err := step(); err != nil {
			return nil, false, err
		}
		ranges = []*Range{{start: start, end: end}}
	}

	for _, otherN := range g.Nodes {
		if err := step(); err != nil {
			return nil, false, err
		}
		if otherN == nodeA || otherN == nodeB {
			continue
		}
		otherBelowA, err := isDescendant(otherN, nodeA)
		if err != nil {
			return nil, false, err
		}
		aBelowOther, err := isDescendant(nodeA, otherN)
		if err != nil {
			return nil, false, err
		}
		otherBelowB, err := isDescendant(otherN, nodeB)
		if err != nil {
			return nil, false, err
		}
		bBelowOther, err := isDescendant(nodeB, otherN)
		if err != nil {
			return nil, false, err
		}
		if otherBelowA || aBelowOther || otherBelowB || bBelowOther {
			continue
		}

		// We're reusing a function that assumes order, so the different order matters
		if otherN.IsBlocked(nodeA, nodeB, true, visibleHorizontally) || otherN.IsBlocked(nodeB, nodeA, true, visibleHorizontally) {
			return nil, false, nil
		}
		// Skip ones not in between
		if visibleHorizontally {
			if !((nodeA.TopLeft.X < otherN.TopLeft.X && otherN.TopLeft.X < nodeB.TopLeft.X) ||
				(nodeB.TopLeft.X < otherN.TopLeft.X && otherN.TopLeft.X < nodeA.TopLeft.X)) {
				continue
			}
		} else {
			if !((nodeA.TopLeft.Y < otherN.TopLeft.Y && otherN.TopLeft.Y < nodeB.TopLeft.Y) ||
				(nodeB.TopLeft.Y < otherN.TopLeft.Y && otherN.TopLeft.Y < nodeA.TopLeft.Y)) {
				continue
			}
		}

		newRanges := make([]*Range, 0)
		// For every range, a partial obstruction can do one of the following
		// (a) delete it entirely (it obscures the whole range)
		// (b) split it into 2 (it obscures a middle subrange of a range)
		// (c) shorten it (it obscures a non-middle subrange)
		// (d) nothing (it doesn't overlap with any ranges)
		for _, r := range ranges {
			if err := step(); err != nil {
				return nil, false, err
			}
			if visibleHorizontally {
				// (a)
				if otherN.TopLeft.Y <= r.start && (otherN.TopLeft.Y+otherN.Height) >= r.end {
					continue
				}
				// (b)
				if otherN.TopLeft.Y > r.start && (otherN.TopLeft.Y+otherN.Height) < r.end {
					newRanges = append(newRanges,
						[]*Range{
							{start: r.start, end: otherN.TopLeft.Y},
							{start: otherN.TopLeft.Y + otherN.Height, end: r.end},
						}...)
					continue
				}
				// (c) obscures start
				if otherN.TopLeft.Y <= r.start && (otherN.TopLeft.Y+otherN.Height) < r.end && (otherN.TopLeft.Y+otherN.Height) > r.start {
					newRanges = append(newRanges, &Range{start: otherN.TopLeft.Y + otherN.Height, end: r.end})
					continue
				}
				// (c) obscures end
				if otherN.TopLeft.Y > r.start && (otherN.TopLeft.Y+otherN.Height) >= r.end && otherN.TopLeft.Y < r.end {
					newRanges = append(newRanges, &Range{start: r.start, end: otherN.TopLeft.Y})
					continue
				}

				// (d)
				newRanges = append(newRanges, r)
			} else {
				if otherN.TopLeft.X <= r.start && (otherN.TopLeft.X+otherN.Width) >= r.end {
					continue
				}
				if otherN.TopLeft.X > r.start && (otherN.TopLeft.X+otherN.Width) < r.end {
					newRanges = append(newRanges,
						[]*Range{
							{start: r.start, end: otherN.TopLeft.X},
							{start: otherN.TopLeft.X + otherN.Width, end: r.end},
						}...)
					continue
				}
				if otherN.TopLeft.X <= r.start && (otherN.TopLeft.X+otherN.Width) < r.end && (otherN.TopLeft.X+otherN.Width) > r.start {
					newRanges = append(newRanges, &Range{start: otherN.TopLeft.X + otherN.Width, end: r.end})
					continue
				}
				if otherN.TopLeft.X > r.start && (otherN.TopLeft.X+otherN.Width) >= r.end && otherN.TopLeft.X < r.end {
					newRanges = append(newRanges, &Range{start: r.start, end: otherN.TopLeft.X})
					continue
				}
				newRanges = append(newRanges, r)
			}
		}
		ranges = make([]*Range, 0)
		if err := check(); err != nil {
			return nil, false, err
		}
		// Filter out the ranges that are too short for even one tunnel
		for _, s := range newRanges {
			if err := step(); err != nil {
				return nil, false, err
			}
			if !filterOutShortTunnels || s.length() >= segmentSpacingBuffer {
				ranges = append(ranges, s)
			}
		}
	}

	return ranges, visibleHorizontally, nil
}

func buildTunnelsBetween(g *layoutgraph.Graph, nodeA, nodeB *layoutgraph.Node, guard *ovgBuildGuard) ([]*Tunnel, error) {
	if err := guard.check(); err != nil {
		return nil, err
	}
	aBelowB, err := guard.isDescendantOf(nodeA, nodeB)
	if err != nil {
		return nil, err
	}
	bBelowA, err := guard.isDescendantOf(nodeB, nodeA)
	if err != nil {
		return nil, err
	}
	if aBelowB || bBelowA {
		return nil, nil
	}
	ranges, visibleHorizontally, err := tunnelRangesBetween(g, nodeA, nodeB, true, guard)
	if err != nil {
		return nil, err
	}
	if ranges == nil {
		return nil, nil
	}

	numEdgesToFit := 0.0
	for _, otherE := range nodeA.Edges {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if nodeA.Adjacent(otherE) == nodeB {
			numEdgesToFit++
		}
	}

	tunnels := make([]*Tunnel, 0)
	for _, r := range ranges {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if visibleHorizontally {
			numTunnelsFit := math.Min(math.Floor((r.end-r.start)/segmentSpacingBuffer), numEdgesToFit)

			for i := 1.0; i <= numTunnelsFit; i++ {
				if err := guard.step(); err != nil {
					return nil, err
				}
				val := math.Round(r.start + i*(r.end-r.start)/(numTunnelsFit+1))
				nodeAEntry, err := guard.newDerivedNode(geo.NewPoint(0, val))
				if err != nil {
					return nil, err
				}
				nodeBEntry, err := guard.newDerivedNode(geo.NewPoint(0, val))
				if err != nil {
					return nil, err
				}
				if err := guard.step(); err != nil {
					return nil, err
				}
				tunnel := NewTunnel(NewTunnelEntry(nodeA, nodeAEntry), NewTunnelEntry(nodeB, nodeBEntry))
				if nodeA.TopLeft.X > nodeB.TopLeft.X {
					tunnel.EntryA.OVGNode.Point.X = nodeA.TopLeft.X
					tunnel.EntryB.OVGNode.Point.X = nodeB.TopLeft.X + nodeB.Width
				} else {
					tunnel.EntryA.OVGNode.Point.X = nodeA.TopLeft.X + nodeA.Width
					tunnel.EntryB.OVGNode.Point.X = nodeB.TopLeft.X
				}
				tunnels = append(tunnels, tunnel)
				numEdgesToFit--
			}
		} else {
			numTunnelsFit := math.Min(math.Floor((r.end-r.start)/segmentSpacingBuffer), numEdgesToFit)

			for i := 1.0; i <= numTunnelsFit; i++ {
				if err := guard.step(); err != nil {
					return nil, err
				}
				val := math.Round(r.start + i*(r.end-r.start)/(numTunnelsFit+1))
				nodeAEntry, err := guard.newDerivedNode(geo.NewPoint(val, 0))
				if err != nil {
					return nil, err
				}
				nodeBEntry, err := guard.newDerivedNode(geo.NewPoint(val, 0))
				if err != nil {
					return nil, err
				}
				if err := guard.step(); err != nil {
					return nil, err
				}
				tunnel := NewTunnel(NewTunnelEntry(nodeA, nodeAEntry), NewTunnelEntry(nodeB, nodeBEntry))
				if nodeA.TopLeft.Y > nodeB.TopLeft.Y {
					tunnel.EntryA.OVGNode.Point.Y = nodeA.TopLeft.Y
					tunnel.EntryB.OVGNode.Point.Y = nodeB.TopLeft.Y + nodeB.Height
				} else {
					tunnel.EntryA.OVGNode.Point.Y = nodeA.TopLeft.Y + nodeA.Height
					tunnel.EntryB.OVGNode.Point.Y = nodeB.TopLeft.Y
				}
				tunnels = append(tunnels, tunnel)
				numEdgesToFit--
			}
		}
	}
	return tunnels, nil
}

func buildTunnels(g *layoutgraph.Graph, guard *ovgBuildGuard) ([]*Tunnel, error) {
	if err := guard.check(); err != nil {
		return nil, err
	}
	out := make([]*Tunnel, 0)
	specialNodes := map[*layoutgraph.Node]struct{}{}
	for _, c := range g.Clusters {
		if err := guard.step(); err != nil {
			return nil, err
		}
		for _, cn := range c.Nodes {
			if err := guard.step(); err != nil {
				return nil, err
			}
			specialNodes[cn] = struct{}{}
		}
	}
	for n := range g.NodeToTree {
		if err := guard.step(); err != nil {
			return nil, err
		}
		specialNodes[n] = struct{}{}
	}

	searched := make(map[*layoutgraph.Node]map[*layoutgraph.Node]bool)
	for _, n := range g.Nodes {
		if err := guard.step(); err != nil {
			return nil, err
		}
		if _, ok := specialNodes[n]; ok {
			continue
		}
		if n.IsTable() {
			// no tunnels for tables
			continue
		}
		if _, ok := searched[n]; !ok {
			searched[n] = make(map[*layoutgraph.Node]bool)
		}
		for _, e := range n.Edges {
			if err := guard.step(); err != nil {
				return nil, err
			}
			adj := n.Adjacent(e)
			if adj == n {
				continue
			}
			if _, ok := specialNodes[adj]; ok {
				continue
			}
			if _, ok := searched[adj]; !ok {
				searched[adj] = make(map[*layoutgraph.Node]bool)
			}
			if _, ok := searched[n][adj]; ok {
				continue
			}
			searched[n][adj] = true
			searched[adj][n] = true

			tunnels, err := buildTunnelsBetween(g, n, adj, guard)
			if err != nil {
				return nil, err
			}
			out = append(out, tunnels...)
		}
	}

	return out, nil
}
