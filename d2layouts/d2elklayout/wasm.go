//go:build js && wasm

package d2elklayout

import (
	"context"
	"fmt"
	"math"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
	"oss.terrastruct.com/util-go/xdefer"
)

// This is mostly copy paste from Layout until elk.layout step
func ConvertGraph(ctx context.Context, g *d2graph.Graph, opts *ConfigurableOpts) (_ *ELKGraph, err error) {
	if opts == nil {
		opts = &DefaultOpts
	}
	defer xdefer.Errorf(&err, "failed to ELK layout")

	elkGraph := &ELKGraph{
		ID: "",
		LayoutOptions: &elkOpts{
			Thoroughness:                 8,
			EdgeEdgeBetweenLayersSpacing: 50,
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
		// +5 for a tiny bit of padding
		elkGraph.LayoutOptions.ConfigurableOpts.SelfLoopSpacing = go2.Max(elkGraph.LayoutOptions.ConfigurableOpts.SelfLoopSpacing, childrenMaxSelfLoop(g.Root, g.Root.Direction.Value == "down" || g.Root.Direction.Value == "" || g.Root.Direction.Value == "up")/2+5)
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
		if g.ASCII {
			labelPadding = 1.
		}
		margin, _ := obj.SpacingOpt(labelPadding, labelPadding, false)
		width := margin.Left + obj.Width + margin.Right
		height := margin.Top + obj.Height + margin.Bottom
		adjustments[obj] = margin

		n := &ELKNode{
			ID:     obj.AbsID(),
			Width:  width,
			Height: height,
		}

		if len(obj.ChildrenArray) > 0 {
			n.LayoutOptions = &elkOpts{
				ForceNodeModelOrder:          true,
				Thoroughness:                 8,
				EdgeEdgeBetweenLayersSpacing: 50,
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
				n.LayoutOptions.ConfigurableOpts.SelfLoopSpacing = go2.Max(n.LayoutOptions.ConfigurableOpts.SelfLoopSpacing, childrenMaxSelfLoop(obj, g.Root.Direction.Value == "down" || g.Root.Direction.Value == "" || g.Root.Direction.Value == "up")/2+5)
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
	return elkGraph, nil
}
