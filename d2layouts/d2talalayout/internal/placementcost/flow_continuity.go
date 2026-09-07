package placementcost

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// Experimental placement preferences. The completed-layout score is unchanged.
const flowSpineWeight = 1.0
const flowBranchWeight = 1.0

// flowContinuityCost treats paths through a node as a visual unit. An incoming
// and outgoing connection should have at least one recognizable continuation;
// branches should leave enough angular room to read their separate attachments.
// A fixed, small incident-degree limit bounds the pair work in every existing
// placement candidate. Group vessels, port-specific edges, and cross-container
// abstractions retain their existing placement treatment.
func flowContinuityCost(node *layoutgraph.Node, s *edgeScratch) float64 {
	if node.TopLeft == nil || len(node.Edges) < 2 || len(node.Edges) > 8 ||
		node.Cluster != nil || node.Sequence != nil || node.HerdAssignment != nil ||
		len(node.Graph.Containers[node]) != 0 {
		return 0
	}
	const (
		incomingDirection uint8 = 1 << iota
		outgoingDirection
	)
	type ray struct {
		node       *layoutgraph.Node
		x, y       float64
		directions uint8
	}
	var rays [8]ray
	n := 0
	cx, cy := node.TopLeft.X+node.Width/2, node.TopLeft.Y+node.Height/2
	for i, e := range node.Edges {
		if e.IsInvisible || e.From == e.To || e.HasTableColumn() ||
			e.HasSourceArrow() == e.HasTargetArrow() || s.nRepl[i] != node {
			continue
		}
		adj := s.aRepl[i]
		if adj == nil || adj.TopLeft == nil || adj.Container != node.Container {
			continue
		}
		incoming := e.To == node
		if e.HasSourceArrow() {
			incoming = !incoming
		}
		direction := outgoingDirection
		if incoming {
			direction = incomingDirection
		}
		duplicate := false
		for j := 0; j < n; j++ {
			if rays[j].node == adj {
				// Parallel edges share one geometric ray. Reciprocal edges
				// retain both roles regardless of their declaration order.
				rays[j].directions |= direction
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		x, y := adj.TopLeft.X+adj.Width/2-cx, adj.TopLeft.Y+adj.Height/2-cy
		length := math.Hypot(x, y)
		if length == 0 {
			continue
		}
		rays[n] = ray{adj, x / length, y / length, direction}
		n++
	}
	spine := math.Inf(1)
	branchSum, branches := 0.0, 0
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			dot := math.Max(-1, math.Min(1, rays[i].x*rays[j].x+rays[i].y*rays[j].y))
			a, b := rays[i].directions, rays[j].directions
			if a&incomingDirection != 0 && b&outgoingDirection != 0 ||
				a&outgoingDirection != 0 && b&incomingDirection != 0 {
				spine = math.Min(spine, 1+dot)
			}
			if a&b != 0 {
				// Directions less than 60 degrees apart compete for a small
				// visual wedge at the attachment; wider angles incur no cost.
				// A reciprocal pair can also be a continuation, but each pair
				// contributes to the branch average only once.
				branchSum += math.Max(0, 2*dot-1)
				branches++
			}
		}
	}
	cost := 0.0
	if !math.IsInf(spine, 1) {
		cost += flowSpineWeight * spine
	}
	if branches > 0 {
		cost += flowBranchWeight * branchSum / float64(branches)
	}
	return node.Graph.TurnCost() * cost
}
