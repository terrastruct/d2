package d2dagrelayout

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"cdr.dev/slog"
	"github.com/dop251/goja"

	"oss.terrastruct.com/util-go/xdefer"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/log"
)

//go:embed setup.js
var setupJS string

//go:embed dagre.js
var dagreJS string

const (
	MIN_RANK_SEP   = 60
	EDGE_LABEL_GAP = 20
	MIN_MARGIN     = 10.
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
	vm := goja.New()
	if _, err := vm.RunString(dagreJS); err != nil {
		return err
	}
	if _, err := vm.RunString(setupJS); err != nil {
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
	if _, err := vm.RunString(configJS); err != nil {
		return err
	}

	loadScript := ""
	idToObj := make(map[string]*d2graph.Object)
	idToWidth := make(map[string]float64)
	idToHeight := make(map[string]float64)
	for _, obj := range g.Objects {
		id := obj.AbsID()
		idToObj[id] = obj

		// Note: we handle vertical spacing adjustments separately after layout,
		// but for horizontal adjustments we change the width we pass to dagre.
		// For containers, we use phantom nodes to adjust container widths
		width, height := obj.Width, obj.Height
		// if isHorizontal {
		// 	height = adjustHeight(obj)
		// 	idToHeight[id] = height
		// } else {
		// 	width = adjustWidth(obj)
		// 	idToWidth[id] = width
		// }
		// TODO update to work with direction: right/left (in that case we need to adjust heights here,
		// and horizontal spacing will be adjusted separately after layout

		loadScript += generateAddNodeLine(id, int(width), int(height))
		if obj.Parent != g.Root {
			loadScript += generateAddParentLine(id, obj.Parent.AbsID())
		}
	}

	for _, obj := range g.Objects {
		if !obj.IsContainer() || true {
			continue
		}
		id := obj.AbsID()
		phantomWidth, phantomHeight := 1, 1
		// when a container has nodes with no connections, the layout will be in a row
		// adding a node will add NodeSep width in addition to the node's width
		// to add a specific amount of space we need to subtract this from the desired width
		// if we add the phantom node at rank 0 it should be at the far right and top
		if isHorizontal {
			heightDelta := int(math.Ceil(idToHeight[id] - obj.Height))
			phantomWidth = heightDelta - rootAttrs.NodeSep
		} else {
			widthDelta := int(math.Ceil(idToWidth[id] - obj.Width))
			phantomWidth = widthDelta - rootAttrs.NodeSep
		}

		// add phantom children to adjust container widths
		phantomID := id + "___phantom"
		loadScript += generateAddNodeLine(phantomID, phantomWidth, phantomHeight)
		loadScript += generateAddParentLine(phantomID, id)
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

		loadScript += generateAddEdgeLine(src.AbsID(), dst.AbsID(), edge.AbsID(), width, height)
	}

	if debugJS {
		log.Debug(ctx, "script", slog.F("all", setupJS+configJS+loadScript))
	}

	if _, err := vm.RunString(loadScript); err != nil {
		return err
	}

	if _, err := vm.RunString(`dagre.layout(g)`); err != nil {
		if debugJS {
			log.Warn(ctx, "layout error", slog.F("err", err))
		}
		return err
	}

	for i := range g.Objects {
		val, err := vm.RunString(fmt.Sprintf("JSON.stringify(g.node(g.nodes()[%d]))", i))
		if err != nil {
			return err
		}
		var dn DagreNode
		if err := json.Unmarshal([]byte(val.String()), &dn); err != nil {
			return err
		}
		if debugJS {
			log.Debug(ctx, "graph", slog.F("json", dn))
		}

		obj := idToObj[dn.ID]

		// dagre gives center of node
		obj.TopLeft = geo.NewPoint(math.Round(dn.X-dn.Width/2), math.Round(dn.Y-dn.Height/2))
		obj.Width = math.Ceil(dn.Width)
		obj.Height = math.Ceil(dn.Height)
	}

	for i, edge := range g.Edges {
		val, err := vm.RunString(fmt.Sprintf("JSON.stringify(g.edge(g.edges()[%d]))", i))
		if err != nil {
			return err
		}
		var de DagreEdge
		if err := json.Unmarshal([]byte(val.String()), &de); err != nil {
			return err
		}
		if debugJS {
			log.Debug(ctx, "graph", slog.F("json", de))
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

	adjustSpacing(g, float64(rootAttrs.ranksep), isHorizontal)
	// for _, obj := range g.Objects {
	// 	cleanupAdjustment(obj, isHorizontal)
	// }

	for _, edge := range g.Edges {
		points := edge.Route
		startIndex, endIndex := 0, len(points)-1
		start, end := points[startIndex], points[endIndex]

		// arrowheads can appear broken if segments are very short from dagre routing a point just outside the shape
		// to fix this, we try extending the previous segment into the shape instead of having a very short segment
		if !start.Equals(points[0]) && startIndex+2 < len(points) {
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
		if !end.Equals(points[len(points)-1]) && endIndex-2 >= 0 {
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

		startIndex, endIndex = edge.TraceToShape(points, startIndex, endIndex)
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
			edge.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
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

func escapeID(id string) string {
	// fixes \\
	id = strings.ReplaceAll(id, "\\", `\\`)
	// replaces \n with \\n whenever \n is not preceded by \ (does not replace \\n)
	re := regexp.MustCompile(`[^\\]\n`)
	id = re.ReplaceAllString(id, `\\n`)
	// avoid an unescaped \r becoming a \n in the layout result
	id = strings.ReplaceAll(id, "\r", `\r`)
	return id
}

func generateAddNodeLine(id string, width, height int) string {
	id = escapeID(id)
	return fmt.Sprintf("g.setNode(`%s`, { id: `%s`, width: %d, height: %d });\n", id, id, width, height)
}

func generateAddParentLine(childID, parentID string) string {
	return fmt.Sprintf("g.setParent(`%s`, `%s`);\n", escapeID(childID), escapeID(parentID))
}

func generateAddEdgeLine(fromID, toID, edgeID string, width, height int) string {
	return fmt.Sprintf("g.setEdge({v:`%s`, w:`%s`, name:`%s`}, { width:%d, height:%d, labelpos: `c` });\n", escapeID(fromID), escapeID(toID), escapeID(edgeID), width, height)
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

type spacing struct {
	top, bottom, left, right float64
}

func getSpacing(obj *d2graph.Object) (margin, padding spacing) {
	// reserve spacing for labels
	if obj.HasLabel() {
		var position label.Position
		if obj.LabelPosition != nil {
			position = label.Position(*obj.LabelPosition)
		} else if len(obj.ChildrenArray) == 0 && obj.HasOutsideBottomLabel() {
			position = label.OutsideBottomCenter
		}

		labelWidth := float64(obj.LabelDimensions.Width) + 2*label.PADDING
		labelHeight := float64(obj.LabelDimensions.Height) + 2*label.PADDING

		switch position {
		case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight:
			margin.top = labelHeight
		case label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight:
			margin.bottom = labelHeight
		case label.OutsideLeftTop, label.OutsideLeftMiddle, label.OutsideLeftBottom:
			margin.left = labelWidth
		case label.OutsideRightTop, label.OutsideRightMiddle, label.OutsideRightBottom:
			margin.right = labelWidth
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			padding.top = labelHeight
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			padding.bottom = labelHeight
		case label.InsideMiddleLeft:
			padding.left = labelWidth
		case label.InsideMiddleRight:
			padding.right = labelWidth
		}
	}

	if obj.Icon != nil && obj.Shape.Value != d2target.ShapeImage {
		var position label.Position
		if obj.IconPosition != nil {
			position = label.Position(*obj.IconPosition)
		}

		iconSize := float64(d2target.MAX_ICON_SIZE + 2*label.PADDING)
		switch position {
		case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight:
			margin.top = math.Max(margin.top, iconSize)
		case label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight:
			margin.bottom = math.Max(margin.bottom, iconSize)
		case label.OutsideLeftTop, label.OutsideLeftMiddle, label.OutsideLeftBottom:
			margin.left = math.Max(margin.left, iconSize)
		case label.OutsideRightTop, label.OutsideRightMiddle, label.OutsideRightBottom:
			margin.right = math.Max(margin.right, iconSize)
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			padding.top = math.Max(padding.top, iconSize)
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			padding.bottom = math.Max(padding.bottom, iconSize)
		case label.InsideMiddleLeft:
			padding.left = math.Max(padding.left, iconSize)
		case label.InsideMiddleRight:
			padding.right = math.Max(padding.right, iconSize)
		}
	}

	// reserve extra space for 3d/multiple by providing dagre the larger dimensions
	dx, dy := obj.GetModifierElementAdjustments()
	margin.right += dx
	margin.top += dy

	return
}

func positionLabelsIcons(obj *d2graph.Object) {
	if obj.Icon != nil && obj.IconPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			obj.IconPosition = go2.Pointer(string(label.OutsideTopLeft))
			if obj.LabelPosition == nil {
				obj.LabelPosition = go2.Pointer(string(label.OutsideTopRight))
				return
			}
		} else {
			obj.IconPosition = go2.Pointer(string(label.InsideMiddleCenter))
		}
	}
	if obj.HasLabel() && obj.LabelPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			obj.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
		} else if obj.HasOutsideBottomLabel() {
			obj.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
		} else if obj.Icon != nil {
			obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
		} else {
			obj.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
		}
	}
}

func getRanks(g *d2graph.Graph, isHorizontal bool) (ranks [][]*d2graph.Object, objectRanks, startingParentRanks, endingParentRanks map[*d2graph.Object]int) {
	alignedObjects := make(map[float64][]*d2graph.Object)
	for _, obj := range g.Objects {
		if !obj.IsContainer() {
			if !isHorizontal {
				y := obj.TopLeft.Y + obj.Height/2
				alignedObjects[y] = append(alignedObjects[y], obj)
			} else {
				x := obj.TopLeft.X + obj.Width/2
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
	// for _, obj := range g.Objects {
	// 	if rank, has := objectRanks[obj]; has {
	// 		fmt.Printf("%v rank: %d\n", obj.AbsID(), rank)
	// 	} else {
	// 		fmt.Printf("%v rank: none\n", obj.AbsID())
	// 	}
	// }

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
	// for parent, start := range startingParentRanks {
	// 	fmt.Printf("parent %v start %v end %v\n", parent.AbsID(), start, endingParentRanks[parent])
	// }

	return ranks, objectRanks, startingParentRanks, endingParentRanks
}

func getRankRange(rank []*d2graph.Object, isHorizontal bool) (min, max float64) {
	min = math.Inf(1)
	max = math.Inf(-1)
	for _, obj := range rank {
		if isHorizontal {
			min = math.Min(min, obj.TopLeft.X)
			max = math.Max(max, obj.TopLeft.X+obj.Width)
		} else {
			min = math.Min(min, obj.TopLeft.Y)
			max = math.Max(max, obj.TopLeft.Y+obj.Height)
		}
	}
	return
}

func getPositions(ranks [][]*d2graph.Object, isHorizontal bool) (starts, centers, ends []float64) {
	for _, objects := range ranks {
		min, max := getRankRange(objects, isHorizontal)
		starts = append(starts, min)
		if isHorizontal {
			centers = append(centers, objects[0].TopLeft.X+objects[0].Width/2.)
		} else {
			centers = append(centers, objects[0].TopLeft.Y+objects[0].Height/2.)
		}
		ends = append(ends, max)
	}
	return
}

// shift everything down by distance if it is at or below start position
func shiftDown(g *d2graph.Graph, start, distance float64, isHorizontal bool) {
	if isHorizontal {
		for _, obj := range g.Objects {
			if obj.TopLeft.X < start {
				continue
			}
			obj.TopLeft.X += distance
		}
		for _, edge := range g.Edges {
			for _, p := range edge.Route {
				// Note: == so incoming edge shifts down with object at startY
				// +1 in case it is off by 1
				if p.X+1 <= start {
					continue
				}
				p.X += distance
			}
		}
	} else {
		for _, obj := range g.Objects {
			if obj.TopLeft.Y < start {
				continue
			}
			obj.TopLeft.Y += distance
		}
		for _, edge := range g.Edges {
			for _, p := range edge.Route {
				// Note: == so incoming edge shifts down with object at startY
				// +1 in case it is off by 1
				if p.Y+1 <= start {
					continue
				}
				p.Y += distance
			}
		}
	}
}

// shift down everything that is below start
// shift all nodes that are reachable via an edge or being directly below a shifting node or expanding container
// expand containers to wrap shifted nodes
func shiftReachableDown(g *d2graph.Graph, obj *d2graph.Object, start, distance float64, isHorizontal, isMargin bool) {
	fmt.Printf("shifting %v at %v by %v\n", obj.AbsID(), start, distance)
	// if obj.ID == "s" || obj.ID == "k" || true {
	// 	return
	// }
	q := []*d2graph.Object{obj}

	seen := make(map[*d2graph.Object]struct{})
	shifted := make(map[*d2graph.Object]struct{})
	shiftedEdges := make(map[*d2graph.Edge]struct{})
	queue := func(o *d2graph.Object) {
		if _, in := seen[o]; in {
			return
		}
		fmt.Printf("queue %v\n", o.AbsID())
		q = append(q, o)
	}

	checkBelow := func(curr *d2graph.Object) {
		fmt.Printf("checking below %v\n", curr.AbsID())
		currBottom := curr.TopLeft.Y + curr.Height
		currRight := curr.TopLeft.X + curr.Width
		// if object below is within this distance after shifting, also shift it
		threshold := 100.
		if isHorizontal {
			originalRight := currRight
			if _, in := shifted[curr]; in {
				originalRight -= distance
			}
			for _, other := range g.Objects {
				if other == curr || curr.IsDescendantOf(other) {
					continue
				}
				// fmt.Printf("%#v && %#v original right %v currRight %v other left %v\n\t%v\n", curr.AbsID(), other.AbsID(),
				// 	originalRight, currRight, other.TopLeft.X,
				// 	other.TopLeft.X-originalRight,
				// )
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
				fmt.Printf("\twas seen %v\n", curr.AbsID())
				continue
			}
			// skip other objects behind start
			if curr != obj {
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

			if isHorizontal {
				shift := false
				if !isMargin {
					shift = start < curr.TopLeft.X
				} else {
					shift = start <= curr.TopLeft.X
				}

				if shift {
					curr.TopLeft.X += distance
					fmt.Printf("\tshifted %v\n", curr.AbsID())
					shifted[curr] = struct{}{}
				}
			} else {
				shift := false
				if !isMargin {
					shift = start < curr.TopLeft.Y
				} else {
					shift = start <= curr.TopLeft.Y
				}
				if shift {
					curr.TopLeft.Y += distance
					fmt.Printf("\tshifted %v\n", curr.AbsID())
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
					// shift the whole self-loop with object
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
					fmt.Printf("\tshifted %v\n", e.AbsID())
					continue
				} else if e.Src == curr {
					queue(e.Dst)
					if isHorizontal {
						for _, p := range e.Route {
							if start <= p.X {
								p.X += distance
							}
						}
					} else {
						for _, p := range e.Route {
							if start <= p.Y {
								p.Y += distance
							}
						}
					}
					shiftedEdges[e] = struct{}{}
				} else if e.Dst == curr {
					queue(e.Src)
					if isHorizontal {
						for _, p := range e.Route {
							if start <= p.X {
								p.X += distance
							}
						}
					} else {
						for _, p := range e.Route {
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
					fmt.Printf("grow %v\n", parent.AbsID())

					checkBelow(parent)
					processQueue()
				}
			} else {
				if parent.TopLeft.Y < start {
					parent.Height += distance
					grown[parent] = struct{}{}
					fmt.Printf("grow %v\n", parent.AbsID())

					checkBelow(parent)
					processQueue()
				}
			}

		}

	}
}

func adjustRankSpacing(g *d2graph.Graph, rankSep float64, isHorizontal bool) {
	ranks, _, startingParentRanks, endingParentRanks := getRanks(g, isHorizontal)
	starts, _, ends := getPositions(ranks, isHorizontal)

	// shifting bottom rank down first, then moving up to next rank
	for rank := len(ranks) - 1; rank >= 0; rank-- {
		objects := ranks[rank]
		rankMin := starts[rank]
		rankMax := ends[rank]
		var topMargin, bottomMargin, leftMargin, rightMargin float64
		var topPadding, bottomPadding, leftPadding, rightPadding float64
		for _, obj := range objects {
			margin, padding := getSpacing(obj)

			if isHorizontal {
				// if this object isn't the widest in the rank, the actual margin for the rank may be smaller
				// so we compute how much margin goes past the rankMin
				rankMarginLeft := obj.TopLeft.X - rankMin + margin.left
				rankMarginRight := obj.TopLeft.X + obj.Width + margin.right - rankMax
				leftMargin = math.Max(leftMargin, rankMarginLeft)
				rightMargin = math.Max(rightMargin, rankMarginRight)

				topMargin = math.Max(topMargin, margin.top)
				bottomMargin = math.Max(bottomMargin, margin.bottom)
			} else {
				rankMarginTop := obj.TopLeft.Y - rankMin + margin.top
				rankMarginBottom := obj.TopLeft.Y + obj.Height + margin.bottom - rankMax
				topMargin = math.Max(topMargin, rankMarginTop)
				bottomMargin = math.Max(bottomMargin, rankMarginBottom)

				leftMargin = math.Max(leftMargin, margin.left)
				rightMargin = math.Max(rightMargin, margin.right)
			}

			padTopDelta := padding.top - obj.Height/2.
			padBottomDelta := padding.bottom - obj.Height/2.
			padLeftDelta := padding.left - obj.Width/2.
			padRightDelta := padding.right - obj.Width/2.
			// if padTopDelta > 0 {
			// 	obj.Height += padTopDelta
			// }
			// if padBottomDelta > 0 {
			// 	obj.Height += padBottomDelta
			// }
			// if padLeftDelta > 0 {
			// 	obj.Width += padLeftDelta
			// }
			// if padRightDelta > 0 {
			// 	obj.Width += padRightDelta
			// }

			topPadding = math.Max(topPadding, padTopDelta)
			bottomPadding = math.Max(bottomPadding, padBottomDelta)
			leftPadding = math.Max(leftPadding, padLeftDelta)
			rightPadding = math.Max(rightPadding, padRightDelta)
		}

		var startDelta, endDelta float64
		var startPaddingDelta, endPaddingDelta float64
		if isHorizontal {
			startDelta = math.Max(0, MIN_MARGIN+leftMargin-rankSep/2.)
			endDelta = math.Max(0, MIN_MARGIN+rightMargin-rankSep/2.)

			// startPaddingDelta = leftPadding
			// endPaddingDelta = rightPadding
		} else {
			startDelta = math.Max(0, MIN_MARGIN+topMargin-rankSep/2.)
			endDelta = math.Max(0, MIN_MARGIN+bottomMargin-rankSep/2.)

			// startPaddingDelta = topPadding
			// endPaddingDelta = bottomPadding
		}

		fmt.Printf("r%v start %v sp %v ep %v end %v\n", rank, startDelta, startPaddingDelta, endPaddingDelta, endDelta)
		// +1 to not include edges at bottom
		if endDelta > 0 {
			shiftDown(g, ends[rank]+1, endDelta, isHorizontal)
		}
		// TODO each ancestor container of rank may need its own padding shift
		if endPaddingDelta > 0 {
			shiftDown(g, ends[rank]-endPaddingDelta, endPaddingDelta, isHorizontal)
		}
		if startPaddingDelta > 0 {
			shiftDown(g, starts[rank]+startPaddingDelta, startPaddingDelta, isHorizontal)
		}
		if startDelta > 0 {
			shiftDown(g, starts[rank], startDelta, isHorizontal)
		}

		additionalStarts := make(map[float64]float64)
		additionalEnds := make(map[float64]float64)
		var startCoords, endCoords []float64
		for _, obj := range g.Objects {
			if !obj.IsContainer() {
				continue
			}
			start := startingParentRanks[obj]
			end := endingParentRanks[obj]
			if start != rank && end != rank {
				continue
			}
			// check to see if container needs additional margin to parent
			margin, _ := getSpacing(obj)

			addStart := func(k, v float64) {
				if _, has := additionalStarts[k]; !has {
					additionalStarts[k] = v
					startCoords = append(startCoords, k)
				} else {
					additionalStarts[k] = math.Max(additionalStarts[k], v)
				}
			}
			addEnd := func(k, v float64) {
				if _, has := additionalEnds[k]; !has {
					additionalEnds[k] = v
					endCoords = append(endCoords, k)
				} else {
					additionalEnds[k] = math.Max(additionalEnds[k], v)
				}
			}

			if start == rank {
				if isHorizontal && margin.left > 0 {
					addStart(obj.TopLeft.X, margin.left)
				} else if !isHorizontal && margin.top > 0 {
					addStart(obj.TopLeft.Y, margin.top)
				}
			}
			if end == rank {
				if isHorizontal && margin.right > 0 {
					addEnd(obj.TopLeft.X+obj.Width, margin.right)
				} else if !isHorizontal && margin.bottom > 0 {
					addEnd(obj.TopLeft.Y+obj.Height, margin.bottom)
				}
			}
		}
		// bottom up
		sort.Slice(startCoords, func(i, j int) bool {
			return startCoords[i] > startCoords[j]
		})
		sort.Slice(endCoords, func(i, j int) bool {
			return endCoords[i] > endCoords[j]
		})
		for _, coord := range endCoords {
			delta := MIN_MARGIN + additionalEnds[coord] - rankSep/2
			if delta <= 0 {
				continue
			}
			for _, obj := range g.Objects {
				if !obj.IsContainer() {
					continue
				}
				start := startingParentRanks[obj]
				end := endingParentRanks[obj]
				if start <= rank && rank <= end {
					// don't want to grow the container that is shifting
					if isHorizontal && obj.TopLeft.X+obj.Width > coord {
						obj.Width += delta
					} else if !isHorizontal && obj.TopLeft.Y+obj.Height > coord {
						obj.Height += delta
					}
				}
			}
			shiftDown(g, coord+delta, delta, isHorizontal)
		}
		for _, coord := range startCoords {
			delta := MIN_MARGIN + additionalStarts[coord] - rankSep/2
			if delta <= 0 {
				continue
			}
			for _, obj := range g.Objects {
				if !obj.IsContainer() {
					continue
				}
				start := startingParentRanks[obj]
				end := endingParentRanks[obj]
				// expand all containers that pass this rank, except the ones that are moving down to fit the icon
				if start <= rank && rank <= end {
					// expand the containers that contain the ones moving down
					if isHorizontal && obj.TopLeft.X < coord {
						obj.Width += delta
					} else if !isHorizontal && obj.TopLeft.Y < coord {
						obj.Height += delta
					}
				}
			}
			shiftDown(g, coord, delta, isHorizontal)
		}

		// We need to expand parents when shifting descendants downwards
		for _, obj := range g.Objects {
			if !obj.IsContainer() {
				continue
			}
			start := startingParentRanks[obj]
			end := endingParentRanks[obj]
			if start <= rank && rank <= end {
				if isHorizontal {
					obj.Width += startDelta + endDelta + startPaddingDelta + endPaddingDelta
				} else {
					obj.Height += startDelta + endDelta + startPaddingDelta + endPaddingDelta
				}
			}
		}
	}

}

func adjustSpacing(g *d2graph.Graph, rankSep float64, isHorizontal bool) {
	adjustRankSpacing(g, rankSep, isHorizontal)

	// adjust cross-rank spacing
	crossRankIsHorizontal := !isHorizontal
	for _, obj := range g.Objects {
		margin, padding := getSpacing(obj)
		if isHorizontal {
			if margin.bottom > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.Y+obj.Height, margin.bottom, crossRankIsHorizontal, true)
			}
			if padding.bottom > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.Y+obj.Height, padding.bottom, crossRankIsHorizontal, false)
				obj.Height += padding.bottom
			}
			if margin.top > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.Y, margin.top, crossRankIsHorizontal, true)
			}
			if padding.top > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.Y, padding.top, crossRankIsHorizontal, false)
				obj.Height += padding.top
			}
		} else {
			if margin.right > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.X+obj.Width, margin.right, crossRankIsHorizontal, true)
			}
			if padding.right > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.X+obj.Width, padding.right, crossRankIsHorizontal, false)
				obj.Width += padding.right
			}
			if margin.left > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.X, margin.left, crossRankIsHorizontal, true)
			}
			if padding.left > 0 {
				shiftReachableDown(g, obj, obj.TopLeft.X, padding.left, crossRankIsHorizontal, false)
				obj.Width += padding.left
			}
		}
	}
}
