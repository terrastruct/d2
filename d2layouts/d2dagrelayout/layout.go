package d2dagrelayout

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"log/slog"

	"oss.terrastruct.com/util-go/xdefer"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/jsrunner"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/shape"
)

//go:embed setup.js
var setupJS string

const (
	MIN_RANK_SEP    = 60
	EDGE_LABEL_GAP  = 20
	DEFAULT_PADDING = 30.
	MIN_SPACING     = 10.
)

type ConfigurableOpts struct {
	NodeSep int `json:"nodesep"`
	EdgeSep int `json:"edgesep"`
}

var DefaultOpts = ConfigurableOpts{
	NodeSep: 60,
	EdgeSep: 20,
}

type DagreNode struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type DagreEdge struct {
	Points []*geo.Point `json:"points"`
}

type dagreOpts struct {
	// for a top to bottom graph: ranksep is y spacing, nodesep is x spacing, edgesep is x spacing
	ranksep int
	// graph direction: tb (top to bottom)| bt | lr | rl
	rankdir string

	ConfigurableOpts
}

func DefaultLayout(ctx context.Context, g *d2graph.Graph) (err error) {
	return Layout(ctx, g, nil)
}

func Layout(ctx context.Context, g *d2graph.Graph, opts *ConfigurableOpts) (err error) {
	if opts == nil {
		opts = &DefaultOpts
	}
	defer xdefer.Errorf(&err, "failed to dagre layout")

	debugJS := false
	runner := jsrunner.NewJSRunner()
	if _, err := runner.RunString(dagreJS); err != nil {
		return err
	}
	if _, err := runner.RunString(setupJS); err != nil {
		return err
	}

	rootAttrs := dagreOpts{
		ConfigurableOpts: ConfigurableOpts{
			EdgeSep: opts.EdgeSep,
			NodeSep: opts.NodeSep,
		},
	}
	isHorizontal := false
	switch g.Root.Direction.Value {
	case "down":
		rootAttrs.rankdir = "TB"
	case "right":
		rootAttrs.rankdir = "LR"
		isHorizontal = true
	case "left":
		rootAttrs.rankdir = "RL"
		isHorizontal = true
	case "up":
		rootAttrs.rankdir = "BT"
	default:
		rootAttrs.rankdir = "TB"
	}

	// set label and icon positions for dagre
	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	maxLabelWidth := 0
	maxLabelHeight := 0
	for _, edge := range g.Edges {
		width := edge.LabelDimensions.Width
		height := edge.LabelDimensions.Height
		maxLabelWidth = go2.Max(maxLabelWidth, width)
		maxLabelHeight = go2.Max(maxLabelHeight, height)
	}

	if !isHorizontal {
		rootAttrs.ranksep = go2.Max(100, maxLabelHeight+40)
	} else {
		rootAttrs.ranksep = go2.Max(100, maxLabelWidth+40)
		// use existing config
		// rootAttrs.NodeSep = rootAttrs.EdgeSep
		// // configure vertical padding
		// rootAttrs.EdgeSep = maxLabelHeight + 40
		// Note: non-containers have both of these as padding (rootAttrs.NodeSep + rootAttrs.EdgeSep)
	}

	configJS := setGraphAttrs(rootAttrs)
	if _, err := runner.RunString(configJS); err != nil {
		return err
	}

	mapper := NewObjectMapper()
	for _, obj := range g.Objects {
		mapper.Register(obj)
	}
	loadScript := ""
	for _, obj := range g.Objects {
		loadScript += mapper.generateAddNodeLine(obj, int(obj.Width), int(obj.Height))
		if obj.Parent != g.Root {
			loadScript += mapper.generateAddParentLine(obj, obj.Parent)
		}
	}

	for _, edge := range g.Edges {
		src, dst := getEdgeEndpoints(g, edge)

		width := edge.LabelDimensions.Width
		height := edge.LabelDimensions.Height

		numEdges := 0
		for _, e := range g.Edges {
			otherSrc, otherDst := getEdgeEndpoints(g, e)
			if (otherSrc == src && otherDst == dst) || (otherSrc == dst && otherDst == src) {
				numEdges++
			}
		}

		// We want to leave some gap between multiple edges
		if numEdges > 1 {
			switch g.Root.Direction.Value {
			case "down", "up", "":
				width += EDGE_LABEL_GAP
			case "left", "right":
				height += EDGE_LABEL_GAP
			}
		}

		loadScript += mapper.generateAddEdgeLine(src, dst, edge.AbsID(), width, height)
	}

	if debugJS {
		log.Debug(ctx, "script", slog.Any("all", setupJS+configJS+loadScript))
	}

	if _, err := runner.RunString(loadScript); err != nil {
		return err
	}

	if _, err := runner.RunString(`dagre.layout(g)`); err != nil {
		if debugJS {
			log.Warn(ctx, "layout error", slog.Any("err", err))
		}
		return err
	}

	for i := range g.Objects {
		val, err := runner.RunString(fmt.Sprintf("JSON.stringify(g.node(g.nodes()[%d]))", i))
		if err != nil {
			return err
		}
		var dn DagreNode
		if err := json.Unmarshal([]byte(val.String()), &dn); err != nil {
			return err
		}
		if debugJS {
			log.Debug(ctx, "graph", slog.Any("json", dn))
		}

		obj := mapper.ToObj(dn.ID)

		// dagre gives center of node
		obj.TopLeft = geo.NewPoint(math.Round(dn.X-dn.Width/2), math.Round(dn.Y-dn.Height/2))
		obj.Width = math.Ceil(dn.Width)
		obj.Height = math.Ceil(dn.Height)
	}

	for i, edge := range g.Edges {
		val, err := runner.RunString(fmt.Sprintf("JSON.stringify(g.edge(g.edges()[%d]))", i))
		if err != nil {
			return err
		}
		var de DagreEdge
		if err := json.Unmarshal([]byte(val.String()), &de); err != nil {
			return err
		}
		if debugJS {
			log.Debug(ctx, "graph", slog.Any("json", de))
		}

		points := make([]*geo.Point, len(de.Points))
		for i := range de.Points {
			if edge.SrcArrow && !edge.DstArrow {
				points[len(de.Points)-i-1] = de.Points[i].Copy()
			} else {
				points[i] = de.Points[i].Copy()
			}
		}

		startIndex, endIndex := 0, len(points)-1
		start, end := points[startIndex], points[endIndex]

		// chop where edge crosses the source/target boxes since container edges were routed to a descendant
		if edge.Src != edge.Dst {
			for i := 1; i < len(points); i++ {
				segment := *geo.NewSegment(points[i-1], points[i])
				if intersections := edge.Src.Box.Intersections(segment); len(intersections) > 0 {
					start = intersections[0]
					startIndex = i - 1
				}

				if intersections := edge.Dst.Box.Intersections(segment); len(intersections) > 0 {
					end = intersections[0]
					endIndex = i
					break
				}
			}
		}
		points = points[startIndex : endIndex+1]
		points[0] = start
		points[len(points)-1] = end

		edge.Route = points
	}

	adjustRankSpacing(g, float64(rootAttrs.ranksep), isHorizontal)
	adjustCrossRankSpacing(g, float64(rootAttrs.ranksep), !isHorizontal)
	fitContainerPadding(g, float64(rootAttrs.ranksep), isHorizontal)

	for _, edge := range g.Edges {
		points := edge.Route
		startIndex, endIndex := 0, len(points)-1
		start, end := points[startIndex], points[endIndex]

		// arrowheads can appear broken if segments are very short from dagre routing a point just outside the shape
		// to fix this, we try extending the previous segment into the shape instead of having a very short segment
		if startIndex+2 < len(points) {
			newStartingSegment := *geo.NewSegment(start, points[startIndex+1])
			if newStartingSegment.Length() < d2graph.MIN_SEGMENT_LEN {
				// we don't want a very short segment right next to the source because it will mess up the arrowhead
				// instead we want to extend the next segment into the shape border if possible
				nextStart := points[startIndex+1]
				nextEnd := points[startIndex+2]

				// Note: in other direction to extend towards source
				nextSegment := *geo.NewSegment(nextStart, nextEnd)
				v := nextSegment.ToVector()
				extendedStart := nextEnd.ToVector().Add(v.AddLength(d2graph.MIN_SEGMENT_LEN)).ToPoint()
				extended := *geo.NewSegment(nextEnd, extendedStart)

				if intersections := edge.Src.Box.Intersections(extended); len(intersections) > 0 {
					startIndex++
					points[startIndex] = intersections[0]
					start = points[startIndex]
				}
			}
		}
		if endIndex-2 >= 0 {
			newEndingSegment := *geo.NewSegment(end, points[endIndex-1])
			if newEndingSegment.Length() < d2graph.MIN_SEGMENT_LEN {
				// extend the prev segment into the shape border if possible
				prevStart := points[endIndex-2]
				prevEnd := points[endIndex-1]

				prevSegment := *geo.NewSegment(prevStart, prevEnd)
				v := prevSegment.ToVector()
				extendedEnd := prevStart.ToVector().Add(v.AddLength(d2graph.MIN_SEGMENT_LEN)).ToPoint()
				extended := *geo.NewSegment(prevStart, extendedEnd)

				if intersections := edge.Dst.Box.Intersections(extended); len(intersections) > 0 {
					endIndex--
					points[endIndex] = intersections[0]
					end = points[endIndex]
				}
			}
		}

		var originalSrcTL, originalDstTL *geo.Point
		// if the edge passes through 3d/multiple, use the offset box for tracing to border
		if srcDx, srcDy := edge.Src.GetModifierElementAdjustments(); srcDx != 0 || srcDy != 0 {
			if start.X > edge.Src.TopLeft.X+srcDx &&
				start.Y < edge.Src.TopLeft.Y+edge.Src.Height-srcDy {
				originalSrcTL = edge.Src.TopLeft.Copy()
				edge.Src.TopLeft.X += srcDx
				edge.Src.TopLeft.Y -= srcDy
			}
		}
		if dstDx, dstDy := edge.Dst.GetModifierElementAdjustments(); dstDx != 0 || dstDy != 0 {
			if end.X > edge.Dst.TopLeft.X+dstDx &&
				end.Y < edge.Dst.TopLeft.Y+edge.Dst.Height-dstDy {
				originalDstTL = edge.Dst.TopLeft.Copy()
				edge.Dst.TopLeft.X += dstDx
				edge.Dst.TopLeft.Y -= dstDy
			}
		}

		startIndex, endIndex = edge.TraceToShape(points, startIndex, endIndex, float64(label.PADDING))
		points = points[startIndex : endIndex+1]

		// build a curved path from the dagre route
		vectors := make([]geo.Vector, 0, len(points)-1)
		for i := 1; i < len(points); i++ {
			vectors = append(vectors, points[i-1].VectorTo(points[i]))
		}

		path := make([]*geo.Point, 0)
		path = append(path, points[0])
		if len(vectors) > 1 {
			path = append(path, points[0].AddVector(vectors[0].Multiply(.8)))
			for i := 1; i < len(vectors)-2; i++ {
				p := points[i]
				v := vectors[i]
				path = append(path, p.AddVector(v.Multiply(.2)))
				path = append(path, p.AddVector(v.Multiply(.5)))
				path = append(path, p.AddVector(v.Multiply(.8)))
			}
			path = append(path, points[len(points)-2].AddVector(vectors[len(vectors)-1].Multiply(.2)))
			edge.IsCurve = true
		}
		path = append(path, points[len(points)-1])

		edge.Route = path
		// compile needs to assign edge label positions
		if edge.Label.Value != "" {
			edge.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}

		// undo 3d/multiple offset
		if originalSrcTL != nil {
			edge.Src.TopLeft.X = originalSrcTL.X
			edge.Src.TopLeft.Y = originalSrcTL.Y
		}
		if originalDstTL != nil {
			edge.Dst.TopLeft.X = originalDstTL.X
			edge.Dst.TopLeft.Y = originalDstTL.Y
		}
	}

	return nil
}

func getEdgeEndpoints(g *d2graph.Graph, edge *d2graph.Edge) (*d2graph.Object, *d2graph.Object) {
	// dagre doesn't work with edges to containers so we connect container edges to their first child instead (going all the way down)
	// we will chop the edge where it intersects the container border so it only shows the edge from the container
	src := edge.Src
	for len(src.Children) > 0 && src.Class == nil && src.SQLTable == nil {
		// We want to get the bottom node of sources, setting its rank higher than all children
		src = getLongestEdgeChainTail(g, src)
	}
	dst := edge.Dst
	for len(dst.Children) > 0 && dst.Class == nil && dst.SQLTable == nil {
		dst = getLongestEdgeChainHead(g, dst)
	}
	if edge.SrcArrow && !edge.DstArrow {
		// for `b <- a`, edge.Edge is `a -> b` and we expect this routing result
		src, dst = dst, src
	}
	return src, dst
}

func setGraphAttrs(attrs dagreOpts) string {
	return fmt.Sprintf(`g.setGraph({
  ranksep: %d,
  edgesep: %d,
  nodesep: %d,
  rankdir: "%s",
});
`,
		attrs.ranksep,
		attrs.ConfigurableOpts.EdgeSep,
		attrs.ConfigurableOpts.NodeSep,
		attrs.rankdir,
	)
}

// getLongestEdgeChainHead finds the longest chain in a container and gets its head
// If there are multiple chains of the same length, get the head closest to the center
func getLongestEdgeChainHead(g *d2graph.Graph, container *d2graph.Object) *d2graph.Object {
	rank := make(map[*d2graph.Object]int)
	chainLength := make(map[*d2graph.Object]int)

	for _, obj := range container.ChildrenArray {
		isHead := true
		for _, e := range g.Edges {
			if inContainer(e.Src, container) != nil && inContainer(e.Dst, obj) != nil {
				isHead = false
				break
			}
		}
		if !isHead {
			continue
		}
		rank[obj] = 1
		chainLength[obj] = 1
		// BFS
		queue := []*d2graph.Object{obj}
		visited := make(map[*d2graph.Object]struct{})
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			if _, ok := visited[curr]; ok {
				continue
			}
			visited[curr] = struct{}{}
			for _, e := range g.Edges {
				child := inContainer(e.Dst, container)
				if child == curr {
					continue
				}
				if child != nil && inContainer(e.Src, curr) != nil {
					if rank[curr]+1 > rank[child] {
						rank[child] = rank[curr] + 1
						chainLength[obj] = go2.Max(chainLength[obj], rank[child])
					}
					queue = append(queue, child)
				}
			}
		}
	}
	max := int(math.MinInt32)
	for _, obj := range container.ChildrenArray {
		if chainLength[obj] > max {
			max = chainLength[obj]
		}
	}

	var heads []*d2graph.Object
	for i, obj := range container.ChildrenArray {
		if rank[obj] == 1 && chainLength[obj] == max {
			heads = append(heads, container.ChildrenArray[i])
		}
	}

	if len(heads) > 0 {
		return heads[int(math.Floor(float64(len(heads))/2.0))]
	}
	return container.ChildrenArray[0]
}

// getLongestEdgeChainTail gets the node at the end of the longest edge chain, because that will be the end of the container
// and is what external connections should connect with.
// If there are multiple of same length, get the one closest to the middle
func getLongestEdgeChainTail(g *d2graph.Graph, container *d2graph.Object) *d2graph.Object {
	rank := make(map[*d2graph.Object]int)

	for _, obj := range container.ChildrenArray {
		isHead := true
		for _, e := range g.Edges {
			if inContainer(e.Src, container) != nil && inContainer(e.Dst, obj) != nil {
				isHead = false
				break
			}
		}
		if !isHead {
			continue
		}
		rank[obj] = 1
		// BFS
		queue := []*d2graph.Object{obj}
		visited := make(map[*d2graph.Object]struct{})
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			if _, ok := visited[curr]; ok {
				continue
			}
			visited[curr] = struct{}{}
			for _, e := range g.Edges {
				child := inContainer(e.Dst, container)
				if child == curr {
					continue
				}
				if child != nil && inContainer(e.Src, curr) != nil {
					rank[child] = go2.Max(rank[child], rank[curr]+1)
					queue = append(queue, child)
				}
			}
		}
	}
	max := int(math.MinInt32)
	for _, obj := range container.ChildrenArray {
		if rank[obj] > max {
			max = rank[obj]
		}
	}

	var tails []*d2graph.Object
	for i, obj := range container.ChildrenArray {
		if rank[obj] == max {
			tails = append(tails, container.ChildrenArray[i])
		}
	}

	return tails[int(math.Floor(float64(len(tails))/2.0))]
}

func inContainer(obj, container *d2graph.Object) *d2graph.Object {
	if obj == nil {
		return nil
	}
	if obj == container {
		return obj
	}
	if obj.Parent == container {
		return obj
	}
	return inContainer(obj.Parent, container)
}

func positionLabelsIcons(obj *d2graph.Object) {
	if obj.Icon != nil && obj.IconPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
			if obj.LabelPosition == nil {
				obj.LabelPosition = go2.Pointer(label.OutsideTopRight.String())
				return
			}
		} else if obj.SQLTable != nil || obj.Class != nil || obj.Language != "" {
			obj.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
		} else {
			obj.IconPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}
	if obj.HasLabel() && obj.LabelPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
		} else if obj.HasOutsideBottomLabel() {
			obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
		} else if obj.Icon != nil {
			obj.LabelPosition = go2.Pointer(label.InsideTopCenter.String())
		} else {
			obj.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
		if float64(obj.LabelDimensions.Width) > obj.Width || float64(obj.LabelDimensions.Height) > obj.Height {
			if len(obj.ChildrenArray) > 0 {
				obj.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
			} else {
				obj.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
			}
		}
	}
}

func getRanks(g *d2graph.Graph, isHorizontal bool) (ranks [][]*d2graph.Object, objectRanks, startingParentRanks, endingParentRanks map[*d2graph.Object]int) {
	alignedObjects := make(map[float64][]*d2graph.Object)
	for _, obj := range g.Objects {
		if !obj.IsContainer() {
			if !isHorizontal {
				y := math.Ceil(obj.TopLeft.Y + obj.Height/2)
				alignedObjects[y] = append(alignedObjects[y], obj)
			} else {
				x := math.Ceil(obj.TopLeft.X + obj.Width/2)
				alignedObjects[x] = append(alignedObjects[x], obj)
			}
		}
	}

	levels := make([]float64, 0, len(alignedObjects))
	for l := range alignedObjects {
		levels = append(levels, l)
	}
	sort.Slice(levels, func(i, j int) bool {
		return levels[i] < levels[j]
	})

	ranks = make([][]*d2graph.Object, 0, len(levels))
	objectRanks = make(map[*d2graph.Object]int)
	for i, l := range levels {
		for _, obj := range alignedObjects[l] {
			objectRanks[obj] = i
		}
		ranks = append(ranks, alignedObjects[l])
	}

	startingParentRanks = make(map[*d2graph.Object]int)
	endingParentRanks = make(map[*d2graph.Object]int)
	for _, obj := range g.Objects {
		if obj.IsContainer() {
			continue
		}
		r := objectRanks[obj]
		// update all ancestor's min/max ranks
		for parent := obj.Parent; parent != nil && parent != g.Root; parent = parent.Parent {
			if start, has := startingParentRanks[parent]; !has || r < start {
				startingParentRanks[parent] = r
			}
			if end, has := endingParentRanks[parent]; !has || r > end {
				endingParentRanks[parent] = r
			}
		}
	}

	return ranks, objectRanks, startingParentRanks, endingParentRanks
}

// shift everything down by distance if it is at or below start position
func shiftDown(g *d2graph.Graph, start, distance float64, isHorizontal bool) {
	if isHorizontal {
		for _, edge := range g.Edges {
			first, last := edge.Route[0], edge.Route[len(edge.Route)-1]
			if start <= first.X {
				onStaticSrc := first.X == edge.Src.TopLeft.X+edge.Src.Width && edge.Src.TopLeft.X < start
				if !onStaticSrc {
					// src is not shifting and we are on src so don't shift
					first.X += distance
				}
			}
			if start <= last.X {
				onStaticDst := last.X == edge.Dst.TopLeft.X+edge.Dst.Width && edge.Dst.TopLeft.X < start
				if !onStaticDst {
					last.X += distance
				}
			}
			for i := 1; i < len(edge.Route)-1; i++ {
				p := edge.Route[i]
				if p.X < start {
					continue
				}
				p.X += distance
			}
		}
		for _, obj := range g.Objects {
			if obj.TopLeft.X < start {
				continue
			}
			obj.TopLeft.X += distance
		}
	} else {
		for _, edge := range g.Edges {
			first, last := edge.Route[0], edge.Route[len(edge.Route)-1]
			if start <= first.Y {
				onStaticSrc := first.Y == edge.Src.TopLeft.Y+edge.Src.Height && edge.Src.TopLeft.Y < start
				if !onStaticSrc {
					// src is not shifting and we are on src so don't shift
					first.Y += distance
				}
			}
			if start <= last.Y {
				onStaticDst := last.Y == edge.Dst.TopLeft.Y+edge.Dst.Height && edge.Dst.TopLeft.Y < start
				if !onStaticDst {
					last.Y += distance
				}
			}
			for i := 1; i < len(edge.Route)-1; i++ {
				p := edge.Route[i]
				if p.Y < start {
					continue
				}
				p.Y += distance
			}
		}
		for _, obj := range g.Objects {
			if obj.TopLeft.Y < start {
				continue
			}
			obj.TopLeft.Y += distance
		}
	}
}

func shiftUp(g *d2graph.Graph, start, distance float64, isHorizontal bool) {
	if isHorizontal {
		for _, edge := range g.Edges {
			first, last := edge.Route[0], edge.Route[len(edge.Route)-1]
			if first.X <= start {
				onStaticSrc := first.X == edge.Src.TopLeft.X && start < edge.Src.TopLeft.X+edge.Src.Width
				if !onStaticSrc {
					// src is not shifting and we are on src so don't shift
					first.X -= distance
				}
			}
			if last.X <= start {
				onStaticDst := last.X == edge.Dst.TopLeft.X && start < edge.Dst.TopLeft.X+edge.Dst.Width
				if !onStaticDst {
					last.X -= distance
				}
			}
			for i := 1; i < len(edge.Route)-1; i++ {
				p := edge.Route[i]
				if start < p.X {
					continue
				}
				p.X -= distance
			}
		}
		for _, obj := range g.Objects {
			if start < obj.TopLeft.X {
				continue
			}
			obj.TopLeft.X -= distance
		}
	} else {
		for _, edge := range g.Edges {
			first, last := edge.Route[0], edge.Route[len(edge.Route)-1]
			if first.Y <= start {
				// don't shift first edge point if src is not shifting and we are on src
				onStaticSrc := first.Y == edge.Src.TopLeft.Y && start < edge.Src.TopLeft.Y+edge.Src.Height
				if !onStaticSrc {
					first.Y -= distance
				}
			}
			if last.Y <= start {
				onStaticDst := last.Y == edge.Dst.TopLeft.Y && start < edge.Dst.TopLeft.Y
				if !onStaticDst {
					last.Y -= distance
				}
			}
			for i := 1; i < len(edge.Route)-1; i++ {
				p := edge.Route[i]
				// for _, p := range edge.Route {
				if start < p.Y {
					continue
				}
				p.Y -= distance
			}
		}
		for _, obj := range g.Objects {
			if start < obj.TopLeft.Y {
				continue
			}
			obj.TopLeft.Y -= distance
		}
	}
}

// shift down everything that is below start
// shift all nodes that are reachable via an edge or being directly below a shifting node or expanding container
// expand containers to wrap shifted nodes
func shiftReachableDown(g *d2graph.Graph, obj *d2graph.Object, start, distance float64, isHorizontal, isMargin bool) map[*d2graph.Object]struct{} {
	q := []*d2graph.Object{obj}

	needsMove := make(map[*d2graph.Object]struct{})
	seen := make(map[*d2graph.Object]struct{})
	shifted := make(map[*d2graph.Object]struct{})
	shiftedEdges := make(map[*d2graph.Edge]struct{})
	queue := func(o *d2graph.Object) {
		if _, in := seen[o]; in {
			return
		}
		q = append(q, o)
	}

	// if object below is within this distance after shifting, also shift it
	threshold := 100.
	checkBelow := func(curr *d2graph.Object) {
		currBottom := curr.TopLeft.Y + curr.Height
		currRight := curr.TopLeft.X + curr.Width
		if isHorizontal {
			originalRight := currRight
			if _, in := shifted[curr]; in {
				originalRight -= distance
			}
			for _, other := range g.Objects {
				if other == curr || curr.IsDescendantOf(other) {
					continue
				}
				if originalRight < other.TopLeft.X &&
					other.TopLeft.X < originalRight+distance+threshold &&
					curr.TopLeft.Y < other.TopLeft.Y+other.Height &&
					other.TopLeft.Y < currBottom {
					queue(other)
				}
			}
		} else {
			originalBottom := currBottom
			if _, in := shifted[curr]; in {
				originalBottom -= distance
			}
			for _, other := range g.Objects {
				if other == curr || curr.IsDescendantOf(other) {
					continue
				}
				if originalBottom < other.TopLeft.Y &&
					other.TopLeft.Y < originalBottom+distance+threshold &&
					curr.TopLeft.X < other.TopLeft.X+other.Width &&
					other.TopLeft.X < currRight {
					queue(other)
				}
			}
		}
	}

	processQueue := func() {
		for len(q) > 0 {
			curr := q[0]
			q = q[1:]
			if _, was := seen[curr]; was {
				continue
			}
			// skip other objects behind start
			if curr != obj {
				if _, in := needsMove[curr]; !in {
					if isHorizontal {
						if curr.TopLeft.X < start {
							continue
						}
					} else {
						if curr.TopLeft.Y < start {
							continue
						}
					}
				}
			}

			if isHorizontal {
				_, shift := needsMove[curr]
				if !shift {
					if !isMargin {
						shift = start < curr.TopLeft.X
					} else {
						shift = start <= curr.TopLeft.X
					}
				}

				if shift {
					curr.TopLeft.X += distance
					shifted[curr] = struct{}{}
				}
			} else {
				_, shift := needsMove[curr]
				if !shift {
					if !isMargin {
						shift = start < curr.TopLeft.Y
					} else {
						shift = start <= curr.TopLeft.Y
					}
				}
				if shift {
					curr.TopLeft.Y += distance
					shifted[curr] = struct{}{}
				}
			}
			seen[curr] = struct{}{}

			if curr.Parent != g.Root && !curr.IsDescendantOf(obj) {
				queue(curr.Parent)
			}

			for _, child := range curr.ChildrenArray {
				queue(child)
			}

			for _, e := range g.Edges {
				if _, in := shiftedEdges[e]; in {
					continue
				}
				if e.Src == curr && e.Dst == curr {
					if isHorizontal {
						for _, p := range e.Route {
							p.X += distance
						}
					} else {
						for _, p := range e.Route {
							p.Y += distance
						}
					}
					shiftedEdges[e] = struct{}{}
					continue
				} else if e.Src == curr {
					last := e.Route[len(e.Route)-1]
					if isHorizontal {
						if start <= last.X &&
							e.Dst.TopLeft.X+e.Dst.Width < last.X+distance {
							needsMove[e.Dst] = struct{}{}
						}
					} else {
						if start <= last.Y &&
							e.Dst.TopLeft.Y+e.Dst.Height < last.Y+distance {
							needsMove[e.Dst] = struct{}{}
						}
					}
					queue(e.Dst)
					first := e.Route[0]
					startIndex := 0
					_, wasShifted := shifted[curr]
					if isHorizontal {
						if wasShifted && first.X < curr.TopLeft.X && first.X < start {
							first.X += distance
							startIndex++
						}
						for i := startIndex; i < len(e.Route); i++ {
							p := e.Route[i]
							if start <= p.X {
								p.X += distance
							}
						}
					} else {
						if wasShifted && first.Y < curr.TopLeft.Y && first.Y < start {
							first.Y += distance
							startIndex++
						}
						for i := startIndex; i < len(e.Route); i++ {
							p := e.Route[i]
							if start <= p.Y {
								p.Y += distance
							}
						}
					}
					shiftedEdges[e] = struct{}{}
				} else if e.Dst == curr {
					first := e.Route[0]
					if isHorizontal {
						if start <= first.X &&
							e.Src.TopLeft.X+e.Src.Width < first.X+distance {
							needsMove[e.Src] = struct{}{}
						}
					} else {
						if start <= first.Y &&
							e.Src.TopLeft.Y+e.Src.Height < first.Y+distance {
							needsMove[e.Src] = struct{}{}
						}
					}
					queue(e.Src)
					last := e.Route[len(e.Route)-1]
					endIndex := len(e.Route)
					_, wasShifted := shifted[curr]
					if isHorizontal {
						if wasShifted && last.X < curr.TopLeft.X && last.X < start {
							last.X += distance
							endIndex--
						}
						for i := 0; i < endIndex; i++ {
							p := e.Route[i]
							if start <= p.X {
								p.X += distance
							}
						}
					} else {
						if wasShifted && last.Y < curr.TopLeft.Y && last.Y < start {
							last.Y += distance
							endIndex--
						}
						for i := 0; i < endIndex; i++ {
							p := e.Route[i]
							if start <= p.Y {
								p.Y += distance
							}
						}
					}
					shiftedEdges[e] = struct{}{}
				}
			}

			// check for nodes below that need to move from the shift
			checkBelow(curr)
		}
	}

	processQueue()

	grown := make(map[*d2graph.Object]struct{})
	for o := range seen {
		if o.Parent == g.Root {
			continue
		}
		if _, in := shifted[o.Parent]; in {
			continue
		}
		if _, in := grown[o.Parent]; in {
			continue
		}

		for parent := o.Parent; parent != g.Root; parent = parent.Parent {
			if _, in := shifted[parent]; in {
				break
			}
			if _, in := grown[parent]; in {
				break
			}

			if isHorizontal {
				if parent.TopLeft.X < start {
					parent.Width += distance
					grown[parent] = struct{}{}

					checkBelow(parent)
					processQueue()
				}
			} else {
				if parent.TopLeft.Y < start {
					parent.Height += distance
					grown[parent] = struct{}{}

					checkBelow(parent)
					processQueue()
				}
			}
		}
	}

	increasedMargins := make(map[*d2graph.Object]struct{})
	movedObjects := make([]*d2graph.Object, 0, len(shifted))
	for obj := range shifted {
		movedObjects = append(movedObjects, obj)
	}
	for obj := range grown {
		movedObjects = append(movedObjects, obj)
	}
	for _, moved := range movedObjects {
		counts := true
		// check if any other shifted is directly above
		for _, other := range movedObjects {
			if other == moved {
				continue
			}
			if isHorizontal {
				if other.TopLeft.Y+other.Height < moved.TopLeft.Y ||
					moved.TopLeft.Y+moved.Height < other.TopLeft.Y {
					// doesn't line up vertically
					continue
				}

				// above and within threshold
				if other.TopLeft.X < moved.TopLeft.X &&
					moved.TopLeft.X < other.TopLeft.X+other.Width+threshold {
					counts = false
					break
				}
			} else {
				if other.TopLeft.X+other.Width < moved.TopLeft.X ||
					moved.TopLeft.X+moved.Width < other.TopLeft.X {
					// doesn't line up horizontally
					continue
				}

				// above and within threshold
				if other.TopLeft.Y < moved.TopLeft.Y &&
					moved.TopLeft.Y < other.TopLeft.Y+other.Height+threshold {
					counts = false
					break
				}
			}
		}
		if counts {
			increasedMargins[moved] = struct{}{}
		}
	}

	return increasedMargins
}

func adjustRankSpacing(g *d2graph.Graph, rankSep float64, isHorizontal bool) {
	ranks, objectRanks, startingParentRanks, endingParentRanks := getRanks(g, isHorizontal)

	// shifting bottom rank down first, then moving up to next rank
	for rank := len(ranks) - 1; rank >= 0; rank-- {
		var startingParents []*d2graph.Object
		var endingParents []*d2graph.Object
		for _, obj := range ranks[rank] {
			if obj.Parent == g.Root {
				continue
			}
			if r, has := endingParentRanks[obj.Parent]; has && r == rank {
				endingParents = append(endingParents, obj.Parent)
			}
			if r, has := startingParentRanks[obj.Parent]; has && r == rank {
				startingParents = append(startingParents, obj.Parent)
			}
		}

		startingAncestorPositions := make(map[*d2graph.Object]float64)
		for len(startingParents) > 0 {
			var ancestors []*d2graph.Object
			for _, parent := range startingParents {
				_, padding := parent.Spacing()
				if _, has := startingAncestorPositions[parent]; !has {
					startingAncestorPositions[parent] = math.Inf(1)
				}
				var startPosition float64
				if isHorizontal {
					paddingIncrease := math.Max(0, padding.Left-rankSep/2)
					startPosition = parent.TopLeft.X - paddingIncrease
				} else {
					paddingIncrease := math.Max(0, padding.Top-rankSep/2)
					startPosition = parent.TopLeft.Y - paddingIncrease
				}
				startingAncestorPositions[parent] = math.Min(startingAncestorPositions[parent], startPosition)
				for _, child := range parent.ChildrenArray {
					if r, has := objectRanks[child]; has {
						if r != rank {
							continue
						}
					} else {
						if startingParentRanks[child] != rank {
							continue
						}
					}
					margin, _ := child.Spacing()
					if isHorizontal {
						startPosition = child.TopLeft.X - margin.Left - padding.Left
					} else {
						startPosition = child.TopLeft.Y - margin.Top - padding.Top
					}
					startingAncestorPositions[parent] = math.Min(startingAncestorPositions[parent], startPosition)
				}
				if parent.Parent != g.Root {
					ancestors = append(ancestors, parent.Parent)
				}
			}
			startingParents = ancestors
		}

		endingAncestorPositions := make(map[*d2graph.Object]float64)
		for len(endingParents) > 0 {
			var ancestors []*d2graph.Object
			for _, parent := range endingParents {
				_, padding := parent.Spacing()
				if _, has := endingAncestorPositions[parent]; !has {
					endingAncestorPositions[parent] = math.Inf(-1)
				}
				var endPosition float64
				if isHorizontal {
					endPosition = parent.TopLeft.X + parent.Width + padding.Right - rankSep/2.
				} else {
					endPosition = parent.TopLeft.Y + parent.Height + padding.Bottom - rankSep/2.
				}

				endingAncestorPositions[parent] = math.Max(endingAncestorPositions[parent], endPosition)
				for _, child := range parent.ChildrenArray {
					if r, has := objectRanks[child]; has {
						if r != rank {
							continue
						}
					} else {
						if endingParentRanks[child] != rank {
							continue
						}
					}
					margin, _ := child.Spacing()

					if isHorizontal {
						endPosition = child.TopLeft.X + child.Width + margin.Right + padding.Right
					} else {
						endPosition = child.TopLeft.Y + child.Height + margin.Bottom + padding.Bottom
					}
					endingAncestorPositions[parent] = math.Max(endingAncestorPositions[parent], endPosition)
				}
				if parent.Parent != g.Root {
					ancestors = append(ancestors, parent.Parent)
				}
			}
			endingParents = ancestors
		}

		startingAdjustmentOrder := make([]*d2graph.Object, 0, len(startingAncestorPositions))
		for ancestor := range startingAncestorPositions {
			startingAdjustmentOrder = append(startingAdjustmentOrder, ancestor)
		}
		// adjust starting ancestors top-down
		sort.Slice(startingAdjustmentOrder, func(i, j int) bool {
			iPos := startingAncestorPositions[startingAdjustmentOrder[i]]
			jPos := startingAncestorPositions[startingAdjustmentOrder[j]]
			return iPos < jPos
		})

		endingAdjustmentOrder := make([]*d2graph.Object, 0, len(endingAncestorPositions))
		for ancestor := range endingAncestorPositions {
			endingAdjustmentOrder = append(endingAdjustmentOrder, ancestor)
		}

		// adjust ending ancestors bottom-up
		sort.Slice(endingAdjustmentOrder, func(i, j int) bool {
			iPos := endingAncestorPositions[endingAdjustmentOrder[i]]
			jPos := endingAncestorPositions[endingAdjustmentOrder[j]]
			return jPos < iPos
		})

		for _, ancestor := range endingAdjustmentOrder {
			var position float64
			if isHorizontal {
				position = ancestor.TopLeft.X + ancestor.Width
			} else {
				position = ancestor.TopLeft.Y + ancestor.Height
			}
			endDelta := endingAncestorPositions[ancestor] - position
			if endDelta > 0 {
				for _, obj := range g.Objects {
					if !obj.IsContainer() {
						continue
					}
					start := startingParentRanks[obj]
					end := endingParentRanks[obj]
					if start <= rank && rank <= end {
						if isHorizontal && position <= obj.TopLeft.X+obj.Width {
							obj.Width += endDelta
						} else if !isHorizontal &&
							position <= obj.TopLeft.Y+obj.Height {
							obj.Height += endDelta
						}
					}
				}
				shiftDown(g, position, endDelta, isHorizontal)
			}
		}

		for _, ancestor := range startingAdjustmentOrder {
			var position float64
			if isHorizontal {
				position = ancestor.TopLeft.X
			} else {
				position = ancestor.TopLeft.Y
			}
			startDelta := position - startingAncestorPositions[ancestor]
			if startDelta > 0 {
				for _, obj := range g.Objects {
					if !obj.IsContainer() {
						continue
					}
					start := startingParentRanks[obj]
					end := endingParentRanks[obj]
					if start <= rank && rank <= end {
						if isHorizontal && obj.TopLeft.X <= position {
							obj.Width += startDelta
						} else if !isHorizontal && obj.TopLeft.Y <= position {
							obj.Height += startDelta
						}
					}
				}
				shiftUp(g, position, startDelta, isHorizontal)
			}
		}
	}
}

func adjustCrossRankSpacing(g *d2graph.Graph, rankSep float64, isHorizontal bool) {
	var prevMarginTop, prevMarginBottom, prevMarginLeft, prevMarginRight map[*d2graph.Object]float64
	if isHorizontal {
		prevMarginLeft = make(map[*d2graph.Object]float64)
		prevMarginRight = make(map[*d2graph.Object]float64)
	} else {
		prevMarginTop = make(map[*d2graph.Object]float64)
		prevMarginBottom = make(map[*d2graph.Object]float64)
	}
	for _, obj := range g.Objects {
		if obj.IsGridDiagram() {
			continue
		}
		margin, padding := obj.Spacing()
		if !isHorizontal {
			if prevShift, has := prevMarginBottom[obj]; has {
				margin.Bottom -= prevShift
			}
			if margin.Bottom > 0 {
				increased := shiftReachableDown(g, obj, obj.TopLeft.Y+obj.Height, margin.Bottom, isHorizontal, true)
				for o := range increased {
					prevMarginBottom[o] = math.Max(prevMarginBottom[o], margin.Bottom)
				}
			}
			if padding.Bottom > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.Y+obj.Height, padding.Bottom, isHorizontal, false)
				obj.Height += padding.Bottom
			}
			if prevShift, has := prevMarginTop[obj]; has {
				margin.Top -= prevShift
			}
			if margin.Top > 0 {
				increased := shiftReachableDown(g, obj, obj.TopLeft.Y, margin.Top, isHorizontal, true)
				for o := range increased {
					prevMarginTop[o] = math.Max(prevMarginTop[o], margin.Top)
				}
			}
			if padding.Top > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.Y, padding.Top, isHorizontal, false)
				obj.Height += padding.Top
			}
		} else {
			if prevShift, has := prevMarginRight[obj]; has {
				margin.Right -= prevShift
			}
			if margin.Right > 0 {
				increased := shiftReachableDown(g, obj, obj.TopLeft.X+obj.Width, margin.Right, isHorizontal, true)
				for o := range increased {
					prevMarginRight[o] = math.Max(prevMarginRight[o], margin.Right)
				}
			}
			if padding.Right > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.X+obj.Width, padding.Right, isHorizontal, false)
				obj.Width += padding.Right
			}
			if prevShift, has := prevMarginLeft[obj]; has {
				margin.Left -= prevShift
			}
			if margin.Left > 0 {
				increased := shiftReachableDown(g, obj, obj.TopLeft.X, margin.Left, isHorizontal, true)
				for o := range increased {
					prevMarginLeft[o] = math.Max(prevMarginLeft[o], margin.Left)
				}
			}
			if padding.Left > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.X, padding.Left, isHorizontal, false)
				obj.Width += padding.Left
			}
		}
	}
}

func fitContainerPadding(g *d2graph.Graph, rankSep float64, isHorizontal bool) {
	for _, obj := range g.Root.ChildrenArray {
		fitPadding(obj)
	}
}

func fitPadding(obj *d2graph.Object) {
	dslShape := strings.ToLower(obj.Shape.Value)
	shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[dslShape]
	// Note: there's no shape-specific padding/placement in dagre yet
	if !obj.IsContainer() || shapeType != shape.SQUARE_TYPE {
		return
	}
	for _, child := range obj.ChildrenArray {
		fitPadding(child)
	}

	// we will compute a perfectly fit innerBox merging our padding with children's margin,
	// but we need to add padding and margin together if an outside child label will overlap with our inside label
	_, padding := obj.Spacing()
	padding.Top = math.Max(padding.Top, DEFAULT_PADDING)
	padding.Bottom = math.Max(padding.Bottom, DEFAULT_PADDING)
	padding.Left = math.Max(padding.Left, DEFAULT_PADDING)
	padding.Right = math.Max(padding.Right, DEFAULT_PADDING)

	// where we are (current*) vs where we want to fit each side to (inner*)
	currentTop := obj.TopLeft.Y
	currentBottom := obj.TopLeft.Y + obj.Height
	currentLeft := obj.TopLeft.X
	currentRight := obj.TopLeft.X + obj.Width

	innerTop := math.Inf(1)
	innerBottom := math.Inf(-1)
	innerLeft := math.Inf(1)
	innerRight := math.Inf(-1)

	// we create boxes for our inside label and icon, and will check against overlaps with any internal boxes
	var labelPosition, iconPosition label.Position
	var labelBox, iconBox *geo.Box
	if obj.HasLabel() && obj.LabelPosition != nil {
		labelPosition = label.FromString(*obj.LabelPosition)
		switch labelPosition {
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight,
			label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight,
			label.InsideMiddleLeft, label.InsideMiddleRight:
			labelTL := obj.GetLabelTopLeft()
			if labelTL != nil {
				labelBox = geo.NewBox(labelTL, float64(obj.LabelDimensions.Width)+2*label.PADDING, float64(obj.LabelDimensions.Height))
			}
		}
	}
	if obj.HasIcon() && obj.IconPosition != nil {
		iconPosition = label.FromString(*obj.IconPosition)
		switch iconPosition {
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight,
			label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight,
			label.InsideMiddleLeft, label.InsideMiddleRight:
			iconTL := obj.GetIconTopLeft()
			if iconTL != nil {
				iconBox = geo.NewBox(iconTL, d2target.MAX_ICON_SIZE, d2target.MAX_ICON_SIZE)
			}
		}
	}

	// update the inner positions for children's margin and collect the outside boxes that we cannot overlap with
	var innerBoxes []geo.Box
	for _, child := range obj.ChildrenArray {
		margin, _ := child.Spacing()
		dx, dy := child.GetModifierElementAdjustments()

		if labelBox != nil || iconBox != nil {
			var childLabelBox *geo.Box
			var childLabelPosition, childIconPosition label.Position
			if child.HasLabel() && child.LabelPosition != nil {
				childLabelPosition = label.FromString(*child.LabelPosition)
				if childLabelPosition.IsOutside() {
					childLabelTL := child.GetLabelTopLeft()

					childLabelBox = geo.NewBox(
						childLabelTL,
						float64(child.LabelDimensions.Width),
						float64(child.LabelDimensions.Height),
					)
					innerBoxes = append(innerBoxes, *childLabelBox)
				}
			}
			if child.HasIcon() && child.IconPosition != nil {
				childIconPosition = label.FromString(*child.IconPosition)
				if childIconPosition.IsOutside() {
					childIconTL := child.GetIconTopLeft()

					childIconBox := geo.NewBox(childIconTL, d2target.MAX_ICON_SIZE, d2target.MAX_ICON_SIZE)
					innerBoxes = append(innerBoxes, *childIconBox)
				}
			}
		}

		innerTop = math.Min(innerTop, child.TopLeft.Y-dy-math.Max(margin.Top, padding.Top))
		innerBottom = math.Max(innerBottom, child.TopLeft.Y+child.Height+math.Max(margin.Bottom, padding.Bottom))
		innerLeft = math.Min(innerLeft, child.TopLeft.X-math.Max(margin.Left, padding.Left))
		innerRight = math.Max(innerRight, child.TopLeft.X+child.Width+dx+math.Max(margin.Right, padding.Right))
	}

	// collect edge label boxes and update inner box for internal edges
	for _, edge := range obj.Graph.Edges {
		if !edge.Src.IsDescendantOf(obj) || !edge.Dst.IsDescendantOf(obj) {
			continue
		}
		// check internal edge + their labels
		if edge.Label.Value != "" {
			labelPosition := label.InsideMiddleCenter
			if edge.LabelPosition != nil {
				labelPosition = label.FromString(*edge.LabelPosition)
			}
			labelWidth := float64(edge.LabelDimensions.Width)
			labelHeight := float64(edge.LabelDimensions.Height)
			point, _ := labelPosition.GetPointOnRoute(edge.Route, 2, 0, labelWidth, labelHeight)

			if labelBox != nil || iconBox != nil {
				innerBoxes = append(innerBoxes, geo.Box{TopLeft: point, Width: labelWidth, Height: labelHeight})
			}

			innerTop = math.Min(innerTop, point.Y-padding.Top)
			innerBottom = math.Max(innerBottom, point.Y+labelHeight+padding.Bottom)
			innerLeft = math.Min(innerLeft, point.X-padding.Left)
			innerRight = math.Max(innerRight, point.X+labelWidth+padding.Right)
		}
		for _, point := range edge.Route {
			innerTop = math.Min(innerTop, point.Y-padding.Top)
			innerBottom = math.Max(innerBottom, point.Y+padding.Bottom)
			innerLeft = math.Min(innerLeft, point.X-padding.Left)
			innerRight = math.Max(innerRight, point.X+padding.Right)
		}
	}

	// how much do we need to shrink each side
	topDelta := innerTop - currentTop
	bottomDelta := currentBottom - innerBottom
	leftDelta := innerLeft - currentLeft
	rightDelta := currentRight - innerRight

	if topDelta > 0 || bottomDelta > 0 || leftDelta > 0 || rightDelta > 0 {
		var leftOverlap, rightOverlap, topOverlap, bottomOverlap float64
		var labelSide, iconSide geo.Orientation
		if labelBox != nil {
			switch labelPosition {
			case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
				labelSide = geo.Top
			case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
				labelSide = geo.Bottom
			case label.InsideMiddleLeft:
				labelSide = geo.Left
			case label.InsideMiddleRight:
				labelSide = geo.Right
			default:
				labelSide = geo.NONE
			}
			// move labelBox to its position with the merged delta and check for overlaps
			switch labelSide {
			case geo.Top:
				if topDelta > 0 {
					labelBox.TopLeft.Y += topDelta
				}
			case geo.Bottom:
				if bottomDelta > 0 {
					labelBox.TopLeft.Y -= bottomDelta
				}
			case geo.Left:
				if leftDelta > 0 {
					labelBox.TopLeft.X += leftDelta
				}
			case geo.Right:
				if rightDelta > 0 {
					labelBox.TopLeft.X -= rightDelta
				}
			}
			switch labelSide {
			case geo.Top:
				if topDelta > 0 {
					for _, box := range innerBoxes {
						if labelBox.Overlaps(box) {
							dy := labelBox.TopLeft.Y + labelBox.Height - box.TopLeft.Y
							topOverlap = go2.Max(topOverlap, dy)
						}
					}
				}
			case geo.Bottom:
				if bottomDelta > 0 {
					for _, box := range innerBoxes {
						if labelBox.Overlaps(box) {
							dy := box.TopLeft.Y + box.Height - labelBox.TopLeft.Y
							bottomOverlap = go2.Max(bottomOverlap, dy)
						}
					}
				}
			case geo.Left:
				if leftDelta > 0 {
					for _, box := range innerBoxes {
						if labelBox.Overlaps(box) {
							dx := labelBox.TopLeft.X + labelBox.Width - box.TopLeft.X
							leftOverlap = go2.Max(leftOverlap, dx)
						}
					}
				}
			case geo.Right:
				if rightDelta > 0 {
					for _, box := range innerBoxes {
						if labelBox.Overlaps(box) {
							dx := box.TopLeft.X + box.Width - labelBox.TopLeft.X
							rightOverlap = go2.Max(rightOverlap, dx)
						}
					}
				}
			}
		}
		if iconBox != nil {
			switch iconPosition {
			case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
				iconSide = geo.Top
			case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
				iconSide = geo.Bottom
			case label.InsideMiddleLeft:
				iconSide = geo.Left
			case label.InsideMiddleRight:
				iconSide = geo.Right
			default:
				iconSide = geo.NONE
			}
			// move iconBox to its position with the merged delta and check for overlaps
			switch iconSide {
			case geo.Top:
				if topDelta > 0 {
					iconBox.TopLeft.Y += topDelta
				}
			case geo.Bottom:
				if bottomDelta > 0 {
					iconBox.TopLeft.Y -= bottomDelta
				}
			case geo.Left:
				if leftDelta > 0 {
					iconBox.TopLeft.X += leftDelta
				}
			case geo.Right:
				if rightDelta > 0 {
					iconBox.TopLeft.X -= rightDelta
				}
			}
			switch iconSide {
			case geo.Top:
				if topDelta > 0 {
					for _, box := range innerBoxes {
						if iconBox.Overlaps(box) {
							dy := iconBox.TopLeft.Y + iconBox.Height - box.TopLeft.Y
							topOverlap = go2.Max(topOverlap, dy)
						}
					}
				}
			case geo.Bottom:
				if bottomDelta > 0 {
					for _, box := range innerBoxes {
						if iconBox.Overlaps(box) {
							dy := box.TopLeft.Y + box.Height - iconBox.TopLeft.Y
							bottomOverlap = go2.Max(bottomOverlap, dy)
						}
					}
				}
			case geo.Left:
				if leftDelta > 0 {
					for _, box := range innerBoxes {
						if iconBox.Overlaps(box) {
							dx := iconBox.TopLeft.X + iconBox.Width - box.TopLeft.X
							leftOverlap = go2.Max(leftOverlap, dx)
						}
					}
				}
			case geo.Right:
				if rightDelta > 0 {
					for _, box := range innerBoxes {
						if iconBox.Overlaps(box) {
							dx := box.TopLeft.X + box.Width - iconBox.TopLeft.X
							rightOverlap = go2.Max(rightOverlap, dx)
						}
					}
				}
			}
		}

		if leftOverlap > 0 {
			leftDelta -= leftOverlap + MIN_SPACING
		}
		if rightOverlap > 0 {
			rightDelta -= rightOverlap + MIN_SPACING
		}
		if topOverlap > 0 {
			topDelta -= topOverlap + MIN_SPACING
		}
		if bottomOverlap > 0 {
			bottomDelta -= bottomOverlap + MIN_SPACING
		}
	}

	if 0 < topDelta {
		topDelta = adjustDeltaForEdges(obj, currentTop, topDelta, false)
		if 0 < topDelta {
			adjustEdges(obj, currentTop, topDelta, false)
			obj.TopLeft.Y += topDelta
			obj.Height -= topDelta
		}
	}
	if 0 < bottomDelta {
		bottomDelta = adjustDeltaForEdges(obj, currentBottom, -bottomDelta, false)
		if 0 < bottomDelta {
			adjustEdges(obj, currentBottom, -bottomDelta, false)
			obj.Height -= bottomDelta
		}
	}
	if 0 < leftDelta {
		leftDelta = adjustDeltaForEdges(obj, currentLeft, leftDelta, true)
		if 0 < leftDelta {
			adjustEdges(obj, currentLeft, leftDelta, true)
			obj.TopLeft.X += leftDelta
			obj.Width -= leftDelta
		}
	}
	if 0 < rightDelta {
		rightDelta = adjustDeltaForEdges(obj, currentRight, -rightDelta, true)
		if 0 < rightDelta {
			adjustEdges(obj, currentRight, -rightDelta, true)
			obj.Width -= rightDelta
		}
	}
}

func adjustDeltaForEdges(obj *d2graph.Object, objPosition, delta float64, isHorizontal bool) (newMagnitude float64) {
	isOnCollapsingSide := func(p *geo.Point) bool {
		var position float64
		if isHorizontal {
			position = p.X
		} else {
			position = p.Y
		}
		if geo.PrecisionCompare(position, objPosition, 1) == 0 {
			return false
		}
		// check for edges on side corners
		var isOnSide bool
		if isHorizontal {
			if geo.PrecisionCompare(p.Y, obj.TopLeft.Y, 1) == 0 ||
				geo.PrecisionCompare(p.Y, obj.TopLeft.Y+obj.Height, 1) == 0 {
				isOnSide = true
			}
		} else {
			if geo.PrecisionCompare(p.X, obj.TopLeft.X, 1) == 0 ||
				geo.PrecisionCompare(p.X, obj.TopLeft.X+obj.Width, 1) == 0 {
				isOnSide = true
			}
		}
		if !isOnSide {
			return false
		}
		buffer := MIN_SPACING
		var isInRange bool
		if delta > 0 {
			if objPosition <= position && position <= objPosition+delta+buffer {
				isInRange = true
			}
		} else {
			if objPosition+delta-buffer <= position && position <= objPosition {
				isInRange = true
			}
		}
		return isInRange
	}
	hasEdgeOnCollapsingSide := false
	outermost := objPosition + delta
	for _, edge := range obj.Graph.Edges {
		if edge.Src == obj {
			p := edge.Route[0]
			if isOnCollapsingSide(p) {
				hasEdgeOnCollapsingSide = true
				var position float64
				if isHorizontal {
					position = p.X
				} else {
					position = p.Y
				}
				if delta < 0 {
					outermost = math.Max(outermost, position)
				} else {
					outermost = math.Min(outermost, position)
				}
			}
		}
		if edge.Dst == obj {
			p := edge.Route[len(edge.Route)-1]
			if isOnCollapsingSide(p) {
				hasEdgeOnCollapsingSide = true
				var position float64
				if isHorizontal {
					position = p.X
				} else {
					position = p.Y
				}
				if delta < 0 {
					outermost = math.Max(outermost, position)
				} else {
					outermost = math.Min(outermost, position)
				}
			}
		}
	}
	newMagnitude = math.Abs(delta)
	if hasEdgeOnCollapsingSide {
		// only reduce to outermost + DEFAULT_PADDING
		if delta < 0 {
			newMagnitude = math.Max(0, objPosition-(outermost+DEFAULT_PADDING))
		} else {
			newMagnitude = math.Max(0, (outermost-DEFAULT_PADDING)-objPosition)
		}
	}
	return newMagnitude
}

func adjustEdges(obj *d2graph.Object, objPosition, delta float64, isHorizontal bool) {
	adjust := func(p *geo.Point) {
		var position float64
		if isHorizontal {
			position = p.X
		} else {
			position = p.Y
		}
		if geo.PrecisionCompare(position, objPosition, 1) == 0 {
			if isHorizontal {
				p.X += delta
			} else {
				p.Y += delta
			}
		} else {
			// check side corners
			var isOnSide bool
			if isHorizontal {
				if geo.PrecisionCompare(p.Y, obj.TopLeft.Y, 1) == 0 ||
					geo.PrecisionCompare(p.Y, obj.TopLeft.Y+obj.Height, 1) == 0 {
					isOnSide = true
				}
			} else {
				if geo.PrecisionCompare(p.X, obj.TopLeft.X, 1) == 0 ||
					geo.PrecisionCompare(p.X, obj.TopLeft.X+obj.Width, 1) == 0 {
					isOnSide = true
				}
			}
			if isOnSide {
				var isInRange bool
				if delta > 0 {
					if objPosition < position && position < objPosition+delta {
						isInRange = true
					}
				} else {
					if objPosition+delta < position && position < objPosition {
						isInRange = true
					}
				}
				if isInRange {
					if isHorizontal {
						p.X = objPosition + delta
					} else {
						p.Y = objPosition + delta
					}
				}
			}
		}
	}

	for _, edge := range obj.Graph.Edges {
		if edge.Src == obj {
			adjust(edge.Route[0])
		}
		if edge.Dst == obj {
			adjust(edge.Route[len(edge.Route)-1])
		}
	}
}
