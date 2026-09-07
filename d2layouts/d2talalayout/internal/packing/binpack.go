package packing

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphbounds"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"

	"github.com/d2lang/d2/lib/geo"
)

const (
	subgraphPadding        = 20.0
	subgraphSquareDampener = 0.5
)

// Pack recursively arranges disconnected subgraphs into compact containers.
// Containers may shrink but never grow, and packing may interleave subgraph
// bounding boxes while preserving edge lengths. Hierarchy subgraphs remain
// separate to avoid obstructing routes.
func Pack(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node) error {
	guard, err := newWorkGuard(ctx, limits.MaxBinPackWorkUnits)
	if err != nil {
		return err
	}
	return packAtomic(ctx, g, root, guard)
}

func packAtomic(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, guard *limits.WorkGuard) (err error) {
	if guard == nil {
		return fmt.Errorf("TALA BinPack requires a work guard")
	}
	ctx = layoutgraph.ContextWithTransactionWorkGuard(ctx, guard)
	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{
		CaptureEdgeRoutes: true,
	})
	if err := state.UpdateWithWorkGuard(g, guard); err != nil {
		return err
	}
	snapshot := &layoutgraph.Transaction{Graph: g, PriorGraphState: state}
	complete := false
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot.Rollback()
			panic(recovered)
		}
		if !complete {
			snapshot.Rollback()
		}
	}()
	if err := packGuarded(ctx, g, root, guard); err != nil {
		return err
	}
	if err := guard.Finish(); err != nil {
		return err
	}
	complete = true
	return nil
}

func packGuarded(ctx context.Context, g *layoutgraph.Graph, root *layoutgraph.Node, guard *limits.WorkGuard) error {
	checkCanceled := func() error {
		return guard.Step()
	}
	if err := checkCanceled(); err != nil {
		return err
	}
	edgesPlaced, graphIncidentEdges, err := allEdgesHaveCompleteRoutesGuarded(g, root, guard)
	if err != nil {
		return err
	}

	// Pack more nested before less nested
	for _, n := range g.Containers[root] {
		if err := checkCanceled(); err != nil {
			return err
		}
		if n.IsContainer() {
			if err := packGuarded(ctx, g, n, guard); err != nil {
				return err
			}
		}
	}
	// For this container level, split subgraphs into 3 categories:
	// 0. Fixed nodes and their subgraphs cannot move
	// 1. Cross-container subgraphs (have edges to another container) cannot move
	// 2. Contained subgraphs can move
	var crossContainerSubgraphs []layoutgraph.Nodes
	var containedSubgraphs []layoutgraph.Nodes
	added := make(map[*layoutgraph.Node]struct{})

	// we create the fixedNodesSubgraph first to ensure no subgraph with a fixed node is packed
	// for simplicity we add all nodes to the fixedNodesSubgraph list instead of creating one for each fixed subgraph
	var fixedNodesSubgraph layoutgraph.Nodes
	for _, n := range g.Containers[root] {
		if err := checkCanceled(); err != nil {
			return err
		}
		if n.FixedTopLeft == nil {
			continue
		}

		reachable, err := n.AllReachableNodesContext(true, true, false, map[*layoutgraph.Node]struct{}{root: {}}, guard)
		if err != nil {
			return err
		}
		for _, rn := range reachable {
			if err := guard.Step(); err != nil {
				return err
			}
			if _, was := added[rn]; !was {
				fixedNodesSubgraph = append(fixedNodesSubgraph, rn)
			}
			added[rn] = struct{}{}
		}
	}

	var visitErr error
	shouldVisit := func(n *layoutgraph.Node) bool { return true }
	if root != nil {
		shouldVisit = func(n *layoutgraph.Node) bool {
			if visitErr != nil {
				return false
			}
			var inside bool
			inside, visitErr = binPackIsDescendantOf(n, root, guard)
			return inside
		}
	}

	// 1. get subgraphs contained in root
	// 2. get subgraphs not contained in root
	// 3. filter out subgraphs contained in root from subgraphs not contained in root
	contained := make(map[*layoutgraph.Node]struct{})
	for _, n := range g.Containers[root] {
		if err := checkCanceled(); err != nil {
			return err
		}
		if _, ok := added[n]; ok {
			continue
		}

		reachable, err := n.ReachableNodesContext(shouldVisit, true, true, false, map[*layoutgraph.Node]struct{}{root: {}}, guard)
		if err != nil {
			return err
		}
		if visitErr != nil {
			return visitErr
		}
		cross := false
		for _, rn := range reachable {
			if err := guard.Step(); err != nil {
				return err
			}
			if rn.Hierarchy != nil {
				cross = true
				break
			}
			hasExternalEdge, err := binPackHasExternalConnection(g, rn, root, false, edgesPlaced, guard)
			if err != nil {
				return err
			}
			if hasExternalEdge {
				cross = true
				break
			}
			hasExternalNear, err := binPackHasExternalConnection(g, rn, root, true, false, guard)
			if err != nil {
				return err
			}
			if hasExternalNear {
				cross = true
				break
			}
		}
		if !cross {
			for _, rn := range reachable {
				if err := guard.Step(); err != nil {
					return err
				}
				added[rn] = struct{}{}
				contained[rn] = struct{}{}
			}
			containedSubgraphs = append(containedSubgraphs, layoutgraph.Nodes(reachable))
		}
	}
	for _, n := range g.Containers[root] {
		if err := checkCanceled(); err != nil {
			return err
		}
		if _, ok := added[n]; ok {
			continue
		}
		var filtered []*layoutgraph.Node
		allReachable, err := n.AllReachableNodesContext(true, false, false, map[*layoutgraph.Node]struct{}{root: {}}, guard)
		if err != nil {
			return err
		}
		for _, rn := range allReachable {
			if err := guard.Step(); err != nil {
				return err
			}
			if _, in := contained[rn]; in {
				continue
			}
			filtered = append(filtered, rn)
		}

		for _, rn := range filtered {
			if err := guard.Step(); err != nil {
				return err
			}
			added[rn] = struct{}{}
		}
		crossContainerSubgraphs = append(crossContainerSubgraphs, layoutgraph.Nodes(filtered))
	}

	if len(containedSubgraphs) == 0 {
		if err := checkCanceled(); err != nil {
			return err
		}
		return nil
	}

	packed := crossContainerSubgraphs
	toPack := containedSubgraphs

	if len(fixedNodesSubgraph) > 0 {
		packed = append(packed, fixedNodesSubgraph)
	}

	rootTL, err := binPackContainerTopLeft(g, root, toPack[0], guard)
	if err != nil {
		return err
	}

	originalScore, err := binPackScoreGuarded(layoutgraph.Nodes(g.Containers[root]), root, guard)
	if err != nil {
		return err
	}
	originalPos := make(map[*layoutgraph.Node]pointerSnapshot[geo.Point])
	var originalPosOrder []*layoutgraph.Node
	originalRoute := make(map[*layoutgraph.Edge][]*geo.Point)
	var originalRouteOrder []*layoutgraph.Edge
	returnIfCanceled := func() error {
		return checkCanceled()
	}

	// Move out of the way
	for _, ns := range toPack {
		if err := guard.Step(); err != nil {
			return err
		}
		for _, n := range ns {
			if err := guard.Step(); err != nil {
				return err
			}
			if _, exists := originalPos[n]; !exists {
				originalPos[n] = snapshotPointer(n.TopLeft)
				originalPosOrder = append(originalPosOrder, n)
			}
			n.Translate(100000, 100000)
			if edgesPlaced {
				for _, e := range n.Edges {
					if err := guard.Step(); err != nil {
						return err
					}
					if _, exists := originalRoute[e]; exists {
						continue
					}
					route := []*geo.Point{}
					for _, p := range e.Points {
						if err := guard.Step(); err != nil {
							return err
						}
						route = append(route, p.Copy())
					}
					originalRoute[e] = route
					originalRouteOrder = append(originalRouteOrder, e)
				}
			}
		}
	}

	if len(packed) == 0 {
		currTL, _, err := graphbounds.BoundingBox(toPack[0], guard)
		if err != nil {
			return err
		}
		offsetX := rootTL.X - currTL.X
		offsetY := rootTL.Y - currTL.Y
		for _, n := range toPack[0] {
			if err := guard.Step(); err != nil {
				return err
			}
			n.Translate(offsetX, offsetY)
		}
		if edgesPlaced {
			moved := make(map[*layoutgraph.Edge]struct{})
			for _, n := range toPack[0] {
				if err := guard.Step(); err != nil {
					return err
				}
				for _, e := range n.Edges {
					if err := guard.Step(); err != nil {
						return err
					}
					if _, ok := moved[e]; ok {
						continue
					}
					moved[e] = struct{}{}
					for _, p := range e.Points {
						if err := guard.Step(); err != nil {
							return err
						}
						p.X += (n.TopLeft.X - originalPos[n].value.X)
						p.Y += (n.TopLeft.Y - originalPos[n].value.Y)
					}
				}
			}
		}
		packed = []layoutgraph.Nodes{toPack[0]}
		toPack = toPack[1:]
	}
	if err := returnIfCanceled(); err != nil {
		return err
	}

	var packedRootChildren layoutgraph.Nodes
	var packedSegments []*geo.Segment
	// If edges are already placed, use their actual routes
	// Otherwise consider multiple possible good routes
	if edgesPlaced {
		var edges []*layoutgraph.Edge
		unique := make(map[*layoutgraph.Edge]struct{})
		for _, ns := range packed {
			if err := guard.Step(); err != nil {
				return err
			}
			for _, n := range ns {
				if err := guard.Step(); err != nil {
					return err
				}
				if n == root {
					continue
				}
				inside, err := binPackIsDescendantOf(n, root, guard)
				if err != nil {
					return err
				}
				if !inside {
					continue
				}
				for _, e := range n.Edges {
					if err := guard.Step(); err != nil {
						return err
					}
					if _, ok := unique[e]; !ok {
						unique[e] = struct{}{}
						edges = append(edges, e)
					}
				}

			}
		}
		hSegments, err := routedEdgeSegments(layoutgraph.Edges(edges), true, guard)
		if err != nil {
			return err
		}
		vSegments, err := routedEdgeSegments(layoutgraph.Edges(edges), false, guard)
		if err != nil {
			return err
		}
		for _, s := range hSegments {
			if err := guard.Step(); err != nil {
				return err
			}
			packedSegments = append(packedSegments, &s.Segment)
		}
		for _, s := range vSegments {
			if err := guard.Step(); err != nil {
				return err
			}
			packedSegments = append(packedSegments, &s.Segment)
		}
		for _, ns := range packed {
			if err := guard.Step(); err != nil {
				return err
			}
			for _, n := range ns {
				if err := guard.Step(); err != nil {
					return err
				}
				if n.Container == root {
					packedRootChildren = append(packedRootChildren, n)
				}
			}
		}
	} else {
		packedSegments = make([]*geo.Segment, 0, len(packed[0])*10)
		for _, ns := range packed {
			if err := guard.Step(); err != nil {
				return err
			}
			for _, n := range ns {
				if err := guard.Step(); err != nil {
					return err
				}
				if n.Container == root {
					packedRootChildren = append(packedRootChildren, n)
				}
			}
			xd, yd, err := binPackSmallestDeltas(toPack, guard)
			if err != nil {
				return err
			}

			// We only care about segments of packed nodes that are descendants of root
			// There's no chance of collision of a subgraph with a more nested node or less nested node
			var filtered layoutgraph.Nodes
			for _, n := range ns {
				if err := guard.Step(); err != nil {
					return err
				}
				if n == root {
					continue
				}
				inside, err := binPackIsDescendantOf(n, root, guard)
				if err != nil {
					return err
				}
				if inside {
					filtered = append(filtered, n)
				}
			}
			segments, err := estimateRouteSegments(filtered, g, xd, yd, guard)
			if err != nil {
				return err
			}
			for _, segment := range segments {
				if err := guard.Step(); err != nil {
					return err
				}
				packedSegments = append(packedSegments, segment)
			}
		}
	}

	occupied := make(map[geo.Point]struct{})

	canMoveLeft := true
	canMoveRight := true
	canMoveTop := true
	canMoveBottom := true

	if edgesPlaced && root != nil {
		for _, e := range root.Edges {
			if err := guard.Step(); err != nil {
				return err
			}
			var seg *geo.Segment
			if e.From == root {
				seg = geo.NewSegment(e.Points[0], e.Points[1])
			} else {
				seg = geo.NewSegment(e.Points[len(e.Points)-1], e.Points[len(e.Points)-2])
			}
			if seg.Start.Y == seg.End.Y {
				if seg.Start.X < seg.End.X {
					canMoveRight = false
				}
				if seg.Start.X > seg.End.X {
					canMoveLeft = false
				}
			} else if seg.Start.X == seg.End.X {
				if seg.Start.Y < seg.End.Y {
					canMoveBottom = false
				}
				if seg.Start.Y > seg.End.Y {
					canMoveTop = false
				}
			} else {
				// It's a diagonal line, can't risk any movement
				canMoveRight = false
				canMoveLeft = false
				canMoveTop = false
				canMoveBottom = false
				break
			}
		}
	}

	failed := false
	// place greedily
	// TODO perhaps there's better algorithms here
	// Maybe we can multithread race some random orders and get the best scored one
	for len(toPack) > 0 {
		if err := returnIfCanceled(); err != nil {
			return err
		}
		var candidates []*geo.Point
		set := make(map[geo.Point]struct{})
		hierarchyBoxes, err := binPackHierarchyBoxes(packed, guard)
		if err != nil {
			return err
		}

		tl := geo.NewPoint(rootTL.X, rootTL.Y)
		if _, ok := set[*tl]; !ok {
			candidates = append(candidates, tl)
			set[*tl] = struct{}{}
		}

		packedTLX := math.Inf(1)
		packedTLY := math.Inf(1)
		for _, ns := range packed {
			if err := guard.Step(); err != nil {
				return err
			}
			tl, _, err := graphbounds.BoundingBox(ns, guard)
			if err != nil {
				return err
			}
			packedTLX = math.Min(packedTLX, tl.X)
			packedTLY = math.Min(packedTLY, tl.Y)
			canPlaceWithin := ns[0].Hierarchy == nil
			placementCandidates, err := placementCandidatesGuarded(ns, root, canPlaceWithin, edgesPlaced, guard)
			if err != nil {
				return err
			}
			for _, p := range placementCandidates {
				if err := guard.Step(); err != nil {
					return err
				}
				if _, ok := occupied[*p]; ok {
					continue
				}
				insideHierarchy, err := binPackPointInHierarchy(hierarchyBoxes, p, guard)
				if err != nil {
					return err
				}
				if insideHierarchy {
					continue
				}
				if _, ok := set[*p]; !ok {
					candidates = append(candidates, p)
					set[*p] = struct{}{}
				}
			}
		}

		currPacking := toPack[0]
		toPack = toPack[1:]
		currPackingTL, currPackingBR, err := graphbounds.FixedBoundingBox(currPacking, guard)
		if err != nil {
			return err
		}
		currPackingWidth := currPackingBR.X - currPackingTL.X
		currPackingHeight := currPackingBR.Y - currPackingTL.Y

		// TODO ideally the placement candidates should also take into account which direction it can move
		if canMoveTop {
			candidates = append(candidates, geo.NewPoint(packedTLX, packedTLY-currPackingHeight-subgraphPadding))
		}
		if canMoveLeft {
			candidates = append(candidates, geo.NewPoint(packedTLX-currPackingWidth-subgraphPadding, packedTLY))
		}
		if canMoveTop && canMoveLeft {
			candidates = append(candidates, geo.NewPoint(packedTLX-currPackingWidth-subgraphPadding, packedTLY-currPackingHeight-subgraphPadding))
		}

		txn, err := g.NewRequestTransaction(ctx, layoutgraph.TransactionOptions{
			IgnoreContainerEscape: true,
			AffectEdgeRoutes:      edgesPlaced,
		})
		if err != nil {
			return err
		}

		currTL, _, err := graphbounds.BoundingBox(currPacking, guard)
		if err != nil {
			return err
		}
		packedWithCurr := make(layoutgraph.Nodes, 0, len(packedRootChildren)+len(currPacking))
		for _, node := range packedRootChildren {
			if err := guard.Step(); err != nil {
				return err
			}
			packedWithCurr = append(packedWithCurr, node)
		}
		for _, node := range currPacking {
			if err := guard.Step(); err != nil {
				return err
			}
			packedWithCurr = append(packedWithCurr, node)
		}
		bestScore := math.Inf(1)

		for _, n := range currPacking {
			if err := guard.Step(); err != nil {
				return err
			}
			original := originalPos[n].value
			og := original.Copy()
			if _, ok := set[*og]; !ok {
				candidates = append(candidates, og)
				set[*og] = struct{}{}
			}
		}
		packedXD, packedYD, err := binPackSmallestDeltas(packed, guard)
		if err != nil {
			return err
		}

		var bestCandidate *geo.Point
		for _, p := range candidates {
			if err := returnIfCanceled(); err != nil {
				return err
			}
			txn.Clear()
			txn.AddOp(func() error {
				offsetX := p.X - currTL.X
				offsetY := p.Y - currTL.Y
				for _, n := range currPacking {
					if err := guard.Step(); err != nil {
						return err
					}
					n.Translate(offsetX, offsetY)
				}
				return nil
			})
			if err := txn.Commit(ctx); err != nil {
				if layoutgraph.IsCandidateRejection(err) {
					// TODO handle candidate overlap with the root before committing.
					continue
				}
				if canceledErr := checkCanceled(); canceledErr != nil {
					return canceledErr
				}
				return err
			}
			// Not allowed to expand out of containers
			if root != nil {
				var packedInContainer layoutgraph.Nodes
				for _, n := range packedWithCurr {
					if err := guard.Step(); err != nil {
						txn.Rollback()
						return err
					}
					if n.Container == root {
						packedInContainer = append(packedInContainer, n)
					}
				}
				tl, br, err := graphbounds.BoundingBox(packedInContainer, guard)
				if err != nil {
					txn.Rollback()
					return err
				}
				w, h := br.X-tl.X, br.Y-tl.Y
				innerBox := root.InnerBox()
				if w > innerBox.Width || h > innerBox.Height {
					txn.Rollback()
					continue
				}
			}
			if !edgesPlaced {
				// Not allowed for any node to go into the bounding box of a placed hierarchy
				is := false
				for _, n := range currPacking {
					tl, br, err := graphbounds.NodeBoundingBox(n, nil, guard)
					if err != nil {
						txn.Rollback()
						return err
					}
					topLeftInside, err := binPackPointInHierarchy(hierarchyBoxes, tl, guard)
					if err != nil {
						txn.Rollback()
						return err
					}
					bottomRightInside, err := binPackPointInHierarchy(hierarchyBoxes, br, guard)
					if err != nil {
						txn.Rollback()
						return err
					}
					if topLeftInside || bottomRightInside {
						is = true
						break
					}
				}
				if is {
					txn.Rollback()
					continue
				}
			}
			blocked, err := blocksRoutesWithDeltasGuarded(
				g, currPacking, packed, packedSegments, packedXD, packedYD, guard,
			)
			if err != nil {
				txn.Rollback()
				return err
			}
			if blocked {
				txn.Rollback()
				continue
			}
			score, err := binPackScoreGuarded(packedWithCurr, root, guard)
			if err != nil {
				txn.Rollback()
				return err
			}
			if score < bestScore {
				bestScore = score
				bestCandidate = p
			}
			txn.Rollback()
		}
		if err := returnIfCanceled(); err != nil {
			return err
		}

		if bestCandidate == nil {
			failed = true
			break
		}

		txn.Clear()
		txn.AddOp(func() error {
			offsetX := bestCandidate.X - currTL.X
			offsetY := bestCandidate.Y - currTL.Y
			for _, n := range currPacking {
				if err := guard.Step(); err != nil {
					return err
				}
				n.Translate(offsetX, offsetY)
			}
			if edgesPlaced {
				moved := make(map[*layoutgraph.Edge]struct{})
				for _, n := range currPacking {
					if err := guard.Step(); err != nil {
						return err
					}
					for _, e := range n.Edges {
						if err := guard.Step(); err != nil {
							return err
						}
						if _, ok := moved[e]; ok {
							continue
						}
						moved[e] = struct{}{}
						for _, p := range e.Points {
							if err := guard.Step(); err != nil {
								return err
							}
							p.X += (n.TopLeft.X - originalPos[n].value.X)
							p.Y += (n.TopLeft.Y - originalPos[n].value.Y)
						}
					}
				}
			}
			return nil
		})
		if err := txn.Commit(ctx); err != nil {
			if canceledErr := checkCanceled(); canceledErr != nil {
				return canceledErr
			}
			return err
		}
		occupied[*bestCandidate] = struct{}{}

		for _, node := range currPacking {
			if err := guard.Step(); err != nil {
				return err
			}
			packedRootChildren = append(packedRootChildren, node)
		}
		packed = append(packed, currPacking)
		xd, yd, err := binPackSmallestDeltas(toPack, guard)
		if err != nil {
			return err
		}

		if edgesPlaced {
			var edges []*layoutgraph.Edge
			unique := make(map[*layoutgraph.Edge]struct{})
			for _, n := range currPacking {
				if err := guard.Step(); err != nil {
					return err
				}
				if n == root {
					continue
				}
				inside, err := binPackIsDescendantOf(n, root, guard)
				if err != nil {
					return err
				}
				if !inside {
					continue
				}
				for _, e := range n.Edges {
					if err := guard.Step(); err != nil {
						return err
					}
					if _, ok := unique[e]; !ok {
						unique[e] = struct{}{}
						edges = append(edges, e)
					}
				}
			}
			hSegments, err := routedEdgeSegments(layoutgraph.Edges(edges), true, guard)
			if err != nil {
				return err
			}
			vSegments, err := routedEdgeSegments(layoutgraph.Edges(edges), false, guard)
			if err != nil {
				return err
			}
			for _, s := range hSegments {
				if err := guard.Step(); err != nil {
					return err
				}
				packedSegments = append(packedSegments, &s.Segment)
			}
			for _, s := range vSegments {
				if err := guard.Step(); err != nil {
					return err
				}
				packedSegments = append(packedSegments, &s.Segment)
			}
		} else {
			var filtered layoutgraph.Nodes
			for _, n := range currPacking {
				if err := guard.Step(); err != nil {
					return err
				}
				if n == root {
					continue
				}
				inside, err := binPackIsDescendantOf(n, root, guard)
				if err != nil {
					return err
				}
				if inside {
					filtered = append(filtered, n)
				}
			}

			segments, err := estimateRouteSegments(filtered, g, xd, yd, guard)
			if err != nil {
				return err
			}
			for _, segment := range segments {
				if err := guard.Step(); err != nil {
					return err
				}
				packedSegments = append(packedSegments, segment)
			}
		}
	}
	if err := returnIfCanceled(); err != nil {
		return err
	}

	var afterBinPackScore float64
	if failed {
		afterBinPackScore = math.Inf(1)
	} else {
		afterBinPackScore, err = binPackScoreGuarded(layoutgraph.Nodes(g.Containers[root]), root, guard)
		if err != nil {
			return err
		}
	}
	if originalScore < afterBinPackScore {
		for _, node := range originalPosOrder {
			if err := guard.Step(); err != nil {
				return err
			}
			node.TopLeft = originalPos[node].restore()
		}
		if err := binPackSyncClusters(g, guard); err != nil {
			return err
		}
		if err := binPackSyncSequences(g, guard); err != nil {
			return err
		}
		failed = true
	} else if root != nil && root.Cluster == nil {
		// record state to revert to if fitting to the children's packed position causes a bad state
		// we need to save positions of packed children not in toPack
		descendants, err := g.AllDescendantNodesWithWorkGuard(root, true, guard)
		if err != nil {
			return err
		}
		for _, node := range descendants {
			if err := guard.Step(); err != nil {
				return err
			}
			if _, in := originalPos[node]; !in {
				originalPos[node] = snapshotPointer(node.TopLeft)
				originalPosOrder = append(originalPosOrder, node)
			}
		}

		rootX := root.TopLeft.X
		rootY := root.TopLeft.Y
		rootWidth, rootHeight := root.Width, root.Height
		originalRootBox := geo.Box{
			TopLeft: geo.NewPoint(rootX, rootY),
			Width:   rootWidth,
			Height:  rootHeight,
		}

		if err := binPackWrapChildren(root, guard); err != nil {
			return err
		}
		routedContainerHandled := false
		if edgesPlaced {
			decision, err := binPackCanUseRoutedContainerBox(
				g, root, &originalRootBox, graphIncidentEdges, guard,
			)
			if err != nil {
				return err
			}
			switch decision {
			case routedContainerDeferToSideConstraints:
			case routedContainerUseProposedBox:
				routedContainerHandled = true
			case routedContainerKeepOriginalBox:
				routedContainerHandled = true
				// Unsupported shapes, detached endpoints, and route geometry cut
				// by the proposed shrink retain the exact routed box.
				root.TopLeft.X = rootX
				root.TopLeft.Y = rootY
				root.Width = rootWidth
				root.Height = rootHeight
			default:
				return fmt.Errorf("TALA BinPack received an invalid routed-container decision")
			}
		}
		if !routedContainerHandled {
			if !canMoveLeft {
				root.TopLeft.X = rootX
			}
			if !canMoveTop {
				root.TopLeft.Y = rootY
			}
			if !canMoveRight {
				root.Width = rootWidth
			}
			if !canMoveBottom {
				root.Height = rootHeight
			}
			// wrapChildren preserves the aspect ratio of circles and other square
			// shapes. Restoring one route-constrained side independently can break
			// that invariant, so retain the larger constrained dimension on both
			// axes. The child-fit and bad-state checks below still reject an unsafe
			// expansion and restore the complete original box.
			if root.AspectRatio1() {
				size := math.Max(root.Width, root.Height)
				root.Width = size
				root.Height = size
			}
		}

		childrenFit := true
		innerBox := root.InnerBox()
		for _, child := range g.Containers[root] {
			if err := guard.Step(); err != nil {
				return err
			}
			if !layoutgraph.Covers(innerBox, &child.Box) {
				childrenFit = false
				break
			}
		}
		badState, err := g.IsBadStateWithWorkGuard(root, nil, false, guard)
		if err != nil {
			return err
		}
		if !childrenFit || badState {
			for _, node := range originalPosOrder {
				if err := guard.Step(); err != nil {
					return err
				}
				node.TopLeft = originalPos[node].restore()
			}
			root.TopLeft.X = rootX
			root.TopLeft.Y = rootY
			root.Width = rootWidth
			root.Height = rootHeight
			failed = true
		}
	}
	// Transactions don't deal with edge routes, so rollback manually
	if failed {
		for _, edge := range originalRouteOrder {
			if err := guard.Step(); err != nil {
				return err
			}
			route := originalRoute[edge]
			for i, p := range route {
				if err := guard.Step(); err != nil {
					return err
				}
				edge.Points[i].X = p.X
				edge.Points[i].Y = p.Y
			}
		}
	}
	return guard.Finish()
}

// placementCandidates returns the candidate points in the graph that are suitable to pack more nodes/subgraphs at
// if considerWithin is true, candidate points include points inside the bounding box of the graph
// Currently (Sept 24, 2022), that is false during node placement stage and true for the more global packing stage.

// E.g., the "@" are always included
// the "*" is included iff considerWithin is true
// ┌─────────────┬───┬─*─────┐ @
// │             │   │       │
// │             └───┘ *     │
// ├───┐ *       *           │
// │   │                 ┌───┤ *
// ├───┘ *               │   │
// *                     └───┤ *
// │      ┌───┐ *        *   │
// │      │   │              │
// └──────┴───┴─*────────────┘ @
// @      *
func placementCandidatesGuarded(nodes layoutgraph.Nodes,
	root *layoutgraph.Node,
	considerWithin, edgesPlaced bool,
	guard *limits.WorkGuard,
) ([]*geo.Point, error) {
	var candidatePoints []*geo.Point
	var inContainer layoutgraph.Nodes
	for _, n := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		// Avoid placement points that'd move into different container
		if n.Container != root {
			continue
		}
		inContainer = append(inContainer, n)
		if !considerWithin {
			continue
		}
		sideDelta := subgraphPadding
		if n.IsTable() {
			sideDelta = layoutgraph.TableNodeGap
		}
		if edgesPlaced {
			candidatePoints = append(candidatePoints, geo.NewPoint(n.TopLeft.X+n.Width+sideDelta, n.TopLeft.Y))
			candidatePoints = append(candidatePoints, geo.NewPoint(n.TopLeft.X, n.TopLeft.Y+n.Height+subgraphPadding))
			candidatePoints = append(candidatePoints, geo.NewPoint(n.TopLeft.X+n.Width+sideDelta, n.TopLeft.Y+n.Height))
		} else {
			// Avoid placement points that are likely to cause obstructions
			hasRight := false
			hasBottom := false
			hasBottomRight := false
			for _, e := range n.Edges {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				adj := n.Adjacent(e)
				switch n.Orientation(adj) {
				case geo.Left:
					hasRight = true
				case geo.Top:
					hasBottom = true
				case geo.TopLeft:
					hasBottomRight = true
				}
				if hasRight && hasBottom && hasBottomRight {
					break
				}
			}

			if !hasRight {
				candidatePoints = append(candidatePoints, geo.NewPoint(n.TopLeft.X+n.Width+sideDelta, n.TopLeft.Y))
			}
			if !hasBottom {
				candidatePoints = append(candidatePoints, geo.NewPoint(n.TopLeft.X, n.TopLeft.Y+n.Height+subgraphPadding))
			}
			if !hasBottomRight {
				candidatePoints = append(candidatePoints, geo.NewPoint(n.TopLeft.X+n.Width+sideDelta, n.TopLeft.Y+n.Height))
			}
		}
	}

	graphTL, graphBR, err := graphbounds.FixedBoundingBox(inContainer, guard)
	if err != nil {
		return nil, err
	}

	candidatePoints = append(candidatePoints, geo.NewPoint(graphBR.X, graphTL.Y),
		geo.NewPoint(graphBR.X, graphBR.Y),
		geo.NewPoint(graphTL.X, graphBR.Y),
		geo.NewPoint(graphBR.X+subgraphPadding, graphTL.Y),
		geo.NewPoint(graphBR.X+subgraphPadding, graphBR.Y),
		geo.NewPoint(graphTL.X, graphBR.Y+subgraphPadding),
	)
	return candidatePoints, guard.Finish()
}

// blocksRoutesWithDeltasGuarded checks if the given `packing` placement overlaps with some desired routes in `packed`
// or if `packed` nodes overlap with routes in `packing`.
// Desired routes are roughly estimated as L-shaped and S-shaped routes between connected nodes
// For example, we want to avoid bin packing to the positions shown as `#` below
// . ┌─────────┐
// . │         │      #####
// . │         ├──────#####
// . └────┬────┘      #####
// .      │               │
// .      │          ┌────┴────┐
// .      ####       │         │
// .      ####───────┤         │
// .      ####       └─────────┘
// TODO: if top-right and bottom-left are available, it can overlap with one of them as it would be able
// to generate a good route using the remaining one
//
// The function checks one candidate using the gaps for the immutable packed
// set. BinPack evaluates many translations of the same
// packing against that set, so its caller computes these deltas once per set
// instead of rescanning every packed node for every candidate.
func blocksRoutesWithDeltasGuarded(
	g *layoutgraph.Graph,
	packing layoutgraph.Nodes,
	packed []layoutgraph.Nodes,
	packedSegments []*geo.Segment,
	xd, yd float64,
	guard *limits.WorkGuard,
) (bool, error) {
	if guard == nil {
		return false, fmt.Errorf("TALA BinPack route check requires a work guard")
	}
	packingSegments, err := estimateRouteSegments(packing, g, xd, yd, guard)
	if err != nil {
		return false, err
	}

	// check if `packed` nodes could block routes routes in `packing`
	for _, nodes := range packed {
		if err := guard.Step(); err != nil {
			return false, err
		}
		for _, n := range nodes {
			if err := guard.Step(); err != nil {
				return false, err
			}
			for _, seg := range packingSegments {
				if err := guard.Step(); err != nil {
					return false, err
				}
				if n.OverlapsLine(seg.Start, seg.End, layoutgraph.NodeGap) {
					if err := guard.Finish(); err != nil {
						return false, err
					}
					return true, nil
				}
			}
		}
	}
	// check if `packing` nodes could block routes in `packed`
	for _, n := range packing {
		if err := guard.Step(); err != nil {
			return false, err
		}
		for _, seg := range packedSegments {
			if err := guard.Step(); err != nil {
				return false, err
			}
			if n.OverlapsLine(seg.Start, seg.End, layoutgraph.NodeGap) {
				if err := guard.Finish(); err != nil {
					return false, err
				}
				return true, nil
			}
		}
	}

	if err := guard.Finish(); err != nil {
		return false, err
	}
	return false, nil
}

// estimateRouteSegments generates all L-shaped and S-shaped estimates between
// connected nodes, sharing the complete BinPack operation's guard.
func estimateRouteSegments(
	nodes layoutgraph.Nodes,
	g *layoutgraph.Graph,
	smallestXGap, smallestYGap float64,
	guard *limits.WorkGuard,
) ([]*geo.Segment, error) {
	// just an estimate: 1 edge per node
	segments := make([]*geo.Segment, 0, len(nodes)*6)
	unique := make(map[*layoutgraph.Edge]struct{})
	for _, n := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		for _, e := range n.Edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			if _, exists := unique[e]; exists {
				continue
			}
			unique[e] = struct{}{}
			edgeSegments, err := estimateEdgeSegments(e, g, smallestXGap, smallestYGap, guard)
			if err != nil {
				return nil, err
			}
			segments = append(segments, edgeSegments...)
		}
	}
	return segments, guard.Finish()
}

func routedEdgeSegments(edges layoutgraph.Edges, isHorizontal bool, guard *limits.WorkGuard) ([]*layoutgraph.EdgeSegment, error) {
	out := make([]*layoutgraph.EdgeSegment, 0)
	for _, edge := range edges {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if edge == nil {
			return nil, invariant.New("BinPack routed segment scan encountered a nil edge")
		}
		for index := 0; index < len(edge.Points)-1; index++ {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			first, second := edge.Points[index], edge.Points[index+1]
			if first == nil || second == nil {
				return nil, invariant.New("BinPack routed segment scan encountered a nil point")
			}
			var start, end *geo.Point
			if isHorizontal {
				if first.Y == second.Y {
					if first.X < second.X {
						start, end = first, second
					} else {
						start, end = second, first
					}
				}
			} else if first.X == second.X {
				if first.Y < second.Y {
					start, end = first, second
				} else {
					start, end = second, first
				}
			}
			if start != nil && end != nil {
				out = append(out, layoutgraph.NewEdgeSegment(start, end, edge))
			}
		}
	}
	return out, guard.Finish()
}

// estimateEdgeSegments generates horizontal and vertical segments for the given edge.
// It only generates the segments for diagonal edges, as these are the ones that can have S/L-shaped routes.
// There's an exception for cluster vessels, in which is is allowed to be vertical or horizontal.
// . ┌──────────┐
// . │          ├───────────1──────────────┐
// . │          │                          │
// . │          ├─────8───┐                │
// . └───┬───┬──┘         │                2
// .     │   5            │                │
// .     3   │            9                │
// .     │   └─────6──────┼───────────┐    │
// .     │                │           7    │
// .     │                │        ┌──┴────┴────┐
// .     │                │        │            │
// .     │                └──10────┤            │
// .     └───4─────────────────────┤            │
// .                               └────────────┘
// Didn't want to revise the ascii art, but we don't need 8, 10, 5, and 7, they overlap with others
func estimateEdgeSegments(e *layoutgraph.Edge, g *layoutgraph.Graph, smallestXGap, smallestYGap float64, guard *limits.WorkGuard) ([]*geo.Segment, error) {
	if err := guard.Step(); err != nil {
		return nil, err
	}
	if e == nil || e.From == nil || e.To == nil || e.From.TopLeft == nil || e.To.TopLeft == nil {
		return nil, invariant.New("BinPack segment estimate encountered an incomplete edge")
	}
	o := e.From.Orientation(e.To)

	// Straight lines only matter if they're long (can potentially fit other subgraphs)
	if !o.IsDiagonal() && !e.From.IsClusterVessel() && !e.To.IsClusterVessel() {
		from := e.From.Center()
		to := e.To.Center()

		if o.IsHorizontal() {
			if math.Abs(from.X-to.X) > smallestXGap {
				return []*geo.Segment{geo.NewSegment(from, to)}, nil
			}
		}

		if o.IsVertical() {
			if math.Abs(from.Y-to.Y) > smallestYGap {
				return []*geo.Segment{geo.NewSegment(from, to)}, nil
			}
		}

		return nil, nil
	}

	// for clusters, as they can be properly aligned vertically or horizontally
	// generate the segments using the endpoints depending on the arrangement
	//               ┌───────────┐                     ┌───────────┐
	//               │           │                     │endpoint 1 │
	//               │           │                     └─────┬─────┘
	//               │           │                           │
	//               │           │                           │
	//               │           │                           │
	// ┌───────┐     │           │       ┌───────┐           │
	// │       │     │           │       │       ├───────────┘
	// │  n    ├─────┤ cluster   │       │   n   │
	// │       │     │           │       │       ├───────────┐
	// └───────┘     │           │       └───────┘           │
	//               │           │                           │
	//               │           │                           │
	//               │           │                           │
	//               │           │                           │
	//               │           │                     ┌─────┴─────┐
	//               │           │                     │endpoint 2 │
	//               └───────────┘                     └───────────┘
	clusterEndpoints := func(c *layoutgraph.Cluster) []*geo.Point {
		center := c.Vessel.Center()
		tl, br := c.Vessel.BoundingBox(nil)
		if c.Arrangement == layoutgraph.Row {
			return []*geo.Point{geo.NewPoint(tl.X, center.Y), geo.NewPoint(br.X, center.Y)}
		}
		if c.Arrangement == layoutgraph.Column {
			return []*geo.Point{geo.NewPoint(center.X, tl.Y), geo.NewPoint(center.X, br.Y)}
		}

		return nil
	}
	// for non-cluster nodes, it generates 10 segments
	segments := make([]*geo.Segment, 0, 6)
	var fromPoints []*geo.Point
	var toPoints []*geo.Point
	if e.From.IsClusterVessel() {
		fromPoints = clusterEndpoints(g.Clusters[e.From])
	} else {
		fromPoints = []*geo.Point{e.From.Center()}
	}
	if e.To.IsClusterVessel() {
		toPoints = clusterEndpoints(g.Clusters[e.To])
	} else {
		toPoints = []*geo.Point{e.To.Center()}
	}

	for _, from := range fromPoints {
		for _, to := range toPoints {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			// L-shaped estimate (segments 1-4 in the method doc above)
			segments = append(segments, geo.NewSegment(from, geo.NewPoint(from.X, to.Y)))
			segments = append(segments, geo.NewSegment(geo.NewPoint(from.X, to.Y), to))
			segments = append(segments, geo.NewSegment(from, geo.NewPoint(to.X, from.Y)))
			segments = append(segments, geo.NewSegment(geo.NewPoint(to.X, from.Y), to))
			// S-shaped estimate (segments 5-10 in the method doc above)
			midX := math.Round((from.X + to.X) / 2.)
			segments = append(segments, geo.NewSegment(geo.NewPoint(midX, from.Y), geo.NewPoint(midX, to.Y)))
			midY := math.Round((from.Y + to.Y) / 2.)
			segments = append(segments, geo.NewSegment(geo.NewPoint(from.X, midY), geo.NewPoint(to.X, midY)))
		}
	}

	return segments, guard.Finish()
}
