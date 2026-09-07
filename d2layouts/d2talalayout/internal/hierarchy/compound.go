package hierarchy

import (
	"context"
	"math"
	"math/rand"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/nodeshape"
	"github.com/d2lang/d2/lib/geo"
)

// A compound candidate reuses already laid-out interiors as rigid blocks. The
// first implementation is deliberately bounded to one connected outer graph;
// separate outer components must not be expanded into one another before routing.
const maxCompoundBlocks = 64
const maxCompoundInterfaces = 256

type compoundBlock struct {
	original *layoutgraph.Node
	proxy    *layoutgraph.Node
	members  []*layoutgraph.Node
}

type compoundInterface struct {
	from, to *compoundBlock
	// Coordinates relative to each block retain the ordering of real descendant
	// endpoints. They are projected to the appropriate boundary after placement.
	fromOffset, toOffset geo.Point
}

// PlaceCompound lays out the outer flow of detailed containers after their
// interiors have been placed and rescaled, but before edge routing. It preserves
// every interior coordinate relative to its container, including local directions
// and specialized inner layouts. The caller compares this complete candidate
// against ordinary placement; this stage is not an unconditional improvement.
func PlaceCompound(ctx context.Context, g *layoutgraph.Graph, random *rand.Rand) (changed bool, err error) {
	if err := layoutgraph.Validate(ctx, "PlaceCompound", g); err != nil {
		return false, err
	}
	if len(g.Containers[nil]) < 3 || len(g.Containers[nil]) > maxCompoundBlocks {
		return false, nil
	}
	ctx, guard, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "PlaceCompoundTransactions")
	if err != nil {
		return false, err
	}
	proxy := layoutgraph.NewGraph()
	proxy.Directions[nil] = g.Direction(nil)
	blocks := make([]*compoundBlock, 0, len(g.Containers[nil]))
	byRoot := make(map[*layoutgraph.Node]*compoundBlock)
	for _, root := range g.Containers[nil] {
		if err := guard.Step(); err != nil {
			return false, err
		}
		n := layoutgraph.NewNode(root.ID, root.Width, root.Height)
		n.TopLeft = root.TopLeft.Copy()
		proxy.AddNewNodeToContainer(nil, n)
		block := &compoundBlock{original: root, proxy: n}
		blocks = append(blocks, block)
		byRoot[root] = block
	}
	owner := make(map[*layoutgraph.Node]*compoundBlock, len(g.Nodes))
	for _, node := range g.Nodes {
		if err := guard.Step(); err != nil {
			return false, err
		}
		// Absolute coordinates and active grouping vessels are not rigid-block input.
		if node.FixedTopLeft != nil || g.IsSequenceVessel(node) || node.IsClusterVessel() {
			return false, nil
		}
		root := node
		for root.Container != nil {
			if err := guard.Step(); err != nil {
				return false, err
			}
			root = root.Container
		}
		block := byRoot[root]
		if block == nil {
			return false, nil
		}
		block.members = append(block.members, node)
		owner[node] = block
	}
	// Trees must already be restored into the ordinary node collection. A tree
	// sentinel can be an authored node; it is safe to move once all members exist.
	for node := range g.NodeToTree {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if owner[node] == nil {
			return false, nil
		}
	}
	interfaces := make([]compoundInterface, 0)
	detailed := false
	for _, edge := range g.Edges {
		if err := guard.Step(); err != nil {
			return false, err
		}
		from, to := owner[edge.From], owner[edge.To]
		if from == nil || to == nil {
			return false, nil
		}
		if from == to {
			detailed = detailed || (from.original.IsContainer() && edge.From != edge.To)
			continue
		}
		if len(interfaces) == maxCompoundInterfaces {
			return false, nil
		}
		e := proxy.Connect(from.proxy, to.proxy)
		e.ID = edge.ID
		e.SourceArrowhead = edge.SourceArrowhead
		e.TargetArrowhead = edge.TargetArrowhead
		if edge.Label != nil {
			copied := *edge.Label
			e.Label = &copied
		}
		interfaces = append(interfaces, compoundInterface{
			from: from, to: to,
			fromOffset: compoundEndpointOffset(edge.From, from.original, edge.FromTableColumnIndex),
			toOffset:   compoundEndpointOffset(edge.To, to.original, edge.ToTableColumnIndex),
		})
	}
	if !detailed || !compoundConnected(blocks, interfaces) {
		return false, nil
	}
	// The compactness heuristics for ordinary nodes do not decide whether
	// rearranging these opaque blocks helps the completed drawing. Rank only
	// this isolated proxy, then retain the compound proposal's original directed
	// backbone requirement. The caller compares its finished route geometry.
	outerHierarchy, err := build(ctx, proxy, true, Candidates(proxy), nil)
	if err != nil {
		return false, err
	}
	if outerHierarchy == nil {
		return false, nil
	}
	forward, other := countEdgeDirection(outerHierarchy, proxy)
	if outerHierarchy.LevelCount < 2 || forward == 0 || float64(forward) < 1.5*float64(other) {
		return false, nil
	}
	for _, node := range proxy.Nodes {
		if err := guard.Step(); err != nil {
			return false, err
		}
		node.Hierarchy = outerHierarchy
	}
	if err := Place(ctx, proxy, nil, random); err != nil {
		return false, err
	}
	if err := orderCompoundInterfaces(blocks, interfaces, proxy.Direction(nil).IsHorizontal(), guard); err != nil {
		return false, err
	}
	tl, br := layoutgraph.Nodes(proxy.Nodes).BoundingBox()
	if br.X-tl.X > limits.MaxGraphSize || br.Y-tl.Y > limits.MaxGraphSize {
		return false, nil
	}
	// All planning is isolated. Snapshot only when there is an applicable plan, and
	// keep mutations atomic even if cancellation arrives during rigid translation.
	state := layoutgraph.NewGraphStateSnapshot(layoutgraph.GraphStateSnapshotOptions{CaptureTopology: true, CaptureEdgeRoutes: true})
	if err := state.UpdateWithWorkGuard(g, guard); err != nil {
		return false, err
	}
	rollback := &layoutgraph.Transaction{Graph: g, PriorGraphState: state}
	complete := false
	defer func() {
		if !complete {
			rollback.Rollback()
		}
	}()
	originalTL, _ := layoutgraph.Nodes(g.Containers[nil]).BoundingBox()
	h := layoutgraph.NewHierarchy()
	h.LevelCount = proxy.Nodes[0].Hierarchy.LevelCount
	for _, block := range blocks {
		if err := guard.Step(); err != nil {
			return false, err
		}
		dx := block.proxy.TopLeft.X - tl.X + originalTL.X - block.original.TopLeft.X
		dy := block.proxy.TopLeft.Y - tl.Y + originalTL.Y - block.original.TopLeft.Y
		for _, member := range block.members {
			if err := guard.Step(); err != nil {
				return false, err
			}
			member.Translate(dx, dy)
		}
		// Only the opaque block belongs to the outer hierarchy. Its descendants
		// retain their own hierarchy and rank, rather than being flattened into one row.
		if previous := block.original.Hierarchy; previous != nil {
			delete(previous.Levels(), block.original)
		}
		block.original.Hierarchy = h
		h.Levels()[block.original] = block.proxy.HierarchyLevel()
	}
	if err := guard.Finish(); err != nil {
		return false, err
	}
	complete = true
	return true, nil
}

func compoundEndpointOffset(endpoint, block *layoutgraph.Node, column *int) geo.Point {
	center := endpoint.Center()
	if column != nil {
		if port, ok := nodeshape.TableColumnPortValue(endpoint.Shape, geo.Right, *column); ok {
			center = &port
		}
	}
	return geo.Point{X: math.Max(0, math.Min(block.Width, center.X-block.TopLeft.X)), Y: math.Max(0, math.Min(block.Height, center.Y-block.TopLeft.Y))}
}

func compoundConnected(blocks []*compoundBlock, interfaces []compoundInterface) bool {
	adjacent := make(map[*compoundBlock][]*compoundBlock, len(blocks))
	for _, edge := range interfaces {
		adjacent[edge.from] = append(adjacent[edge.from], edge.to)
		adjacent[edge.to] = append(adjacent[edge.to], edge.from)
	}
	seen := map[*compoundBlock]bool{blocks[0]: true}
	queue := []*compoundBlock{blocks[0]}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		for _, next := range adjacent[block] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return len(seen) == len(blocks)
}

// orderCompoundInterfaces refines the center-based layered ordering using actual
// boundary offsets. At most two adjacent-swap sweeps are attempted. Blocks keep
// their rank and size, so accepted swaps cannot introduce node overlap.
func orderCompoundInterfaces(blocks []*compoundBlock, interfaces []compoundInterface, horizontal bool, guard *limits.WorkGuard) error {
	levels := make(map[int][]*compoundBlock)
	for _, block := range blocks {
		levels[block.proxy.HierarchyLevel()] = append(levels[block.proxy.HierarchyLevel()], block)
	}
	coordinates := func(b *compoundBlock) (*float64, float64) {
		if horizontal {
			return &b.proxy.TopLeft.Y, b.proxy.Height
		}
		return &b.proxy.TopLeft.X, b.proxy.Width
	}
	for level := 0; level < blocks[0].proxy.Hierarchy.LevelCount; level++ {
		row := levels[level]
		slices.SortFunc(row, func(a, b *compoundBlock) int {
			ax, _ := coordinates(a)
			bx, _ := coordinates(b)
			if *ax < *bx {
				return -1
			}
			if *ax > *bx {
				return 1
			}
			return 0
		})
		levels[level] = row
	}
	score, err := compoundInterfaceScore(interfaces, horizontal, guard)
	if err != nil {
		return err
	}
	for pass := 0; pass < 2; pass++ {
		improved := false
		for level := 0; level < blocks[0].proxy.Hierarchy.LevelCount; level++ {
			row := levels[level]
			for i := 0; i+1 < len(row); i++ {
				if err := guard.Step(); err != nil {
					return err
				}
				a, b := row[i], row[i+1]
				ax, aw := coordinates(a)
				bx, bw := coordinates(b)
				oldA, oldB := *ax, *bx
				*ax, *bx = oldB+bw-aw, oldA
				candidate, err := compoundInterfaceScore(interfaces, horizontal, guard)
				if err != nil {
					return err
				}
				if candidate.crossings < score.crossings || (candidate.crossings == score.crossings && candidate.span < score.span) {
					score = candidate
					row[i], row[i+1] = b, a
					improved = true
				} else {
					*ax, *bx = oldA, oldB
				}
			}
		}
		if !improved {
			break
		}
	}
	return guard.Finish()
}

type compoundScore struct {
	crossings int
	span      float64
}
type compoundSegment struct{ a, b geo.Point }

func compoundInterfaceScore(interfaces []compoundInterface, horizontal bool, guard *limits.WorkGuard) (compoundScore, error) {
	score := compoundScore{}
	segments := make([]compoundSegment, len(interfaces))
	for i, edge := range interfaces {
		if err := guard.Step(); err != nil {
			return score, err
		}
		from, to := edge.from.proxy, edge.to.proxy
		a := geo.Point{X: from.TopLeft.X + edge.fromOffset.X, Y: from.TopLeft.Y + edge.fromOffset.Y}
		b := geo.Point{X: to.TopLeft.X + edge.toOffset.X, Y: to.TopLeft.Y + edge.toOffset.Y}
		if horizontal {
			if from.Center().X < to.Center().X {
				a.X = from.TopLeft.X + from.Width
				b.X = to.TopLeft.X
			} else {
				a.X = from.TopLeft.X
				b.X = to.TopLeft.X + to.Width
			}
			score.span += math.Abs(a.Y - b.Y)
		} else {
			if from.Center().Y < to.Center().Y {
				a.Y = from.TopLeft.Y + from.Height
				b.Y = to.TopLeft.Y
			} else {
				a.Y = from.TopLeft.Y
				b.Y = to.TopLeft.Y + to.Height
			}
			score.span += math.Abs(a.X - b.X)
		}
		segments[i] = compoundSegment{a, b}
	}
	orientation := func(a, b, c geo.Point) float64 { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }
	for i, a := range segments {
		for _, b := range segments[i+1:] {
			if err := guard.Step(); err != nil {
				return score, err
			}
			if orientation(a.a, a.b, b.a)*orientation(a.a, a.b, b.b) < 0 && orientation(b.a, b.b, a.a)*orientation(b.a, b.b, a.b) < 0 {
				score.crossings++
			}
		}
	}
	return score, nil
}
