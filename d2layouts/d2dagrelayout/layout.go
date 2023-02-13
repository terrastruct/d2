package d2dagrelayout

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
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
	"oss.terrastruct.com/d2/lib/shape"
)

//go:embed setup.js
var setupJS string

//go:embed dagre.js
var dagreJS string

const (
	MIN_SEGMENT_LEN = 10
	MIN_RANK_SEP    = 60
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
	switch g.Root.Attributes.Direction.Value {
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

	maxContainerLabelHeight := 0
	for _, obj := range g.Objects {
		if len(obj.ChildrenArray) == 0 || obj.Parent == g.Root {
			continue
		}
		if obj.LabelHeight != nil {
			maxContainerLabelHeight = go2.Max(maxContainerLabelHeight, *obj.LabelHeight+label.PADDING)
		}

		if obj.Attributes.Icon != nil && obj.Attributes.Shape.Value != d2target.ShapeImage {
			contentBox := geo.NewBox(geo.NewPoint(0, 0), float64(obj.Width), float64(obj.Height))
			shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[obj.Attributes.Shape.Value]
			s := shape.NewShape(shapeType, contentBox)
			iconSize := d2target.GetIconSize(s.GetInnerBox(), string(label.InsideTopLeft))
			// Since dagre container labels are pushed up, we don't want a child container to collide
			maxContainerLabelHeight = go2.Max(maxContainerLabelHeight, (iconSize+label.PADDING*2)*2)
		}
	}

	maxLabelSize := 0
	for _, edge := range g.Edges {
		size := edge.LabelDimensions.Width
		if !isHorizontal {
			size = edge.LabelDimensions.Height
		}
		maxLabelSize = go2.Max(maxLabelSize, size)
	}
	rootAttrs.ranksep = go2.Max(go2.Max(100, maxLabelSize+40), maxContainerLabelHeight)

	configJS := setGraphAttrs(rootAttrs)
	if _, err := vm.RunString(configJS); err != nil {
		return err
	}

	loadScript := ""
	idToObj := make(map[string]*d2graph.Object)
	for _, obj := range g.Objects {
		id := obj.AbsID()
		idToObj[id] = obj

		height := obj.Height
		if obj.LabelWidth != nil && obj.LabelHeight != nil {
			if obj.Attributes.Shape.Value == d2target.ShapeImage || obj.Attributes.Icon != nil {
				height += float64(*obj.LabelHeight) + label.PADDING
			}
			if len(obj.ChildrenArray) > 0 {
				height += float64(*obj.LabelHeight) + label.PADDING
			}
		}
		loadScript += generateAddNodeLine(id, int(obj.Width), int(height))
		if obj.Parent != g.Root {
			loadScript += generateAddParentLine(id, obj.Parent.AbsID())
		}
	}
	for _, edge := range g.Edges {
		// dagre doesn't work with edges to containers so we connect container edges to their first child instead (going all the way down)
		// we will chop the edge where it intersects the container border so it only shows the edge from the container
		src := edge.Src
		for len(src.Children) > 0 && src.Class == nil && src.SQLTable == nil {
			src = src.ChildrenArray[0]
		}
		dst := edge.Dst
		for len(dst.Children) > 0 && dst.Class == nil && dst.SQLTable == nil {
			dst = dst.ChildrenArray[0]
		}
		if edge.SrcArrow && !edge.DstArrow {
			// for `b <- a`, edge.Edge is `a -> b` and we expect this routing result
			src, dst = dst, src
		}
		loadScript += generateAddEdgeLine(src.AbsID(), dst.AbsID(), edge.AbsID(), edge.LabelDimensions.Width, edge.LabelDimensions.Height)
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
		obj.Width = dn.Width
		obj.Height = dn.Height

		if obj.LabelWidth != nil && obj.LabelHeight != nil {
			if len(obj.ChildrenArray) > 0 {
				obj.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
			} else if obj.Attributes.Shape.Value == d2target.ShapeImage {
				obj.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
				// remove the extra height we added to the node when passing to dagre
				obj.Height -= float64(*obj.LabelHeight) + label.PADDING
			} else if obj.Attributes.Icon != nil {
				obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			} else {
				obj.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
			}
		}
		if obj.Attributes.Icon != nil {
			if len(obj.ChildrenArray) > 0 {
				obj.IconPosition = go2.Pointer(string(label.OutsideTopLeft))
				obj.LabelPosition = go2.Pointer(string(label.OutsideTopRight))
			} else {
				obj.IconPosition = go2.Pointer(string(label.InsideMiddleCenter))
			}
		}
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

	for _, obj := range g.Objects {
		if obj.LabelHeight == nil || len(obj.ChildrenArray) <= 0 {
			continue
		}

		// usually you don't want to take away here more than what was added, which is the label height
		// however, if the label height is more than the ranksep/2, we'll have no padding around children anymore
		// so cap the amount taken off at ranksep/2
		subtract := float64(go2.Min(rootAttrs.ranksep/2, *obj.LabelHeight+label.PADDING))

		obj.Height -= subtract

		// If the edge is connected to two descendants that are about to be downshifted, their whole route gets downshifted
		movedEdges := make(map[*d2graph.Edge]struct{})
		for _, e := range g.Edges {
			currSrc := e.Src
			currDst := e.Dst

			isSrcDesc := false
			isDstDesc := false

			for currSrc != nil {
				if currSrc == obj {
					isSrcDesc = true
					break
				}
				currSrc = currSrc.Parent
			}
			for currDst != nil {
				if currDst == obj {
					isDstDesc = true
					break
				}
				currDst = currDst.Parent
			}
			if isSrcDesc && isDstDesc {
				stepSize := subtract
				if e.Src != obj || e.Dst != obj {
					stepSize /= 2.
				}
				movedEdges[e] = struct{}{}
				for _, p := range e.Route {
					p.Y += stepSize
				}
			}
		}

		// Downshift descendents and edges that have one endpoint connected to a descendant
		q := []*d2graph.Object{obj}
		for len(q) > 0 {
			curr := q[0]
			q = q[1:]

			stepSize := subtract
			// The object itself needs to move down the height it was just subtracted
			// all descendents move half, to maintain vertical padding
			if curr != obj {
				stepSize /= 2.
			}
			curr.TopLeft.Y += stepSize
			shouldMove := func(p *geo.Point) bool {
				if curr != obj {
					return true
				}
				// Edge should only move if it's not connected to the bottom side of the shrinking container
				return p.Y != obj.TopLeft.Y+obj.Height
			}
			for _, e := range g.Edges {
				if _, ok := movedEdges[e]; ok {
					continue
				}
				if e.Src == curr {
					if shouldMove(e.Route[0]) {
						e.Route[0].Y += stepSize
					}
				}
				if e.Dst == curr {
					if shouldMove(e.Route[len(e.Route)-1]) {
						e.Route[len(e.Route)-1].Y += stepSize
					}
				}
			}
			for _, c := range curr.ChildrenArray {
				q = append(q, c)
			}
		}
	}

	for _, edge := range g.Edges {
		points := edge.Route
		startIndex, endIndex := 0, len(points)-1
		start, end := points[startIndex], points[endIndex]

		// arrowheads can appear broken if segments are very short from dagre routing a point just outside the shape
		// to fix this, we try extending the previous segment into the shape instead of having a very short segment
		if !start.Equals(points[0]) && startIndex+2 < len(points) {
			newStartingSegment := *geo.NewSegment(start, points[startIndex+1])
			if newStartingSegment.Length() < MIN_SEGMENT_LEN {
				// we don't want a very short segment right next to the source because it will mess up the arrowhead
				// instead we want to extend the next segment into the shape border if possible
				nextStart := points[startIndex+1]
				nextEnd := points[startIndex+2]

				// Note: in other direction to extend towards source
				nextSegment := *geo.NewSegment(nextStart, nextEnd)
				v := nextSegment.ToVector()
				extendedStart := nextEnd.ToVector().Add(v.AddLength(MIN_SEGMENT_LEN)).ToPoint()
				extended := *geo.NewSegment(nextEnd, extendedStart)

				if intersections := edge.Src.Box.Intersections(extended); len(intersections) > 0 {
					start = intersections[0]
					startIndex += 1
				}
			}
		}
		if !end.Equals(points[len(points)-1]) && endIndex-2 >= 0 {
			newEndingSegment := *geo.NewSegment(end, points[endIndex-1])
			if newEndingSegment.Length() < MIN_SEGMENT_LEN {
				// extend the prev segment into the shape border if possible
				prevStart := points[endIndex-2]
				prevEnd := points[endIndex-1]

				prevSegment := *geo.NewSegment(prevStart, prevEnd)
				v := prevSegment.ToVector()
				extendedEnd := prevStart.ToVector().Add(v.AddLength(MIN_SEGMENT_LEN)).ToPoint()
				extended := *geo.NewSegment(prevStart, extendedEnd)

				if intersections := edge.Dst.Box.Intersections(extended); len(intersections) > 0 {
					end = intersections[0]
					endIndex -= 1
				}
			}
		}

		srcShape := shape.NewShape(d2target.DSL_SHAPE_TO_SHAPE_TYPE[strings.ToLower(edge.Src.Attributes.Shape.Value)], edge.Src.Box)
		dstShape := shape.NewShape(d2target.DSL_SHAPE_TO_SHAPE_TYPE[strings.ToLower(edge.Dst.Attributes.Shape.Value)], edge.Dst.Box)

		// trace the edge to the specific shape's border
		points[startIndex] = shape.TraceToShapeBorder(srcShape, start, points[startIndex+1])
		points[endIndex] = shape.TraceToShapeBorder(dstShape, end, points[endIndex-1])
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
		if edge.Attributes.Label.Value != "" {
			edge.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
		}
	}

	return nil
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
