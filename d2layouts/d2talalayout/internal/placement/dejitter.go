package placement

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placementcost"
	"github.com/d2lang/d2/lib/geo"
)

// dejitter performs micro-movements on nodes to remove the jittery connections that can be straightened out with slight adjustments in one direction
// E.g. jittery line:
// .             +--------->
// . +-----------+
func Dejitter(ctx context.Context, g *layoutgraph.Graph) (bool, error) {
	ctx, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "DejitterTransactions")
	if err != nil {
		return false, err
	}
	dejittered := false
	txn, err := g.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{AffectContainers: true})
	if err != nil {
		return false, err
	}
	if len(g.Nodes) == 0 {
		return false, nil
	}
	var rollback *layoutgraph.Transaction
	complete := false
	defer func() {
		if rollback != nil && !complete {
			rollback.Rollback()
		}
	}()
	for _, node := range g.Nodes {
		if node.Cluster != nil {
			continue
		}
		// Too much trouble to have to jitter all children too
		if node.IsContainer() {
			continue
		}
		if _, has := g.NodeToTree[node]; has {
			continue
		}
		if node.Sequence != nil {
			continue
		}
		if node.FixedTopLeft != nil {
			continue
		}

		prevSymmetry, err := placementcost.NodeSymmetry(ctx, node, nil)
		if err != nil {
			return false, err
		}
		for _, edge := range node.Edges {
			// No jitter possible with less than 3 segments
			if edge.NumSegments() < 3 {
				continue
			}
			if node.Adjacent(edge).Cluster != nil {
				continue
			}

			var pointBeforeBends *geo.Point
			var pointOnNode *geo.Point
			lastTwoBends := make([]*geo.Point, 0)
			// Skip the first of each since it's the port
			if edge.From == node {
				lastTwoBends = append(lastTwoBends, edge.Points[1], edge.Points[2])
				pointBeforeBends = edge.Points[3]
				pointOnNode = edge.Points[0]
			} else if edge.To == node {
				lastTwoBends = append(lastTwoBends, edge.Points[len(edge.Points)-2], edge.Points[len(edge.Points)-3])
				pointBeforeBends = edge.Points[len(edge.Points)-4]
				pointOnNode = edge.Points[len(edge.Points)-1]
			}

			// Only a jitter if the last two bends fall within threshold
			if geo.EuclideanDistance(lastTwoBends[0].X, lastTwoBends[0].Y, lastTwoBends[1].X, lastTwoBends[1].Y) > jitterThreshold {
				continue
			}

			// A vertical line moves horizontally to the second bend's X to straighten it.
			isVerticalLine := lastTwoBends[0].Y == lastTwoBends[1].Y

			// TODO: support correcting U-turns.
			// Leave them unchanged here because they are not straightforward to collapse safely.
			// A U-turn has both the point before the bends and the point on the node on one side of the bends.
			if isVerticalLine {
				if (pointBeforeBends.Y > lastTwoBends[0].Y) && (pointOnNode.Y > lastTwoBends[0].Y) {
					continue
				}
				if (pointBeforeBends.Y < lastTwoBends[0].Y) && (pointOnNode.Y < lastTwoBends[0].Y) {
					continue
				}
			} else {
				if (pointBeforeBends.X > lastTwoBends[0].X) && (pointOnNode.X > lastTwoBends[0].X) {
					continue
				}
				if (pointBeforeBends.X < lastTwoBends[0].X) && (pointOnNode.X < lastTwoBends[0].X) {
					continue
				}
			}

			if isVerticalLine {
				if lastTwoBends[1].X == lastTwoBends[0].X {
					continue
				}
			} else {
				if lastTwoBends[1].Y == lastTwoBends[0].Y {
					continue
				}
			}

			// ---- from here on we know it is a jitter line

			// Look for other connected edges which are straight lines
			hasAnotherConnectedStraightEdge := false
			for _, otherEdge := range node.Edges {
				if otherEdge == edge {
					continue
				}
				if otherEdge.NumSegments() > 1 {
					continue
				}
				// If the jittering one is vertical, we only look for other vertical ones
				if otherEdge.From == node {
					if isVerticalLine && (otherEdge.Points[0].X == otherEdge.Points[1].X) {
						hasAnotherConnectedStraightEdge = true
						break
					}
					if !isVerticalLine && (otherEdge.Points[0].Y == otherEdge.Points[1].Y) {
						hasAnotherConnectedStraightEdge = true
						break
					}
				} else {
					if isVerticalLine && (otherEdge.Points[len(otherEdge.Points)-1].X == otherEdge.Points[len(otherEdge.Points)-2].X) {
						hasAnotherConnectedStraightEdge = true
						break
					}
					if !isVerticalLine && (otherEdge.Points[len(otherEdge.Points)-1].Y == otherEdge.Points[len(otherEdge.Points)-2].Y) {
						hasAnotherConnectedStraightEdge = true
						break
					}
				}
			}
			// Don't want to move other nodes
			if hasAnotherConnectedStraightEdge {
				continue
			}

			// ---- from here on we know there is no adjacent node connected via a tangent straight line

			var delta float64
			if isVerticalLine {
				delta = math.Round(lastTwoBends[1].X - lastTwoBends[0].X)
			} else {
				delta = math.Round(lastTwoBends[1].Y - lastTwoBends[0].Y)
			}

			// Sometimes it'll get real close to a sign flip but not yet. We want to prevent these super short edges, so we'll extend delta a little to consider those a sign flip
			var signFlipDelta float64
			if delta < 0 {
				signFlipDelta = delta - signFlipPadding
			} else {
				signFlipDelta = delta + signFlipPadding
			}

			// A sign flip is when there's movement on the endpoint but not the bend, and the new endpoint ends up
			// on the other axis of the bend (potentially going into the node)
			// For edges that are opposite directions (one vert one horiz), the concern is that the jitter causes it to go past the nearest bend
			// For edges that are same directions, the concern is that the jitter causes it to go past the second nearest bend
			signFlip := false
			// Map of the original edge to its closest two segments
			newSegments := make(map[*layoutgraph.Edge][]*geo.Point)

			// Note that the edge itself is included in this search when it's opposite direction, as it needs to be checked for new intersections as well
			for _, otherEdge := range node.Edges {
				var isOtherEdgeVertical bool
				if otherEdge.From == node {
					isOtherEdgeVertical = otherEdge.Points[0].X == otherEdge.Points[1].X
				} else {
					isOtherEdgeVertical = otherEdge.Points[len(otherEdge.Points)-1].X == otherEdge.Points[len(otherEdge.Points)-2].X
				}

				// This can happen if they are really close to each other
				// Diagonal points should be free to move around for the purpose of dejitter
				isOtherEdgeDiagonal := (len(otherEdge.Points) == 2) && (otherEdge.Points[0].X != otherEdge.Points[1].X) && (otherEdge.Points[0].Y != otherEdge.Points[1].Y)
				// The distance will be so small we don't have to worry about new intersections, and sign flips can't happen
				if isOtherEdgeDiagonal {
					continue
				}

				// Moving horizontally and edge vertical (move both x distance)
				if isVerticalLine && isOtherEdgeVertical {
					var second int
					var third int
					if otherEdge.From == node {
						second = 1
						third = 2
						newSegments[otherEdge] = []*geo.Point{
							geo.NewPoint(otherEdge.Points[0].X+delta, otherEdge.Points[0].Y),
							geo.NewPoint(otherEdge.Points[1].X+delta, otherEdge.Points[1].Y),
						}
					} else {
						second = len(otherEdge.Points) - 2
						third = len(otherEdge.Points) - 3
						newSegments[otherEdge] = []*geo.Point{
							geo.NewPoint(otherEdge.Points[len(otherEdge.Points)-1].X+delta, otherEdge.Points[len(otherEdge.Points)-1].Y),
							geo.NewPoint(otherEdge.Points[len(otherEdge.Points)-2].X+delta, otherEdge.Points[len(otherEdge.Points)-2].Y),
						}
					}
					if otherEdge != edge {
						if ((otherEdge.Points[second].X+signFlipDelta < otherEdge.Points[third].X) && (otherEdge.Points[second].X > otherEdge.Points[third].X)) ||
							((otherEdge.Points[second].X+signFlipDelta > otherEdge.Points[third].X) && (otherEdge.Points[second].X < otherEdge.Points[third].X)) {
							signFlip = true
						}
					}
				}
				// Moving horizontally and edge horizontal (move closest x distance)
				if isVerticalLine && !isOtherEdgeVertical {
					var first int
					var second int
					if otherEdge.From == node {
						first = 0
						second = 1
						newSegments[otherEdge] = []*geo.Point{
							geo.NewPoint(otherEdge.Points[0].X+delta, otherEdge.Points[0].Y),
							otherEdge.Points[1],
						}
					} else {
						first = len(otherEdge.Points) - 1
						second = len(otherEdge.Points) - 2
						newSegments[otherEdge] = []*geo.Point{
							geo.NewPoint(otherEdge.Points[len(otherEdge.Points)-1].X+delta, otherEdge.Points[len(otherEdge.Points)-1].Y),
							otherEdge.Points[len(otherEdge.Points)-2],
						}
					}
					if ((otherEdge.Points[first].X+signFlipDelta < otherEdge.Points[second].X) && (otherEdge.Points[first].X > otherEdge.Points[second].X)) ||
						((otherEdge.Points[first].X+signFlipDelta > otherEdge.Points[second].X) && (otherEdge.Points[first].X < otherEdge.Points[second].X)) {
						signFlip = true
					}
				}
				// Moving vertical and edge horizontal (move both y distance)
				if !isVerticalLine && !isOtherEdgeVertical {
					var second int
					var third int
					if otherEdge.From == node {
						second = 1
						third = 2
						newSegments[otherEdge] = []*geo.Point{
							geo.NewPoint(otherEdge.Points[0].X, otherEdge.Points[0].Y+delta),
							geo.NewPoint(otherEdge.Points[1].X, otherEdge.Points[1].Y+delta),
						}
					} else {
						second = len(otherEdge.Points) - 2
						third = len(otherEdge.Points) - 3
						newSegments[otherEdge] = []*geo.Point{
							geo.NewPoint(otherEdge.Points[len(otherEdge.Points)-1].X, otherEdge.Points[len(otherEdge.Points)-1].Y+delta),
							geo.NewPoint(otherEdge.Points[len(otherEdge.Points)-2].X, otherEdge.Points[len(otherEdge.Points)-2].Y+delta),
						}
					}
					if otherEdge != edge {
						if ((otherEdge.Points[second].Y+signFlipDelta < otherEdge.Points[third].Y) && (otherEdge.Points[second].Y > otherEdge.Points[third].Y)) ||
							((otherEdge.Points[second].Y+signFlipDelta > otherEdge.Points[third].Y) && (otherEdge.Points[second].Y < otherEdge.Points[third].Y)) {
							signFlip = true
						}
					}
				}
				// Moving vertical and edge vertical (move closest y distance)
				if !isVerticalLine && isOtherEdgeVertical {
					var first int
					var second int
					if otherEdge.From == node {
						first = 0
						second = 1
						newSegments[otherEdge] = []*geo.Point{
							geo.NewPoint(otherEdge.Points[0].X, otherEdge.Points[0].Y+delta),
							otherEdge.Points[1],
						}
					} else {
						first = len(otherEdge.Points) - 1
						second = len(otherEdge.Points) - 2
						newSegments[otherEdge] = []*geo.Point{
							geo.NewPoint(otherEdge.Points[len(otherEdge.Points)-1].X, otherEdge.Points[len(otherEdge.Points)-1].Y+delta),
							otherEdge.Points[len(otherEdge.Points)-2],
						}
					}
					if ((otherEdge.Points[first].Y+signFlipDelta < otherEdge.Points[second].Y) && (otherEdge.Points[first].Y > otherEdge.Points[second].Y)) ||
						((otherEdge.Points[first].Y+signFlipDelta > otherEdge.Points[second].Y) && (otherEdge.Points[first].Y < otherEdge.Points[second].Y)) {
						signFlip = true
					}
				}

				if signFlip {
					break
				}
			}

			if signFlip {
				continue
			}

			intersects := false
			for segmentEdge, newSegment := range newSegments {
				// Check for intersections on other nodes
				for _, otherNode := range g.Nodes {
					if otherNode == node {
						continue
					}
					// Can intersect with a container if one endpoint is inside it
					if segmentEdge.From.IsDescendantOf(otherNode) {
						continue
					}
					if segmentEdge.To.IsDescendantOf(otherNode) {
						continue
					}
					// It can touch the node it's connected to
					if ((otherNode != segmentEdge.To) && (otherNode != segmentEdge.From)) && otherNode.PassesThrough(newSegment[0], newSegment[1]) {
						intersects = true
						break
					}
				}
				if intersects {
					break
				}
			}
			if intersects {
				continue
			}

			// ---- from here on we know there is no object intersection introduced by updating all the edges to avoid the jitter, so we can safely shift to dejitter

			numEdgesPassedThrough := 0
			for _, otherEdge := range g.Edges {
				if (otherEdge.From == node) || (otherEdge.To == node) {
					continue
				}

				for i := 0; i < len(otherEdge.Points)-1; i++ {
					if node.PassesThrough(otherEdge.Points[i], otherEdge.Points[i+1]) {
						numEdgesPassedThrough++
					}
				}
			}

			if rollback == nil {
				state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
					CaptureEdgeRoutes: true,
				})
				if err := state.UpdateWithWorkGuard(g, guard); err != nil {
					return false, err
				}
				rollback = &layoutgraph.Transaction{Graph: g, PriorGraphState: state}
			}

			txn.AddOp(func() error {
				// Tentatively commit the node
				if isVerticalLine {
					node.TopLeft.X += delta
				} else {
					node.TopLeft.Y += delta
				}

				newNumEdgesPassedThrough := 0
				for _, otherEdge := range g.Edges {
					if (otherEdge.From == node) || (otherEdge.To == node) {
						continue
					}

					for i := 0; i < len(otherEdge.Points)-1; i++ {
						if node.PassesThrough(otherEdge.Points[i], otherEdge.Points[i+1]) {
							newNumEdgesPassedThrough++
						}
					}
				}

				currSymmetry, err := placementcost.NodeSymmetry(ctx, node, nil)
				if err != nil {
					return err
				}
				if newNumEdgesPassedThrough > numEdgesPassedThrough {
					return layoutgraph.ErrNonImprovingCandidate
				} else if (node.Container != nil) && node.SpillsOutOf(node.Container) {
					return layoutgraph.ErrInvalidCandidate
				} else if currSymmetry < prevSymmetry {
					return layoutgraph.ErrNonImprovingCandidate
				}
				return nil
			})

			if err := txn.Commit(ctx); err != nil {
				txn.Clear()
				if layoutgraph.IsCandidateRejection(err) {
					continue
				}
				return false, err
			}
			if err := txn.UpdateState(); err != nil {
				return false, err
			}
			txn.Clear()

			// ---- from here on we know jittering this node does not introduce any more route intersections

			// Commit new segments
			for _, otherEdge := range node.Edges {

				var isOtherEdgeVertical bool
				if otherEdge.From == node {
					isOtherEdgeVertical = otherEdge.Points[0].X == otherEdge.Points[1].X
				} else {
					isOtherEdgeVertical = otherEdge.Points[len(otherEdge.Points)-1].X == otherEdge.Points[len(otherEdge.Points)-2].X
				}

				isOtherEdgeDiagonal := (len(otherEdge.Points) == 2) && (otherEdge.Points[0].X != otherEdge.Points[1].X) && (otherEdge.Points[0].Y != otherEdge.Points[1].Y)
				// If it's diagonal, we just need to update the one point on the node that's moved
				if isOtherEdgeDiagonal {
					if isVerticalLine {
						if otherEdge.From == node {
							otherEdge.Points[0].X += delta
						} else {
							otherEdge.Points[len(otherEdge.Points)-1].X += delta
						}
					} else {
						if otherEdge.From == node {
							otherEdge.Points[0].Y += delta
						} else {
							otherEdge.Points[len(otherEdge.Points)-1].Y += delta
						}
					}
				} else {
					// Moving horizontally and edge vertical (move both x distance)
					if isVerticalLine && isOtherEdgeVertical {
						if otherEdge.From == node {
							otherEdge.Points[0].X += delta
							otherEdge.Points[1].X += delta
						} else {
							otherEdge.Points[len(otherEdge.Points)-1].X += delta
							otherEdge.Points[len(otherEdge.Points)-2].X += delta
						}
					}
					// Moving horizontally and edge horizontal (move closest x distance)
					if isVerticalLine && !isOtherEdgeVertical {
						if otherEdge.From == node {
							otherEdge.Points[0].X += delta
						} else {
							otherEdge.Points[len(otherEdge.Points)-1].X += delta
						}
					}
					// Moving vertical and edge horizontal (move both y distance)
					if !isVerticalLine && !isOtherEdgeVertical {
						if otherEdge.From == node {
							otherEdge.Points[0].Y += delta
							otherEdge.Points[1].Y += delta
						} else {
							otherEdge.Points[len(otherEdge.Points)-1].Y += delta
							otherEdge.Points[len(otherEdge.Points)-2].Y += delta
						}
					}
					// Moving vertical and edge vertical (move closest y distance)
					if !isVerticalLine && isOtherEdgeVertical {
						if otherEdge.From == node {
							otherEdge.Points[0].Y += delta
						} else {
							otherEdge.Points[len(otherEdge.Points)-1].Y += delta
						}
					}
				}
			}

			// Remove the two defunct points corresponding to the old bends
			if edge.From == node {
				// This should remove 1 and 2
				edge.Points = append(edge.Points[:1], edge.Points[3:]...)
			} else {
				edge.Points = append(edge.Points[:len(edge.Points)-3], edge.Points[len(edge.Points)-1:]...)
			}
			dejittered = true
		}
	}
	if dejittered {
		if err := guard.Finish(); err != nil {
			return false, err
		}
	}
	complete = true
	return dejittered, nil
}
