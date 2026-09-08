// Package d2elklayout adapts D2 graphs to the native Go port of ELK.
//
// Coordinates are relative to parents.
// See https://www.eclipse.org/elk/documentation/tooldevelopers/graphdatastructure/coordinatesystem.html
package d2elklayout

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	elk "github.com/d2lang/elk-go"
	"github.com/d2lang/util-go/xdefer"

	"github.com/d2lang/util-go/go2"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

type ELKNode struct {
	ID            string      `json:"id"`
	X             float64     `json:"x"`
	Y             float64     `json:"y"`
	Width         float64     `json:"width"`
	Height        float64     `json:"height"`
	Children      []*ELKNode  `json:"children,omitempty"`
	Ports         []*ELKPort  `json:"ports,omitempty"`
	Labels        []*ELKLabel `json:"labels,omitempty"`
	LayoutOptions *elkOpts    `json:"layoutOptions,omitempty"`
}

type PortSide string

const (
	South PortSide = "SOUTH"
	North PortSide = "NORTH"
	East  PortSide = "EAST"
	West  PortSide = "WEST"
)

type Direction string

const (
	Down  Direction = "DOWN"
	Up    Direction = "UP"
	Right Direction = "RIGHT"
	Left  Direction = "LEFT"
)

type ELKPort struct {
	ID            string   `json:"id"`
	X             float64  `json:"x"`
	Y             float64  `json:"y"`
	Width         float64  `json:"width"`
	Height        float64  `json:"height"`
	LayoutOptions *elkOpts `json:"layoutOptions,omitempty"`
}

type ELKLabel struct {
	Text          string   `json:"text"`
	X             float64  `json:"x"`
	Y             float64  `json:"y"`
	Width         float64  `json:"width"`
	Height        float64  `json:"height"`
	LayoutOptions *elkOpts `json:"layoutOptions,omitempty"`
}

type ELKPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type ELKEdgeSection struct {
	Start      ELKPoint   `json:"startPoint"`
	End        ELKPoint   `json:"endPoint"`
	BendPoints []ELKPoint `json:"bendPoints,omitempty"`
}

type ELKEdge struct {
	ID        string           `json:"id"`
	Sources   []string         `json:"sources"`
	Targets   []string         `json:"targets"`
	Sections  []ELKEdgeSection `json:"sections,omitempty"`
	Labels    []*ELKLabel      `json:"labels,omitempty"`
	Container string           `json:"container"`
}

type ELKGraph struct {
	ID            string     `json:"id"`
	LayoutOptions *elkOpts   `json:"layoutOptions"`
	Children      []*ELKNode `json:"children,omitempty"`
	Edges         []*ELKEdge `json:"edges,omitempty"`
}

type ConfigurableOpts struct {
	Algorithm       string `json:"elk.algorithm,omitempty"`
	NodeSpacing     int    `json:"spacing.nodeNodeBetweenLayers,omitempty"`
	Padding         string `json:"elk.padding,omitempty"`
	EdgeNodeSpacing int    `json:"spacing.edgeNodeBetweenLayers,omitempty"`
	EdgeEdgeSpacing int    `json:"spacing.edgeEdgeBetweenLayers"`
	SelfLoopSpacing int    `json:"elk.spacing.nodeSelfLoop"`
}

var DefaultOpts = ConfigurableOpts{
	Algorithm:       "layered",
	NodeSpacing:     70.0,
	Padding:         "[top=50,left=50,bottom=50,right=50]",
	EdgeNodeSpacing: 40.0,
	EdgeEdgeSpacing: 50.0,
	SelfLoopSpacing: 50.0,
}

var port_spacing = 40.
var edge_node_spacing = 40

type objectDegree struct {
	incoming float64
	outgoing float64
}

type selfLoopLabelSize struct {
	width  int
	height int
}

// elkGraphStats contains the D2-specific graph-wide measurements used while
// adapting a graph to ELK. Keeping them here avoids rescanning every edge for
// every object and container.
type elkGraphStats struct {
	degrees          map[*d2graph.Object]objectDegree
	selfLoopLabelMax map[*d2graph.Object]selfLoopLabelSize
}

func collectELKGraphStats(g *d2graph.Graph) elkGraphStats {
	stats := elkGraphStats{
		degrees:          make(map[*d2graph.Object]objectDegree, len(g.Objects)),
		selfLoopLabelMax: make(map[*d2graph.Object]selfLoopLabelSize),
	}
	for _, edge := range g.Edges {
		srcDegree := stats.degrees[edge.Src]
		srcDegree.outgoing++
		stats.degrees[edge.Src] = srcDegree

		dstDegree := stats.degrees[edge.Dst]
		dstDegree.incoming++
		stats.degrees[edge.Dst] = dstDegree

		if edge.Src != edge.Dst || edge.Label.Value == "" || edge.Src.Parent == nil {
			continue
		}
		parent := edge.Src.Parent
		labelSize := stats.selfLoopLabelMax[parent]
		labelSize.width = go2.Max(labelSize.width, edge.LabelDimensions.Width)
		labelSize.height = go2.Max(labelSize.height, edge.LabelDimensions.Height)
		stats.selfLoopLabelMax[parent] = labelSize
	}
	return stats
}

func (s elkGraphStats) maxSelfLoopLabel(parent *d2graph.Object, isWidth bool) int {
	size := s.selfLoopLabelMax[parent]
	if isWidth {
		return size.width
	}
	return size.height
}

type elkOpts struct {
	EdgeNode              int       `json:"elk.spacing.edgeNode,omitempty"`
	FixedAlignment        string    `json:"elk.layered.nodePlacement.bk.fixedAlignment,omitempty"`
	Thoroughness          int       `json:"elk.layered.thoroughness,omitempty"`
	Direction             Direction `json:"elk.direction"`
	HierarchyHandling     string    `json:"elk.hierarchyHandling,omitempty"`
	InlineEdgeLabels      bool      `json:"elk.edgeLabels.inline,omitempty"`
	ForceNodeModelOrder   bool      `json:"elk.layered.crossingMinimization.forceNodeModelOrder,omitempty"`
	ConsiderModelOrder    string    `json:"elk.layered.considerModelOrder.strategy,omitempty"`
	CycleBreakingStrategy string    `json:"elk.layered.cycleBreaking.strategy,omitempty"`

	SelfLoopDistribution string `json:"elk.layered.edgeRouting.selfLoopDistribution,omitempty"`

	NodeSizeConstraints string `json:"elk.nodeSize.constraints,omitempty"`
	ContentAlignment    string `json:"elk.contentAlignment,omitempty"`
	NodeSizeMinimum     string `json:"elk.nodeSize.minimum,omitempty"`

	PortSide        PortSide `json:"elk.port.side,omitempty"`
	PortConstraints string   `json:"elk.portConstraints,omitempty"`

	ConfigurableOpts
}

func newRootLayoutOptions(opts *ConfigurableOpts) *elkOpts {
	return &elkOpts{
		Thoroughness:          8,
		EdgeNode:              edge_node_spacing,
		HierarchyHandling:     "INCLUDE_CHILDREN",
		FixedAlignment:        "BALANCED",
		ConsiderModelOrder:    "NODES_AND_EDGES",
		CycleBreakingStrategy: "GREEDY_MODEL_ORDER",
		NodeSizeConstraints:   "MINIMUM_SIZE",
		ContentAlignment:      "H_CENTER V_CENTER",
		ConfigurableOpts: ConfigurableOpts{
			Algorithm:       opts.Algorithm,
			NodeSpacing:     opts.NodeSpacing,
			EdgeNodeSpacing: opts.EdgeNodeSpacing,
			EdgeEdgeSpacing: opts.EdgeEdgeSpacing,
			SelfLoopSpacing: opts.SelfLoopSpacing,
		},
	}
}

func newContainerLayoutOptions(opts *ConfigurableOpts) *elkOpts {
	return &elkOpts{
		Thoroughness:      8,
		HierarchyHandling: "INCLUDE_CHILDREN",
		FixedAlignment:    "BALANCED",
		EdgeNode:          edge_node_spacing,
		// ELK 0.12 crashes on compound subgraphs when model-order
		// processing is enabled below the root. Keep it disabled there.
		ConsiderModelOrder:    "NONE",
		CycleBreakingStrategy: "GREEDY",
		NodeSizeConstraints:   "MINIMUM_SIZE",
		ContentAlignment:      "H_CENTER V_CENTER",
		ConfigurableOpts: ConfigurableOpts{
			NodeSpacing:     opts.NodeSpacing,
			EdgeNodeSpacing: opts.EdgeNodeSpacing,
			EdgeEdgeSpacing: opts.EdgeEdgeSpacing,
			SelfLoopSpacing: opts.SelfLoopSpacing,
			Padding:         opts.Padding,
		},
	}
}

func DefaultLayout(ctx context.Context, g *d2graph.Graph) (err error) {
	return Layout(ctx, g, nil)
}

func Layout(ctx context.Context, g *d2graph.Graph, opts *ConfigurableOpts) (err error) {
	if opts == nil {
		opts = &DefaultOpts
	}
	defer xdefer.Errorf(&err, "failed to ELK layout")
	if err := ctx.Err(); err != nil {
		return err
	}
	graphStats := collectELKGraphStats(g)

	elkGraph := &ELKGraph{
		ID:            "",
		LayoutOptions: newRootLayoutOptions(opts),
	}
	if elkGraph.LayoutOptions.ConfigurableOpts.SelfLoopSpacing == DefaultOpts.SelfLoopSpacing {
		// +5 for a tiny bit of padding
		elkGraph.LayoutOptions.ConfigurableOpts.SelfLoopSpacing = go2.Max(elkGraph.LayoutOptions.ConfigurableOpts.SelfLoopSpacing, graphStats.maxSelfLoopLabel(g.Root, g.Root.Direction.Value == "down" || g.Root.Direction.Value == "" || g.Root.Direction.Value == "up")/2+5)
	}
	switch g.Root.Direction.Value {
	case "down":
		elkGraph.LayoutOptions.Direction = Down
	case "up":
		elkGraph.LayoutOptions.Direction = Up
	case "right":
		elkGraph.LayoutOptions.Direction = Right
	case "left":
		elkGraph.LayoutOptions.Direction = Left
	default:
		elkGraph.LayoutOptions.Direction = Down
	}

	// set label and icon positions for ELK
	for _, obj := range g.Objects {
		positionLabelsIcons(obj)
	}

	adjustments := make(map[*d2graph.Object]geo.Spacing)
	elkNodes := make(map[*d2graph.Object]*ELKNode)
	elkEdges := make(map[*d2graph.Edge]*ELKEdge)

	// BFS
	var walk func(*d2graph.Object, *d2graph.Object, func(*d2graph.Object, *d2graph.Object))
	walk = func(obj, parent *d2graph.Object, fn func(*d2graph.Object, *d2graph.Object)) {
		if obj.Parent != nil {
			fn(obj, parent)
		}
		for _, ch := range obj.ChildrenArray {
			walk(ch, obj, fn)
		}
	}

	walk(g.Root, nil, func(obj, parent *d2graph.Object) {
		degree := graphStats.degrees[obj]
		incoming := degree.incoming
		outgoing := degree.outgoing
		if incoming >= 2 || outgoing >= 2 {
			switch g.Root.Direction.Value {
			case "right", "left":
				if obj.Attributes.HeightAttr == nil {
					obj.Height = math.Max(obj.Height, math.Max(incoming, outgoing)*port_spacing)
				}
			default:
				if obj.Attributes.WidthAttr == nil {
					obj.Width = math.Max(obj.Width, math.Max(incoming, outgoing)*port_spacing)
				}
			}
		}

		if obj.HasLabel() && obj.HasIcon() {
			// this gives shapes extra height for their label if they also have an icon
			obj.Height += float64(obj.LabelDimensions.Height + label.PADDING)
		}

		margin, _ := obj.SpacingOpt(label.PADDING, label.PADDING, false)
		width := margin.Left + obj.Width + margin.Right
		height := margin.Top + obj.Height + margin.Bottom
		adjustments[obj] = margin

		n := &ELKNode{
			ID:     obj.AbsID(),
			Width:  width,
			Height: height,
		}

		if len(obj.ChildrenArray) > 0 {
			n.LayoutOptions = newContainerLayoutOptions(opts)
			if n.LayoutOptions.ConfigurableOpts.SelfLoopSpacing == DefaultOpts.SelfLoopSpacing {
				n.LayoutOptions.ConfigurableOpts.SelfLoopSpacing = go2.Max(n.LayoutOptions.ConfigurableOpts.SelfLoopSpacing, graphStats.maxSelfLoopLabel(obj, g.Root.Direction.Value == "down" || g.Root.Direction.Value == "" || g.Root.Direction.Value == "up")/2+5)
			}

			switch elkGraph.LayoutOptions.Direction {
			case Down, Up:
				n.LayoutOptions.NodeSizeMinimum = fmt.Sprintf("(%d, %d)", int(math.Ceil(height)), int(math.Ceil(width)))
			case Right, Left:
				n.LayoutOptions.NodeSizeMinimum = fmt.Sprintf("(%d, %d)", int(math.Ceil(width)), int(math.Ceil(height)))
			}
		} else {
			n.LayoutOptions = &elkOpts{
				SelfLoopDistribution: "EQUALLY",
			}
		}

		if obj.IsContainer() {
			padding := parsePadding(opts.Padding)
			padding = adjustPadding(obj, width, height, padding)
			n.LayoutOptions.Padding = padding.String()
		}

		if obj.HasLabel() {
			n.Labels = append(n.Labels, &ELKLabel{
				Text:   obj.Label.Value,
				Width:  float64(obj.LabelDimensions.Width),
				Height: float64(obj.LabelDimensions.Height),
			})
		}

		if parent == g.Root {
			elkGraph.Children = append(elkGraph.Children, n)
		} else {
			elkNodes[parent].Children = append(elkNodes[parent].Children, n)
		}

		if obj.SQLTable != nil {
			n.LayoutOptions.PortConstraints = "FIXED_POS"
			columns := obj.SQLTable.Columns
			colHeight := n.Height / float64(len(columns)+1)
			n.Ports = make([]*ELKPort, 0, len(columns)*2)
			var srcSide, dstSide PortSide
			switch elkGraph.LayoutOptions.Direction {
			case Left:
				srcSide, dstSide = West, East
			default:
				srcSide, dstSide = East, West
			}
			for i, col := range columns {
				n.Ports = append(n.Ports, &ELKPort{
					ID:            srcPortID(obj, col.Name.Label),
					Y:             float64(i+1)*colHeight + colHeight/2,
					LayoutOptions: &elkOpts{PortSide: srcSide},
				})
				n.Ports = append(n.Ports, &ELKPort{
					ID:            dstPortID(obj, col.Name.Label),
					Y:             float64(i+1)*colHeight + colHeight/2,
					LayoutOptions: &elkOpts{PortSide: dstSide},
				})
			}
		}

		elkNodes[obj] = n
	})

	var srcSide, dstSide PortSide
	switch elkGraph.LayoutOptions.Direction {
	case Up:
		srcSide, dstSide = North, South
	default:
		srcSide, dstSide = South, North
	}

	ports := map[struct {
		obj  *d2graph.Object
		side PortSide
	}][]*ELKPort{}

	for ei, edge := range g.Edges {
		var src, dst string

		switch {
		case edge.SrcTableColumnIndex != nil:
			src = srcPortID(edge.Src, edge.Src.SQLTable.Columns[*edge.SrcTableColumnIndex].Name.Label)
		case edge.Src.SQLTable != nil:
			p := &ELKPort{
				ID:            fmt.Sprintf("%s.%d", srcPortID(edge.Src, "__root__"), ei),
				LayoutOptions: &elkOpts{PortSide: srcSide},
			}
			src = p.ID
			elkNodes[edge.Src].Ports = append(elkNodes[edge.Src].Ports, p)
			k := struct {
				obj  *d2graph.Object
				side PortSide
			}{edge.Src, srcSide}
			ports[k] = append(ports[k], p)
		default:
			src = edge.Src.AbsID()
		}

		switch {
		case edge.DstTableColumnIndex != nil:
			dst = dstPortID(edge.Dst, edge.Dst.SQLTable.Columns[*edge.DstTableColumnIndex].Name.Label)
		case edge.Dst.SQLTable != nil:
			p := &ELKPort{
				ID:            fmt.Sprintf("%s.%d", dstPortID(edge.Dst, "__root__"), ei),
				LayoutOptions: &elkOpts{PortSide: dstSide},
			}
			dst = p.ID
			elkNodes[edge.Dst].Ports = append(elkNodes[edge.Dst].Ports, p)
			k := struct {
				obj  *d2graph.Object
				side PortSide
			}{edge.Dst, dstSide}
			ports[k] = append(ports[k], p)
		default:
			dst = edge.Dst.AbsID()
		}

		e := &ELKEdge{
			ID:      edge.AbsID(),
			Sources: []string{src},
			Targets: []string{dst},
		}
		if edge.Label.Value != "" {
			e.Labels = append(e.Labels, &ELKLabel{
				Text:   edge.Label.Value,
				Width:  float64(edge.LabelDimensions.Width),
				Height: float64(edge.LabelDimensions.Height),
				LayoutOptions: &elkOpts{
					InlineEdgeLabels: true,
				},
			})
		}
		elkGraph.Edges = append(elkGraph.Edges, e)
		elkEdges[edge] = e
	}

	for k, ports := range ports {
		width := elkNodes[k.obj].Width
		spacing := width / float64(len(ports)+1)
		for i, p := range ports {
			p.X = float64(i+1) * spacing
		}
	}

	raw, err := json.Marshal(elkGraph)
	if err != nil {
		return err
	}

	jsonOut, err := elk.LayoutJSON(raw)
	if err != nil {
		return fmt.Errorf("native ELK layout failed: %w", err)
	}

	err = json.Unmarshal(jsonOut, &elkGraph)
	if err != nil {
		return err
	}

	byID := make(map[string]*d2graph.Object)
	walk(g.Root, nil, func(obj, parent *d2graph.Object) {
		n := elkNodes[obj]

		parentX := 0.0
		parentY := 0.0
		if parent != nil && parent != g.Root {
			parentX = parent.TopLeft.X
			parentY = parent.TopLeft.Y
		}
		obj.TopLeft = geo.NewPoint(parentX+n.X, parentY+n.Y)
		obj.Width = math.Ceil(n.Width)
		obj.Height = math.Ceil(n.Height)

		byID[obj.AbsID()] = obj
	})

	for _, edge := range g.Edges {
		e := elkEdges[edge]

		parentX := 0.0
		parentY := 0.0
		if e.Container != "" {
			parentX = byID[e.Container].TopLeft.X
			parentY = byID[e.Container].TopLeft.Y
		}

		var points []*geo.Point
		for _, s := range e.Sections {
			points = append(points, &geo.Point{
				X: parentX + s.Start.X,
				Y: parentY + s.Start.Y,
			})
			for _, bp := range s.BendPoints {
				points = append(points, &geo.Point{
					X: parentX + bp.X,
					Y: parentY + bp.Y,
				})
			}
			points = append(points, &geo.Point{
				X: parentX + s.End.X,
				Y: parentY + s.End.Y,
			})
		}
		edge.Route = points
	}

	objEdges := make(map[*d2graph.Object][]*d2graph.Edge)
	for _, e := range g.Edges {
		objEdges[e.Src] = append(objEdges[e.Src], e)
		if e.Dst != e.Src {
			objEdges[e.Dst] = append(objEdges[e.Dst], e)
		}
	}
	descendantShifter := newDescendantShifter(g)

	for _, obj := range g.Objects {
		if margin, has := adjustments[obj]; has {
			edges := objEdges[obj]
			// also move edges with the shrinking sides
			if margin.Left > 0 {
				for _, e := range edges {
					l := len(e.Route)
					if e.Src == obj && e.Route[0].X == obj.TopLeft.X {
						e.Route[0].X += margin.Left
					}
					if e.Dst == obj && e.Route[l-1].X == obj.TopLeft.X {
						e.Route[l-1].X += margin.Left
					}
				}
				obj.TopLeft.X += margin.Left
				descendantShifter.shift(obj, margin.Left/2, 0)
				obj.Width -= margin.Left
			}
			if margin.Right > 0 {
				for _, e := range edges {
					l := len(e.Route)
					if e.Src == obj && e.Route[0].X == obj.TopLeft.X+obj.Width {
						e.Route[0].X -= margin.Right
					}
					if e.Dst == obj && e.Route[l-1].X == obj.TopLeft.X+obj.Width {
						e.Route[l-1].X -= margin.Right
					}
				}
				descendantShifter.shift(obj, -margin.Right/2, 0)
				obj.Width -= margin.Right
			}
			if margin.Top > 0 {
				for _, e := range edges {
					l := len(e.Route)
					if e.Src == obj && e.Route[0].Y == obj.TopLeft.Y {
						e.Route[0].Y += margin.Top
					}
					if e.Dst == obj && e.Route[l-1].Y == obj.TopLeft.Y {
						e.Route[l-1].Y += margin.Top
					}
				}
				obj.TopLeft.Y += margin.Top
				descendantShifter.shift(obj, 0, margin.Top/2)
				obj.Height -= margin.Top
			}
			if margin.Bottom > 0 {
				for _, e := range edges {
					l := len(e.Route)
					if e.Src == obj && e.Route[0].Y == obj.TopLeft.Y+obj.Height {
						e.Route[0].Y -= margin.Bottom
					}
					if e.Dst == obj && e.Route[l-1].Y == obj.TopLeft.Y+obj.Height {
						e.Route[l-1].Y -= margin.Bottom
					}
				}
				descendantShifter.shift(obj, 0, -margin.Bottom/2)
				obj.Height -= margin.Bottom
			}
		}
	}

	for _, edge := range g.Edges {
		points := edge.Route

		startIndex, endIndex := 0, len(points)-1
		start := points[startIndex]
		end := points[endIndex]

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

		if edge.Label.Value != "" {
			edge.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}

		edge.Route = points

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

	deleteBends(g)
	deconflictSelfLoopLabels(g)

	return nil
}

func srcPortID(obj *d2graph.Object, column string) string {
	return fmt.Sprintf("%s.%s.src", obj.AbsID(), column)
}

func dstPortID(obj *d2graph.Object, column string) string {
	return fmt.Sprintf("%s.%s.dst", obj.AbsID(), column)
}

// deconflictSelfLoopLabels keeps the usual centered label position unless two
// labels on self loops around the same object materially overlap. Newer ELK
// versions can place multiple fixed-port loops on the same side of an object,
// making both route midpoints coincide. Moving only the later label preserves
// the routed geometry and existing output for non-overlapping loops.
func deconflictSelfLoopLabels(g *d2graph.Graph) {
	const materialOverlap = float64(label.PADDING)
	candidatePercentages := [...]float64{0.4, 0.6, 0.3, 0.7, 0.2, 0.8, 0.1, 0.9}

	placedByObject := make(map[*d2graph.Object][]*geo.Box)
	for _, edge := range g.Edges {
		if edge.Src != edge.Dst || edge.Label.Value == "" || edge.LabelPosition == nil || len(edge.Route) < 2 {
			continue
		}

		position := label.FromString(*edge.LabelPosition)
		percentage := 0.0
		if edge.LabelPercentage != nil {
			percentage = *edge.LabelPercentage
		}
		box := edgeLabelBox(edge, position, percentage)
		if box == nil {
			continue
		}

		placed := placedByObject[edge.Src]
		if materiallyOverlapsAny(box, placed, materialOverlap) {
			for _, candidate := range candidatePercentages {
				candidateBox := edgeLabelBox(edge, label.UnlockedMiddle, candidate)
				if candidateBox == nil || overlapsAny(candidateBox, placed) {
					continue
				}
				edge.LabelPosition = go2.Pointer(label.UnlockedMiddle.String())
				edge.LabelPercentage = go2.Pointer(candidate)
				box = candidateBox
				break
			}
		}
		placedByObject[edge.Src] = append(placed, box)
	}
}

func edgeLabelBox(edge *d2graph.Edge, position label.Position, percentage float64) *geo.Box {
	topLeft, _ := position.GetPointOnRoute(
		geo.Route(edge.Route),
		0,
		percentage,
		float64(edge.LabelDimensions.Width),
		float64(edge.LabelDimensions.Height),
	)
	if topLeft == nil {
		return nil
	}
	return geo.NewBox(topLeft, float64(edge.LabelDimensions.Width), float64(edge.LabelDimensions.Height))
}

func materiallyOverlapsAny(box *geo.Box, others []*geo.Box, minimum float64) bool {
	for _, other := range others {
		overlapWidth := math.Min(box.TopLeft.X+box.Width, other.TopLeft.X+other.Width) - math.Max(box.TopLeft.X, other.TopLeft.X)
		overlapHeight := math.Min(box.TopLeft.Y+box.Height, other.TopLeft.Y+other.Height) - math.Max(box.TopLeft.Y, other.TopLeft.Y)
		if overlapWidth > minimum && overlapHeight > minimum {
			return true
		}
	}
	return false
}

func overlapsAny(box *geo.Box, others []*geo.Box) bool {
	for _, other := range others {
		if box.Overlaps(*other) {
			return true
		}
	}
	return false
}

// deleteBends is a shim for ELK to delete unnecessary bends
// see https://github.com/terrastruct/d2/issues/1030
func deleteBends(g *d2graph.Graph) {
	hasCandidate := false
	for _, edge := range g.Edges {
		if edge.Src != edge.Dst && len(edge.Route) >= 4 {
			hasCandidate = true
			break
		}
	}
	if !hasCandidate {
		return
	}
	collisionIndex := newBendCollisionIndex(g)

	// Get rid of S-shapes at the source and the target
	// TODO there might be value in repeating this. removal of an S shape introducing another S shape that can still be removed
	for _, isSource := range []bool{true, false} {
		for ei, e := range g.Edges {
			if len(e.Route) < 4 {
				continue
			}
			if e.Src == e.Dst {
				continue
			}
			var endpoint *d2graph.Object
			var start *geo.Point
			var corner *geo.Point
			var end *geo.Point

			var columnIndex *int
			if isSource {
				start = e.Route[0]
				corner = e.Route[1]
				end = e.Route[2]
				endpoint = e.Src
				columnIndex = e.SrcTableColumnIndex
			} else {
				start = e.Route[len(e.Route)-1]
				corner = e.Route[len(e.Route)-2]
				end = e.Route[len(e.Route)-3]
				endpoint = e.Dst
				columnIndex = e.DstTableColumnIndex
			}

			isHorizontal := math.Ceil(start.Y) == math.Ceil(corner.Y)
			dx, dy := endpoint.GetModifierElementAdjustments()

			// Make sure it's still attached
			switch {
			case columnIndex != nil:
				rowHeight := endpoint.Height / float64(len(endpoint.SQLTable.Columns)+1)
				rowCenter := endpoint.TopLeft.Y + rowHeight*float64(*columnIndex+1) + rowHeight/2

				// for row connections new Y coordinate should be within 1/3 row height from the row center
				if math.Abs(end.Y-rowCenter) > rowHeight/3 {
					continue
				}
			case isHorizontal:
				if end.Y <= endpoint.TopLeft.Y+10-dy {
					continue
				}
				if end.Y >= endpoint.TopLeft.Y+endpoint.Height-10 {
					continue
				}
			default:
				if end.X <= endpoint.TopLeft.X+10 {
					continue
				}
				if end.X >= endpoint.TopLeft.X+endpoint.Width-10+dx {
					continue
				}
			}

			var newStart *geo.Point
			if isHorizontal {
				newStart = geo.NewPoint(start.X, end.Y)
			} else {
				newStart = geo.NewPoint(end.X, start.Y)
			}

			endpointShape := shape.NewShape(d2target.DSL_SHAPE_TO_SHAPE_TYPE[strings.ToLower(endpoint.Shape.Value)], endpoint.Box)
			newStart = shape.TraceToShapeBorder(endpointShape, newStart, end)

			// Check that the new segment doesn't collide with anything new

			oldSegment := geo.NewSegment(start, corner)
			newSegment := geo.NewSegment(newStart, end)

			oldIntersects := collisionIndex.countObjectIntersects(e.Src, e.Dst, *oldSegment)
			newIntersects := collisionIndex.countObjectIntersects(e.Src, e.Dst, *newSegment)

			if newIntersects > oldIntersects {
				continue
			}

			oldCrossingsCount, oldOverlapsCount, oldCloseOverlapsCount, oldTouchingCount := collisionIndex.countEdgeIntersects(g.Edges[ei], *oldSegment)
			newCrossingsCount, newOverlapsCount, newCloseOverlapsCount, newTouchingCount := collisionIndex.countEdgeIntersects(g.Edges[ei], *newSegment)

			if newCrossingsCount > oldCrossingsCount {
				continue
			}
			if newOverlapsCount > oldOverlapsCount {
				continue
			}

			if newCloseOverlapsCount > oldCloseOverlapsCount {
				continue
			}
			if newTouchingCount > oldTouchingCount {
				continue
			}

			// commit
			if isSource {
				g.Edges[ei].Route = append(
					[]*geo.Point{newStart},
					e.Route[3:]...,
				)
			} else {
				g.Edges[ei].Route = append(
					e.Route[:len(e.Route)-3],
					newStart,
				)
			}
			collisionIndex.updateEdge(g.Edges[ei])
		}
	}

	// Get rid of ladders
	// ELK likes to do these for some reason
	// .   ┌─
	// . ┌─┘
	// . │
	// We want to transform these into L-shapes

	points := map[geo.Point]int{}
	for _, e := range g.Edges {
		for _, p := range e.Route {
			points[*p]++
		}
	}

	for ei, e := range g.Edges {
		if len(e.Route) < 6 {
			continue
		}
		if e.Src == e.Dst {
			continue
		}

		for i := 1; i < len(e.Route)-3; i++ {
			before := e.Route[i-1]
			start := e.Route[i]
			corner := e.Route[i+1]
			end := e.Route[i+2]
			after := e.Route[i+3]

			if c, _ := points[*corner]; c > 1 {
				// If corner is shared with another edge, they merge
				continue
			}

			// S-shape on sources only concerned one segment, since the other was just along the bound of endpoint
			// These concern two segments

			var newCorner *geo.Point
			if math.Ceil(start.X) == math.Ceil(corner.X) {
				newCorner = geo.NewPoint(end.X, start.Y)
				// not ladder
				if (end.X > start.X) != (start.X > before.X) {
					continue
				}
				if (end.Y > start.Y) != (after.Y > end.Y) {
					continue
				}
			} else {
				newCorner = geo.NewPoint(start.X, end.Y)
				if (end.Y > start.Y) != (start.Y > before.Y) {
					continue
				}
				if (end.X > start.X) != (after.X > end.X) {
					continue
				}
			}

			oldS1 := geo.NewSegment(start, corner)
			oldS2 := geo.NewSegment(corner, end)

			newS1 := geo.NewSegment(start, newCorner)
			newS2 := geo.NewSegment(newCorner, end)

			// Check that the new segments doesn't collide with anything new
			oldIntersects := collisionIndex.countObjectIntersects(e.Src, e.Dst, *oldS1) + collisionIndex.countObjectIntersects(e.Src, e.Dst, *oldS2)
			newIntersects := collisionIndex.countObjectIntersects(e.Src, e.Dst, *newS1) + collisionIndex.countObjectIntersects(e.Src, e.Dst, *newS2)

			if newIntersects > oldIntersects {
				continue
			}

			oldCrossingsCount1, oldOverlapsCount1, oldCloseOverlapsCount1, oldTouchingCount1 := collisionIndex.countEdgeIntersects(g.Edges[ei], *oldS1)
			oldCrossingsCount2, oldOverlapsCount2, oldCloseOverlapsCount2, oldTouchingCount2 := collisionIndex.countEdgeIntersects(g.Edges[ei], *oldS2)
			oldCrossingsCount := oldCrossingsCount1 + oldCrossingsCount2
			oldOverlapsCount := oldOverlapsCount1 + oldOverlapsCount2
			oldCloseOverlapsCount := oldCloseOverlapsCount1 + oldCloseOverlapsCount2
			oldTouchingCount := oldTouchingCount1 + oldTouchingCount2

			newCrossingsCount1, newOverlapsCount1, newCloseOverlapsCount1, newTouchingCount1 := collisionIndex.countEdgeIntersects(g.Edges[ei], *newS1)
			newCrossingsCount2, newOverlapsCount2, newCloseOverlapsCount2, newTouchingCount2 := collisionIndex.countEdgeIntersects(g.Edges[ei], *newS2)
			newCrossingsCount := newCrossingsCount1 + newCrossingsCount2
			newOverlapsCount := newOverlapsCount1 + newOverlapsCount2
			newCloseOverlapsCount := newCloseOverlapsCount1 + newCloseOverlapsCount2
			newTouchingCount := newTouchingCount1 + newTouchingCount2

			if newCrossingsCount > oldCrossingsCount {
				continue
			}
			if newOverlapsCount > oldOverlapsCount {
				continue
			}

			if newCloseOverlapsCount > oldCloseOverlapsCount {
				continue
			}
			if newTouchingCount > oldTouchingCount {
				continue
			}

			// commit
			g.Edges[ei].Route = append(append(
				e.Route[:i],
				newCorner,
			),
				e.Route[i+3:]...,
			)
			collisionIndex.updateEdge(g.Edges[ei])
			break
		}
	}
}

type shapePadding struct {
	top, left, bottom, right int
}

// parse out values from elk padding string. e.g. "[top=50,left=50,bottom=50,right=50]"
func parsePadding(in string) shapePadding {
	reTop := regexp.MustCompile(`top=(\d+)`)
	reLeft := regexp.MustCompile(`left=(\d+)`)
	reBottom := regexp.MustCompile(`bottom=(\d+)`)
	reRight := regexp.MustCompile(`right=(\d+)`)

	padding := shapePadding{}

	submatches := reTop.FindStringSubmatch(in)
	if len(submatches) == 2 {
		i, err := strconv.ParseInt(submatches[1], 10, 64)
		if err == nil {
			padding.top = int(i)
		}
	}

	submatches = reLeft.FindStringSubmatch(in)
	if len(submatches) == 2 {
		i, err := strconv.ParseInt(submatches[1], 10, 64)
		if err == nil {
			padding.left = int(i)
		}
	}

	submatches = reBottom.FindStringSubmatch(in)
	if len(submatches) == 2 {
		i, err := strconv.ParseInt(submatches[1], 10, 64)
		if err == nil {
			padding.bottom = int(i)
		}
	}

	submatches = reRight.FindStringSubmatch(in)
	i, err := strconv.ParseInt(submatches[1], 10, 64)
	if len(submatches) == 2 {
		if err == nil {
			padding.right = int(i)
		}
	}

	return padding
}

func (padding shapePadding) String() string {
	return fmt.Sprintf("[top=%d,left=%d,bottom=%d,right=%d]", padding.top, padding.left, padding.bottom, padding.right)
}

func adjustPadding(obj *d2graph.Object, width, height float64, padding shapePadding) shapePadding {
	if !obj.IsContainer() {
		return padding
	}

	// compute extra space padding for label/icon
	var extraTop, extraBottom, extraLeft, extraRight int
	if obj.HasLabel() && obj.LabelPosition != nil {
		labelHeight := obj.LabelDimensions.Height + 2*label.PADDING
		labelWidth := obj.LabelDimensions.Width + 2*label.PADDING
		switch label.FromString(*obj.LabelPosition) {
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			// Note: for corners we only add height
			extraTop = labelHeight
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			extraBottom = labelHeight
		case label.InsideMiddleLeft:
			extraLeft = labelWidth
		case label.InsideMiddleRight:
			extraRight = labelWidth
		}
	}
	if obj.HasIcon() && obj.IconPosition != nil {
		iconSize := d2target.MAX_ICON_SIZE + 2*label.PADDING
		switch label.FromString(*obj.IconPosition) {
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			extraTop = go2.Max(extraTop, iconSize)
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			extraBottom = go2.Max(extraBottom, iconSize)
		case label.InsideMiddleLeft:
			extraLeft = go2.Max(extraLeft, iconSize)
		case label.InsideMiddleRight:
			extraRight = go2.Max(extraRight, iconSize)
		}
	}

	maxChildWidth, maxChildHeight := math.Inf(-1), math.Inf(-1)
	for _, c := range obj.ChildrenArray {
		if c.Width > maxChildWidth {
			maxChildWidth = c.Width
		}
		if c.Height > maxChildHeight {
			maxChildHeight = c.Height
		}
	}
	// We don't know exactly what the shape dimensions will be after layout, but for more accurate innerBox dimensions,
	// we add the maxChildWidth and maxChildHeight with computed additions for the innerBox calculation
	width += maxChildWidth + float64(extraLeft+extraRight)
	height += maxChildHeight + float64(extraTop+extraBottom)
	contentBox := geo.NewBox(geo.NewPoint(0, 0), width, height)
	shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[obj.Shape.Value]
	s := shape.NewShape(shapeType, contentBox)
	innerBox := s.GetInnerBox()

	// If the shape inner box + label/icon height becomes greater than the default padding, we want to use that
	//
	// ┌OUTER───────────────────────────┬────────────────────────────────────────────┐
	// │                                │                                            │
	// │  ┌INNER──────── ┬ ─────────────│───────────────────────────────────────┐    │
	// │  │              │Label Padding │                                       │    │
	// │  │      ┌LABEL─ ┴ ─────────────│───────┐┬             ┌ICON── ┬ ────┐  │    │
	// │  │      │                      │       ││             │       │     │  │    │
	// │  │      │                      │       ││Label Height │   Icon│     │  │    │
	// │  │      │                      │       ││             │ Height│     │  │    │
	// │  │      └──────────────────────│───────┘┴             │       │     │  │    │
	// │  │                             │                      └────── ┴ ────┘  │    │
	// │  │                             │                                       │    │
	// │  │                             ┴Default ELK Padding                    │    │
	// │  │   ┌CHILD────────────────────────────────────────────────────────┐   │    │
	// │  │   │                                                             │   │    │
	// │  │   │                                                             │   │    │
	// │  │   │                                                             │   │    │
	// │  │   └─────────────────────────────────────────────────────────────┘   │    │
	// │  │                                                                     │    │
	// │  └─────────────────────────────────────────────────────────────────────┘    │
	// │                                                                             │
	// └─────────────────────────────────────────────────────────────────────────────┘

	// estimated shape innerBox padding
	innerTop := int(math.Ceil(innerBox.TopLeft.Y))
	innerBottom := int(math.Ceil(height - (innerBox.TopLeft.Y + innerBox.Height)))
	innerLeft := int(math.Ceil(innerBox.TopLeft.X))
	innerRight := int(math.Ceil(width - (innerBox.TopLeft.X + innerBox.Width)))

	padding.top = go2.Max(padding.top, innerTop+extraTop)
	padding.bottom = go2.Max(padding.bottom, innerBottom+extraBottom)
	padding.left = go2.Max(padding.left, innerLeft+extraLeft)
	padding.right = go2.Max(padding.right, innerRight+extraRight)

	return padding
}

func positionLabelsIcons(obj *d2graph.Object) {
	if obj.Icon != nil && obj.IconPosition == nil {
		if len(obj.ChildrenArray) > 0 {
			obj.IconPosition = go2.Pointer(label.InsideTopLeft.String())
			if obj.LabelPosition == nil {
				obj.LabelPosition = go2.Pointer(label.InsideTopRight.String())
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
			obj.LabelPosition = go2.Pointer(label.InsideTopCenter.String())
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
