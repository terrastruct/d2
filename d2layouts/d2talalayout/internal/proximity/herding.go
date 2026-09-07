package proximity

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// AssignHerds chooses a shared container side for siblings whose external
// cousins should remain mutually accessible during placement.
func AssignHerds(ctx context.Context, graph *layoutgraph.Graph, root *layoutgraph.Node, abductions []*layoutgraph.EdgeAbduction) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("AssignHerds: %w", err)
	}
	grouped, cousins, err := GroupSheep(ctx, graph, root, abductions)
	if err != nil {
		return err
	}
	var uncleOrder []*layoutgraph.Node
	for uncle, nodes := range grouped {
		if len(nodes) <= 1 {
			delete(grouped, uncle)
			delete(cousins, uncle)
		} else {
			uncleOrder = append(uncleOrder, uncle)
		}
	}
	slices.SortFunc(uncleOrder, func(a, b *layoutgraph.Node) int {
		return cmp.Compare(a.ID, b.ID)
	})

	components, err := connectedHerds(ctx, uncleOrder, grouped)
	if err != nil {
		return err
	}
	unbiasedSide := 0
	for _, component := range components {
		// Overlapping groups describe a single equality constraint: every member
		// must use the same side. Choose that side only after intersecting all
		// placed cousins' available sides, so a later uncle cannot change one
		// member and invalidate an earlier group.
		sides := []geo.Orientation{geo.Top, geo.Right, geo.Bottom, geo.Left}
		preferred := geo.NONE
		for _, uncle := range component.uncles {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("AssignHerds: %w", err)
			}
			children := graph.Containers[uncle]
			if len(children) == 0 {
				return invariant.Errorf("herding uncle %s has no children", uncle.DebugID())
			}
			if children[0].TopLeft == nil {
				continue
			}
			for _, node := range grouped[uncle] {
				for _, cousin := range cousins[uncle][node] {
					assignment := cousin.HerdAssignment
					if assignment == nil {
						continue
					}
					orientation := assignment.Orientation
					if !orientation.IsHorizontal() && !orientation.IsVertical() {
						return invariant.Errorf("cousin %s has an invalid herd orientation", cousin.DebugID())
					}
					bothSides := CanUseBothSides(uncle, orientation)
					sides = slices.DeleteFunc(sides, func(side geo.Orientation) bool {
						return side != orientation.GetOpposite() && !(bothSides && side == orientation)
					})
					if preferred == geo.NONE {
						preferred = orientation.GetOpposite()
						if bothSides && assignment.SameSidePairCount() < assignment.OppositeSidePairCount() {
							preferred = orientation
						}
					}
				}
			}
		}
		if len(sides) == 0 {
			// Herding is a placement preference. Cousins on incompatible sides
			// can make that preference impossible for a valid graph; in that
			// case leave the entire connected herd free to place normally.
			for _, node := range component.nodes {
				node.HerdAssignment = nil
			}
			continue
		}
		if preferred == geo.NONE {
			preferred = sides[unbiasedSide%len(sides)]
			unbiasedSide++
		} else if !slices.Contains(sides, preferred) {
			preferred = sides[0]
		}
		for _, node := range component.nodes {
			node.HerdAssignment = layoutgraph.NewHerdAssignment()
			node.HerdAssignment.Orientation = preferred
		}
		// Pair counts influence later herds, so record only the final choice.
		for _, uncle := range component.uncles {
			if graph.Containers[uncle][0].TopLeft == nil {
				continue
			}
			for _, node := range grouped[uncle] {
				for _, cousin := range cousins[uncle][node] {
					if cousin.HerdAssignment == nil {
						continue
					}
					if preferred == cousin.HerdAssignment.Orientation {
						node.HerdAssignment.PairSameSide(uncle)
						cousin.HerdAssignment.PairSameSide(uncle)
					} else {
						node.HerdAssignment.PairOppositeSide(uncle)
						cousin.HerdAssignment.PairOppositeSide(uncle)
					}
				}
			}
		}
	}
	if err := ApplyVirally(ctx, uncleOrder, grouped); err != nil {
		return err
	}

	for _, uncle := range uncleOrder {
		for _, node := range grouped[uncle] {
			if node.HerdAssignment != nil && node.IsClusterVessel() {
				cluster := graph.Clusters[node]
				if (node.HerdAssignment.Orientation == geo.Top || node.HerdAssignment.Orientation == geo.Bottom) && cluster.Arrangement == layoutgraph.Column {
					cluster.Arrangement = layoutgraph.Row
				} else if (node.HerdAssignment.Orientation == geo.Left || node.HerdAssignment.Orientation == geo.Right) && cluster.Arrangement == layoutgraph.Row {
					cluster.Arrangement = layoutgraph.Column
				}
			}
		}
	}
	return nil
}

type herdComponent struct {
	nodes  []*layoutgraph.Node
	uncles []*layoutgraph.Node
}

// connectedHerds joins groups that share a node, retaining deterministic order.
func connectedHerds(ctx context.Context, herdOrder []*layoutgraph.Node, herds map[*layoutgraph.Node][]*layoutgraph.Node) ([]herdComponent, error) {
	byNode := make(map[*layoutgraph.Node][]*layoutgraph.Node)
	for _, uncle := range herdOrder {
		for _, node := range herds[uncle] {
			byNode[node] = append(byNode[node], uncle)
		}
	}
	seenUncles := make(map[*layoutgraph.Node]bool)
	seenNodes := make(map[*layoutgraph.Node]bool)
	var components []herdComponent
	for _, uncle := range herdOrder {
		if seenUncles[uncle] {
			continue
		}
		component := herdComponent{uncles: []*layoutgraph.Node{uncle}}
		seenUncles[uncle] = true
		for i := 0; i < len(component.uncles); i++ {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("AssignHerds: %w", err)
			}
			for _, node := range herds[component.uncles[i]] {
				if seenNodes[node] {
					continue
				}
				seenNodes[node] = true
				component.nodes = append(component.nodes, node)
				for _, related := range byNode[node] {
					if !seenUncles[related] {
						seenUncles[related] = true
						component.uncles = append(component.uncles, related)
					}
				}
			}
		}
		components = append(components, component)
	}
	return components, nil
}

// ApplyVirally propagates each known herd orientation through its ordered groups.
func ApplyVirally(ctx context.Context, herdOrder []*layoutgraph.Node, herds map[*layoutgraph.Node][]*layoutgraph.Node) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("AssignHerds: %w", err)
		}
		end := true
		for _, uncle := range herdOrder {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("AssignHerds: %w", err)
			}
			nodes := herds[uncle]
			var assignment *layoutgraph.HerdAssignment
			for _, node := range nodes {
				if node.HerdAssignment != nil && node.HerdAssignment.Orientation != geo.NONE {
					assignment = node.HerdAssignment
					break
				}
			}
			if assignment != nil {
				for _, node := range nodes {
					if node.HerdAssignment == nil {
						end = false
						node.HerdAssignment = assignment.Copy()
					} else if node.HerdAssignment.Orientation != geo.NONE && node.HerdAssignment.Orientation != assignment.Orientation {
						return invariant.Errorf(
							"node %s has herd orientation %s; expected %s",
							node.DebugID(), node.HerdAssignment.Orientation.ToString(), assignment.Orientation.ToString(),
						)
					}
				}
			}
		}
		if end {
			return nil
		}
	}
}

// CanUseBothSides reports whether node is long enough perpendicular to
// orientation to host herd members on both parallel sides.
func CanUseBothSides(node *layoutgraph.Node, orientation geo.Orientation) bool {
	isWide := node.Width >= 2*node.Height
	isTall := node.Height >= 2*node.Width
	return (orientation == geo.Top || orientation == geo.Bottom) && isWide ||
		(orientation == geo.Left || orientation == geo.Right) && isTall
}

// GroupSheep groups root's children by their external uncle and records the
// cousin connections that define each group.
func GroupSheep(
	ctx context.Context,
	graph *layoutgraph.Graph,
	root *layoutgraph.Node,
	abductions []*layoutgraph.EdgeAbduction,
) (map[*layoutgraph.Node][]*layoutgraph.Node, map[*layoutgraph.Node]map[*layoutgraph.Node][]*layoutgraph.Node, error) {
	byUncle := make(map[*layoutgraph.Node][]*layoutgraph.Node)
	toCousin := make(map[*layoutgraph.Node]map[*layoutgraph.Node][]*layoutgraph.Node)
	used := make([]bool, len(abductions))
	for _, node := range graph.Containers[root] {
		if err := ctx.Err(); err != nil {
			return nil, nil, fmt.Errorf("AssignHerds: %w", err)
		}
		for i, abduction := range abductions {
			if err := ctx.Err(); err != nil {
				return nil, nil, fmt.Errorf("AssignHerds: %w", err)
			}
			if used[i] {
				continue
			}
			if abduction == nil {
				return nil, nil, invariant.New("herding has a nil edge abduction")
			}
			from := groupVessel(abduction.OriginallyFrom)
			to := groupVessel(abduction.OriginallyTo)
			var cousin, current *layoutgraph.Node
			if abduction.OriginallyTo != nil && (from == node || descendantOf(from, node)) {
				if abduction.CurrentTo != nil && !abduction.CurrentTo.IsContainer() {
					continue
				}
				if to == nil || to.OwningContainer() == nil {
					continue
				}
				cousin, current = to, abduction.CurrentTo
			} else if abduction.OriginallyFrom != nil && (to == node || descendantOf(to, node)) {
				if abduction.CurrentFrom != nil && !abduction.CurrentFrom.IsContainer() {
					continue
				}
				if from == nil || from.OwningContainer() == nil {
					continue
				}
				cousin, current = from, abduction.CurrentFrom
			}
			if cousin == nil {
				continue
			}
			used[i] = true
			for cousin.OwningContainer() != current {
				if cousin.Cluster != nil {
					cousin = cousin.Cluster.Vessel
				} else if cousin.Sequence != nil {
					cousin = cousin.Sequence.Vessel
				} else {
					cousin = cousin.OwningContainer()
				}
			}
			uncle := cousin.OwningContainer()
			if uncle == nil || !uncle.IsContainer() {
				continue
			}
			if toCousin[uncle] == nil {
				toCousin[uncle] = make(map[*layoutgraph.Node][]*layoutgraph.Node)
			}
			if _, ok := toCousin[uncle][node]; !ok {
				byUncle[uncle] = append(byUncle[uncle], node)
			}
			toCousin[uncle][node] = append(toCousin[uncle][node], cousin)
		}
	}
	return byUncle, toCousin, nil
}

func descendantOf(node, ancestor *layoutgraph.Node) bool {
	for node != nil {
		if node == ancestor {
			return true
		}
		switch {
		case node.Container != nil:
			node = node.Container
		case node.Cluster != nil:
			node = node.Cluster.Vessel
		case node.Sequence != nil:
			node = node.Sequence.Vessel
		default:
			node = nil
		}
	}
	return ancestor == nil
}

// SyncHerdFences updates the coordinate constraint for every assigned herd to
// the current graph boundary.
func SyncHerdFences(graph *layoutgraph.Graph) {
	topLeft, bottomRight := graph.BoundingBox()
	for _, node := range graph.Nodes {
		if node.HerdAssignment == nil || node.FixedTopLeft != nil {
			continue
		}
		switch node.HerdAssignment.Orientation {
		case geo.Top:
			node.HerdAssignment.Val = topLeft.Y
		case geo.Bottom:
			node.HerdAssignment.Val = bottomRight.Y
		case geo.Left:
			node.HerdAssignment.Val = topLeft.X
		case geo.Right:
			node.HerdAssignment.Val = bottomRight.X
		}
	}
}
