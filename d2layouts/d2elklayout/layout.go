// d2elklayout is a wrapper around the Javascript port of ELK.
//
// Coordinates are relative to parents.
// See https://www.eclipse.org/elk/documentation/tooldevelopers/graphdatastructure/coordinatesystem.html
package d2elklayout

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"oss.terrastruct.com/util-go/xdefer"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/jsrunner"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

//go:embed setup.js
var setupJS string

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
	SelfLoopSpacing int    `json:"elk.spacing.nodeSelfLoop"`
}

var DefaultOpts = ConfigurableOpts{
	Algorithm:       "layered",
	NodeSpacing:     70.0,
	Padding:         "[top=50,left=50,bottom=50,right=50]",
	EdgeNodeSpacing: 40.0,
	SelfLoopSpacing: 50.0,
}

var port_spacing = 40.
var edge_node_spacing = 40
var edge_edge_between_layers_spacing = 50

type elkOpts struct {
	EdgeNode                     int       `json:"elk.spacing.edgeNode,omitempty"`
	FixedAlignment               string    `json:"elk.layered.nodePlacement.bk.fixedAlignment,omitempty"`
	Thoroughness                 int       `json:"elk.layered.thoroughness,omitempty"`
	EdgeEdgeBetweenLayersSpacing int       `json:"elk.layered.spacing.edgeEdgeBetweenLayers,omitempty"`
	Direction                    Direction `json:"elk.direction"`
	HierarchyHandling            string    `json:"elk.hierarchyHandling,omitempty"`
	InlineEdgeLabels             bool      `json:"elk.edgeLabels.inline,omitempty"`
	ForceNodeModelOrder          bool      `json:"elk.layered.crossingMinimization.forceNodeModelOrder,omitempty"`
	ConsiderModelOrder           string    `json:"elk.layered.considerModelOrder.strategy,omitempty"`
	CycleBreakingStrategy        string    `json:"elk.layered.cycleBreaking.strategy,omitempty"`

	SelfLoopDistribution string `json:"elk.layered.edgeRouting.selfLoopDistribution,omitempty"`

	NodeSizeConstraints string `json:"elk.nodeSize.constraints,omitempty"`
	ContentAlignment    string `json:"elk.contentAlignment,omitempty"`
	NodeSizeMinimum     string `json:"elk.nodeSize.minimum,omitempty"`

	PortSide        PortSide `json:"elk.port.side,omitempty"`
	PortConstraints string   `json:"elk.portConstraints,omitempty"`

	ConfigurableOpts
}

func DefaultLayout(ctx context.Context, g *d2graph.Graph) (err error) {
	return Layout(ctx, g, nil)
}

func Layout(ctx context.Context, g *d2graph.Graph, opts *ConfigurableOpts) (err error) {
	if opts == nil {
		opts = &DefaultOpts
	}

	// Override all spacing variables for ASCII mode - use larger values for orthogonal routing
	if g.ASCII {
		// Create a copy of opts to avoid modifying the default
		asciiOpts := *opts
		asciiOpts.Padding = "[top=2,left=3,bottom=3,right=3]"
		asciiOpts.NodeSpacing = 3
		asciiOpts.EdgeNodeSpacing = 2
		asciiOpts.SelfLoopSpacing = 5
		opts = &asciiOpts
		// Override global spacing variables for ASCII mode
		port_spacing = 3.
		edge_node_spacing = 3
		edge_edge_between_layers_spacing = 1
	}

	defer xdefer.Errorf(&err, "failed to ELK layout")

	runner := jsrunner.NewJSRunner()

	// Load ELK for both Goja engine only
	// WASM/JS loads it natively
	if runner.Engine() == jsrunner.Goja {
		console := runner.NewObject()
		if err := runner.Set("console", console); err != nil {
			return err
		}
		if _, err := runner.RunString(elkJS); err != nil {
			return err
		}
		if _, err := runner.RunString(setupJS); err != nil {
			return err
		}
	}

	elkGraph := &ELKGraph{
		ID: "",
		LayoutOptions: &elkOpts{
			Thoroughness:                 8,
			EdgeEdgeBetweenLayersSpacing: edge_edge_between_layers_spacing,
			EdgeNode:                     edge_node_spacing,
			HierarchyHandling:            "INCLUDE_CHILDREN",
			FixedAlignment:               "BALANCED",
			ConsiderModelOrder:           "NODES_AND_EDGES",
			CycleBreakingStrategy:        "GREEDY_MODEL_ORDER",
			NodeSizeConstraints:          "MINIMUM_SIZE",
			ContentAlignment:             "H_CENTER V_CENTER",
			ConfigurableOpts: ConfigurableOpts{
				Algorithm:       opts.Algorithm,
				NodeSpacing:     opts.NodeSpacing,
				EdgeNodeSpacing: opts.EdgeNodeSpacing,
				SelfLoopSpacing: opts.SelfLoopSpacing,
			},
		},
	}
	if elkGraph.LayoutOptions.ConfigurableOpts.SelfLoopSpacing == DefaultOpts.SelfLoopSpacing {
		// Use smaller padding for ASCII mode
		selfLoopPadding := 5
		if g.ASCII {
			selfLoopPadding = 1
		}
		elkGraph.LayoutOptions.ConfigurableOpts.SelfLoopSpacing = go2.Max(elkGraph.LayoutOptions.ConfigurableOpts.SelfLoopSpacing, childrenMaxSelfLoop(g.Root, g.Root.Direction.Value == "down" || g.Root.Direction.Value == "" || g.Root.Direction.Value == "up")/2+selfLoopPadding)
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
		incoming := 0.
		outgoing := 0.
		for _, e := range g.Edges {
			if e.Src == obj {
				outgoing++
			}
			if e.Dst == obj {
				incoming++
			}
		}
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
			iconLabelPadding := label.PADDING
			if g.ASCII {
				iconLabelPadding = 1
			}
			obj.Height += float64(obj.LabelDimensions.Height + iconLabelPadding)
		}

		labelPadding := float64(label.PADDING)
		width := obj.Width
		height := obj.Height
		if g.ASCII {
			labelPadding = 1.
		} else {
			margin, _ := obj.SpacingOpt(labelPadding, labelPadding, false)
			width = margin.Left + obj.Width + margin.Right
			height = margin.Top + obj.Height + margin.Bottom
			adjustments[obj] = margin
		}

		n := &ELKNode{
			ID:     obj.AbsID(),
			Width:  width,
			Height: height,
		}

		if len(obj.ChildrenArray) > 0 {
			n.LayoutOptions = &elkOpts{
				ForceNodeModelOrder:          true,
				Thoroughness:                 8,
				EdgeEdgeBetweenLayersSpacing: edge_edge_between_layers_spacing,
				HierarchyHandling:            "INCLUDE_CHILDREN",
				FixedAlignment:               "BALANCED",
				EdgeNode:                     edge_node_spacing,
				ConsiderModelOrder:           "NODES_AND_EDGES",
				CycleBreakingStrategy:        "GREEDY_MODEL_ORDER",
				NodeSizeConstraints:          "MINIMUM_SIZE",
				ContentAlignment:             "H_CENTER V_CENTER",
				ConfigurableOpts: ConfigurableOpts{
					NodeSpacing:     opts.NodeSpacing,
					EdgeNodeSpacing: opts.EdgeNodeSpacing,
					SelfLoopSpacing: opts.SelfLoopSpacing,
					Padding:         opts.Padding,
				},
			}
			if n.LayoutOptions.ConfigurableOpts.SelfLoopSpacing == DefaultOpts.SelfLoopSpacing {
				selfLoopPadding := 5
				if g.ASCII {
					selfLoopPadding = 1
				}
				n.LayoutOptions.ConfigurableOpts.SelfLoopSpacing = go2.Max(n.LayoutOptions.ConfigurableOpts.SelfLoopSpacing, childrenMaxSelfLoop(obj, g.Root.Direction.Value == "down" || g.Root.Direction.Value == "" || g.Root.Direction.Value == "up")/2+selfLoopPadding)
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
			padding = adjustPadding(g, obj, width, height, padding)
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

	loadScript := fmt.Sprintf(`var graph = %s`, raw)
	if _, err := runner.RunString(loadScript); err != nil {
		return err
	}

	// Use synchronous layout function
	val, err := runner.RunString(`
		elkLayoutSync(graph);
		graph;
	`)
	if err != nil {
		return fmt.Errorf("elkLayoutSync failed: %v", err)
	}

	var jsonOut map[string]interface{}
	// The result should be the modified graph object directly
	if val != nil {
		// Convert JSValue to map
		resultStr, err := runner.RunString(`JSON.stringify(graph)`)
		if err != nil {
			return err
		}

		if err := json.Unmarshal([]byte(resultStr.String()), &jsonOut); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("ELK layout returned nil")
	}

	jsonBytes, err := json.Marshal(jsonOut)
	if err != nil {
		return err
	}

	err = json.Unmarshal(jsonBytes, &elkGraph)
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
				obj.ShiftDescendants(margin.Left/2, 0)
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
				obj.ShiftDescendants(-margin.Right/2, 0)
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
				obj.ShiftDescendants(0, margin.Top/2)
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
				obj.ShiftDescendants(0, -margin.Bottom/2)
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
		if !g.ASCII {
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
		}

		// Use ASCII-adjusted padding if in ASCII mode
		// padding := float64(label.PADDING)
		// if g.ASCII {
		//   padding = 1.0
		// }
		// startIndex, endIndex = edge.TraceToShape(points, startIndex, endIndex, padding)
		// points = points[startIndex : endIndex+1]

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

	return nil
}

func srcPortID(obj *d2graph.Object, column string) string {
	return fmt.Sprintf("%s.%s.src", obj.AbsID(), column)
}

func dstPortID(obj *d2graph.Object, column string) string {
	return fmt.Sprintf("%s.%s.dst", obj.AbsID(), column)
}

// deleteBends is a shim for ELK to delete unnecessary bends
// see https://github.com/terrastruct/d2/issues/1030
func deleteBends(g *d2graph.Graph) {
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
			var dx, dy float64
			if !g.ASCII {
				dx, dy = endpoint.GetModifierElementAdjustments()
			}

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

			oldIntersects := countObjectIntersects(g, e.Src, e.Dst, *oldSegment)
			newIntersects := countObjectIntersects(g, e.Src, e.Dst, *newSegment)

			if newIntersects > oldIntersects {
				continue
			}

			oldCrossingsCount, oldOverlapsCount, oldCloseOverlapsCount, oldTouchingCount := countEdgeIntersects(g, g.Edges[ei], *oldSegment)
			newCrossingsCount, newOverlapsCount, newCloseOverlapsCount, newTouchingCount := countEdgeIntersects(g, g.Edges[ei], *newSegment)

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
			oldIntersects := countObjectIntersects(g, e.Src, e.Dst, *oldS1) + countObjectIntersects(g, e.Src, e.Dst, *oldS2)
			newIntersects := countObjectIntersects(g, e.Src, e.Dst, *newS1) + countObjectIntersects(g, e.Src, e.Dst, *newS2)

			if newIntersects > oldIntersects {
				continue
			}

			oldCrossingsCount1, oldOverlapsCount1, oldCloseOverlapsCount1, oldTouchingCount1 := countEdgeIntersects(g, g.Edges[ei], *oldS1)
			oldCrossingsCount2, oldOverlapsCount2, oldCloseOverlapsCount2, oldTouchingCount2 := countEdgeIntersects(g, g.Edges[ei], *oldS2)
			oldCrossingsCount := oldCrossingsCount1 + oldCrossingsCount2
			oldOverlapsCount := oldOverlapsCount1 + oldOverlapsCount2
			oldCloseOverlapsCount := oldCloseOverlapsCount1 + oldCloseOverlapsCount2
			oldTouchingCount := oldTouchingCount1 + oldTouchingCount2

			newCrossingsCount1, newOverlapsCount1, newCloseOverlapsCount1, newTouchingCount1 := countEdgeIntersects(g, g.Edges[ei], *newS1)
			newCrossingsCount2, newOverlapsCount2, newCloseOverlapsCount2, newTouchingCount2 := countEdgeIntersects(g, g.Edges[ei], *newS2)
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
			break
		}
	}
}

func countObjectIntersects(g *d2graph.Graph, src, dst *d2graph.Object, s geo.Segment) int {
	count := 0
	for i, o := range g.Objects {
		if g.Objects[i] == src || g.Objects[i] == dst {
			continue
		}
		if o.Intersects(s, float64(edge_node_spacing)-1) {
			count++
		}
	}
	return count
}

// countEdgeIntersects counts both crossings AND getting too close to a parallel segment
func countEdgeIntersects(g *d2graph.Graph, sEdge *d2graph.Edge, s geo.Segment) (int, int, int, int) {
	isHorizontal := math.Ceil(s.Start.Y) == math.Ceil(s.End.Y)
	crossingsCount := 0
	overlapsCount := 0
	closeOverlapsCount := 0
	touchingCount := 0
	for i, e := range g.Edges {
		if g.Edges[i] == sEdge {
			continue
		}

		for i := 0; i < len(e.Route)-1; i++ {
			otherS := geo.NewSegment(e.Route[i], e.Route[i+1])
			otherIsHorizontal := math.Ceil(otherS.Start.Y) == math.Ceil(otherS.End.Y)
			if isHorizontal == otherIsHorizontal {
				if s.Overlaps(*otherS, !isHorizontal, 0.) {
					if isHorizontal {
						if math.Abs(s.Start.Y-otherS.Start.Y) < float64(edge_node_spacing)/2. {
							overlapsCount++
							if math.Abs(s.Start.Y-otherS.Start.Y) < float64(edge_node_spacing)/4. {
								closeOverlapsCount++
								if math.Abs(s.Start.Y-otherS.Start.Y) < 1. {
									touchingCount++
								}
							}
						}
					} else {
						if math.Abs(s.Start.X-otherS.Start.X) < float64(edge_node_spacing)/2. {
							overlapsCount++
							if math.Abs(s.Start.X-otherS.Start.X) < float64(edge_node_spacing)/4. {
								closeOverlapsCount++
								if math.Abs(s.Start.Y-otherS.Start.Y) < 1. {
									touchingCount++
								}
							}
						}
					}
				}
			} else {
				if s.Intersects(*otherS) {
					crossingsCount++
				}
			}
		}

	}
	return crossingsCount, overlapsCount, closeOverlapsCount, touchingCount
}

func childrenMaxSelfLoop(parent *d2graph.Object, isWidth bool) int {
	max := 0
	for _, ch := range parent.Children {
		for _, e := range parent.Graph.Edges {
			if e.Src == e.Dst && e.Src == ch && e.Label.Value != "" {
				if isWidth {
					max = go2.Max(max, e.LabelDimensions.Width)
				} else {
					max = go2.Max(max, e.LabelDimensions.Height)
				}
			}
		}
	}

	return max
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

func adjustPadding(g *d2graph.Graph, obj *d2graph.Object, width, height float64, padding shapePadding) shapePadding {
	if !obj.IsContainer() {
		return padding
	}

	// compute extra space padding for label/icon
	var extraTop, extraBottom, extraLeft, extraRight int
	if obj.HasLabel() && obj.LabelPosition != nil {
		labelPadding := 2 * label.PADDING
		if g.ASCII {
			labelPadding = 2
		}
		labelHeight := obj.LabelDimensions.Height + labelPadding
		labelWidth := obj.LabelDimensions.Width + labelPadding
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
		iconPadding := 2 * label.PADDING
		if g.ASCII {
			iconPadding = 2
		}
		iconSize := d2target.MAX_ICON_SIZE + iconPadding
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
