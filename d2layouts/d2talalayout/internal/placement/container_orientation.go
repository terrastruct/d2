package placement

import (
	"context"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type containerOrientationDisabled struct{}

// A container footprint can change the later placement of its siblings. Until
// those later operations validate self-loop envelopes, leave loop-bearing
// layouts on their existing placement path. This scan happens once per attempt.
func orientationContext(ctx context.Context, g *layoutgraph.Graph) (context.Context, error) {
	guard, err := limits.NewWorkGuard(ctx, "ContainerOrientationPreflight", limits.MaxEngineWorkUnits)
	if err != nil {
		return ctx, err
	}
	for _, e := range g.Edges {
		if err := guard.Step(); err != nil {
			return ctx, err
		}
		if e.IsLoop() {
			return context.WithValue(ctx, containerOrientationDisabled{}, true), nil
		}
	}
	return ctx, guard.Finish()
}

// orientSourceInterior aligns the internal flow of a small source container
// with its surrounding flow, before the parent is fitted around the result.
// The optional move is local: it does not add another complete layout attempt.
func orientSourceInterior(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, obstacles []geo.Box) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ctx.Value(containerOrientationDisabled{}) == true || root == nil || len(g.Nodes) < 3 || len(g.Nodes) > 16 || len(obstacles) != 0 || root.FixedTopLeft != nil || root.Cluster != nil || root.Sequence != nil || root.ForceHierarchy || root.Graph.Direction(root) != geo.NONE {
		return nil
	}
	direction := g.Direction(root.OwningContainer())
	if !direction.IsVertical() || len(root.Edges) < 3 {
		return nil
	}
	guard, err := limits.NewWorkGuard(ctx, "ContainerOrientation", limits.MaxEngineWorkUnits)
	if err != nil {
		return err
	}
	destinations := make(map[*layoutgraph.Node]struct{})
	for _, e := range root.Edges {
		if err := guard.Step(); err != nil {
			return err
		}
		from, to, ok := e.DirectedEndpoints()
		if !ok || from != root || to == root {
			return nil
		}
		destinations[to] = struct{}{}
	}
	if len(destinations) < 2 {
		return nil
	}
	local := make(map[*layoutgraph.Node]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		if n.IsContainer() || n.Hierarchy != nil || n.FixedTopLeft != nil || n.Sequence != nil || g.IsTreeSentinel(n) || g.IsSequenceVessel(n) || n.HerdAssignment != nil {
			return nil
		}
		// Self-loop envelopes do not turn with the boxes. Keep their established
		// placement until orientation can preserve that extra route clearance.
		for _, offset := range n.LoopOffsets {
			if offset > 0 {
				return nil
			}
		}
		local[n] = true
	}
	for _, n := range g.Nodes {
		for near := range n.Nears {
			if !local[near] {
				return nil
			}
		}
	}
	flow, err := interiorFlow(g, local, guard)
	if err != nil {
		return err
	}
	// A quarter-turn needs a clear transverse flow to improve. Disconnected
	// contents, balanced cycles and already vertical compositions are left alone.
	if flow.across <= flow.along+1e-9 || math.Abs(flow.x) <= 1e-9 {
		return nil
	}
	turn := 1.0
	if (flow.x < 0) != (direction == geo.Top) {
		turn = -1
	}

	beforeWidth, beforeHeight := orientationFootprintSize(g, root)
	txn, err := g.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{IgnoreContainerEscape: true})
	if err != nil {
		return err
	}
	txn.AddOp(func() error {
		centers := make(map[*layoutgraph.Node]geo.Point, len(g.Nodes))
		for _, n := range g.Nodes {
			centers[n] = *n.Center()
		}
		for _, n := range g.Nodes {
			if err := guard.Step(); err != nil {
				return err
			}
			if c := g.Clusters[n]; c != nil {
				c.Arrangement = c.Arrangement.Flip()
				c.DesiredArrangement = c.Arrangement
				// Preserve the group's logical member order, but recalculate spacing for
				// its new axis, including icon and outside-label room.
				c.Padding = grouping.PaddingBetween(c, true)
				c.Resize(n)
			}
			p := centers[n]
			n.TopLeft = geo.NewPoint(math.Round(-turn*p.Y-n.Width/2), math.Round(turn*p.X-n.Height/2))
		}
		g.SyncNestedGeometry()
		oldCell := g.CellSize
		g.CellSize = 1
		defer func() { g.CellSize = oldCell }()
		for _, axis := range []layoutAxis{horizontalAxis, verticalAxis} {
			if err := compaction(ctx, g, compactionOptions{axis: axis, includeSizes: true, factor: 1, transition: true}); err != nil {
				return err
			}
		}
		g.SyncNestedGeometry()
		tl, br := g.BoundingBox()
		width, height := orientationFootprintSize(g, root)
		if br.X-tl.X > limits.MaxGraphSize || br.Y-tl.Y > limits.MaxGraphSize || !(width >= 0 && height >= 0 && width <= limits.MaxGraphSize && height <= limits.MaxGraphSize) {
			return layoutgraph.ErrInvalidCandidate
		}
		// Upright boxes can expand substantially during a quarter-turn. Allow
		// at most one additional original footprint, including the containing
		// shape, label room, and authored minimum dimensions.
		rejected := width*height > 2*beforeWidth*beforeHeight
		if rejected {
			return layoutgraph.ErrNonImprovingCandidate
		}
		after, err := interiorFlow(g, local, guard)
		if err != nil {
			return err
		}
		sign := 1.0
		if direction == geo.Top {
			sign = -1
		}
		if sign*after.y <= sign*flow.y+1e-9 {
			return layoutgraph.ErrNonImprovingCandidate
		}
		return guard.Finish()
	})
	// Commit validates node clearances and restores positions, dimensions and
	// cluster metadata on rejection or error. Root fitting happens after success.
	if err := txn.Commit(ctx); err != nil {
		if layoutgraph.IsCandidateRejection(err) {
			return nil
		}
		return err
	}
	return nil
}

// interiorFlow gives each distinct directed adjacency one vote, normalized by
// its Manhattan length so a single long edge cannot dominate the orientation.
type containerFlow struct{ x, y, across, along float64 }

func interiorFlow(g *layoutgraph.Graph, local map[*layoutgraph.Node]bool, guard *limits.WorkGuard) (flow containerFlow, err error) {
	seen := make(map[[2]*layoutgraph.Node]bool)
	for _, e := range g.Edges {
		if err := guard.Step(); err != nil {
			return containerFlow{}, err
		}
		from, to, ok := e.DirectedEndpoints()
		if !ok || from == to || !local[from] || !local[to] {
			continue
		}
		key := [2]*layoutgraph.Node{from, to}
		if seen[key] {
			continue
		}
		seen[key] = true
		a, b := from.Center(), to.Center()
		dx, dy := b.X-a.X, b.Y-a.Y
		length := math.Abs(dx) + math.Abs(dy)
		if length == 0 {
			continue
		}
		flow.x += dx / length
		flow.across += math.Abs(dx) / length
		flow.y += dy / length
		flow.along += math.Abs(dy) / length
	}
	return flow, guard.Finish()
}

// Use the actual fitting path, including label room, shape geometry and authored
// minimum dimensions. Its only mutations are Width/Height; restore both before
// returning so optional admission does not resize the parent early.
func orientationFitsContainer(g *layoutgraph.Graph, root *layoutgraph.Node) bool {
	width, height := orientationFootprintSize(g, root)
	return width >= 0 && height >= 0 && width <= limits.MaxGraphSize && height <= limits.MaxGraphSize
}

// Fitting mutates only the containing node's dimensions. Restore those values
// so observing the original or optional footprint never resizes the parent early.
func orientationFootprintSize(g *layoutgraph.Graph, root *layoutgraph.Node) (width, height float64) {
	oldWidth, oldHeight := root.Width, root.Height
	defer func() { root.Width, root.Height = oldWidth, oldHeight }()
	root.FitToGraph(g, g.ContainerPadding(root, false))
	return root.Width, root.Height
}
