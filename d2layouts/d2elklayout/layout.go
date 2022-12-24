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
	"strings"

	"github.com/dop251/goja"

	"oss.terrastruct.com/util-go/xdefer"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

//go:embed elk.js
var elkJS string

//go:embed setup.js
var setupJS string

type ELKNode struct {
	ID            string            `json:"id"`
	X             float64           `json:"x"`
	Y             float64           `json:"y"`
	Width         float64           `json:"width"`
	Height        float64           `json:"height"`
	Children      []*ELKNode        `json:"children,omitempty"`
	Labels        []*ELKLabel       `json:"labels,omitempty"`
	LayoutOptions *ELKLayoutOptions `json:"layoutOptions,omitempty"`
}

type ELKLabel struct {
	Text          string            `json:"text"`
	X             float64           `json:"x"`
	Y             float64           `json:"y"`
	Width         float64           `json:"width"`
	Height        float64           `json:"height"`
	LayoutOptions *ELKLayoutOptions `json:"layoutOptions,omitempty"`
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
	ID            string            `json:"id"`
	LayoutOptions *ELKLayoutOptions `json:"layoutOptions"`
	Children      []*ELKNode        `json:"children,omitempty"`
	Edges         []*ELKEdge        `json:"edges,omitempty"`
}

type ELKLayoutOptions struct {
	Algorithm           string  `json:"elk.algorithm,omitempty"`
	HierarchyHandling   string  `json:"elk.hierarchyHandling,omitempty"`
	NodeSpacing         float64 `json:"spacing.nodeNodeBetweenLayers,omitempty"`
	Padding             string  `json:"elk.padding,omitempty"`
	EdgeNodeSpacing     float64 `json:"spacing.edgeNodeBetweenLayers,omitempty"`
	Direction           string  `json:"elk.direction"`
	SelfLoopSpacing     float64 `json:"elk.spacing.nodeSelfLoop"`
	InlineEdgeLabels    bool    `json:"elk.edgeLabels.inline,omitempty"`
	ConsiderModelOrder  string  `json:"elk.layered.considerModelOrder.strategy,omitempty"`
	ForceNodeModelOrder bool    `json:"elk.layered.crossingMinimization.forceNodeModelOrder,omitempty"`
}

func Layout(ctx context.Context, g *d2graph.Graph) (err error) {
	defer xdefer.Errorf(&err, "failed to ELK layout")

	vm := goja.New()

	console := vm.NewObject()
	if err := vm.Set("console", console); err != nil {
		return err
	}

	if _, err := vm.RunString(elkJS); err != nil {
		return err
	}
	if _, err := vm.RunString(setupJS); err != nil {
		return err
	}

	elkGraph := &ELKGraph{
		ID: "root",
		LayoutOptions: &ELKLayoutOptions{
			Algorithm:          "layered",
			HierarchyHandling:  "INCLUDE_CHILDREN",
			NodeSpacing:        100.0,
			EdgeNodeSpacing:    50.0,
			SelfLoopSpacing:    50.0,
			ConsiderModelOrder: "NODES_AND_EDGES",
		},
	}
	switch g.Root.Attributes.Direction.Value {
	case "down":
		elkGraph.LayoutOptions.Direction = "DOWN"
	case "up":
		elkGraph.LayoutOptions.Direction = "UP"
	case "right":
		elkGraph.LayoutOptions.Direction = "RIGHT"
	case "left":
		elkGraph.LayoutOptions.Direction = "LEFT"
	default:
		elkGraph.LayoutOptions.Direction = "DOWN"
	}

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
		height := obj.Height
		if obj.Attributes.Shape.Value == d2target.ShapeImage || obj.Attributes.Icon != nil {
			height += float64(*obj.LabelHeight) + label.PADDING
		}

		n := &ELKNode{
			ID:     obj.AbsID(),
			Width:  obj.Width,
			Height: height,
		}

		if len(obj.ChildrenArray) > 0 {
			n.LayoutOptions = &ELKLayoutOptions{
				Padding:             "[top=75,left=75,bottom=75,right=75]",
				ForceNodeModelOrder: true,
			}
		}

		if obj.LabelWidth != nil && obj.LabelHeight != nil {
			n.Labels = append(n.Labels, &ELKLabel{
				Text:   obj.Attributes.Label.Value,
				Width:  float64(*obj.LabelWidth),
				Height: float64(*obj.LabelHeight),
			})
		}

		if parent == g.Root {
			elkGraph.Children = append(elkGraph.Children, n)
		} else {
			elkNodes[parent].Children = append(elkNodes[parent].Children, n)
		}
		elkNodes[obj] = n
	})

	for _, edge := range g.Edges {
		e := &ELKEdge{
			ID:      edge.AbsID(),
			Sources: []string{edge.Src.AbsID()},
			Targets: []string{edge.Dst.AbsID()},
		}
		if edge.Attributes.Label.Value != "" {
			e.Labels = append(e.Labels, &ELKLabel{
				Text:   edge.Attributes.Label.Value,
				Width:  float64(edge.LabelDimensions.Width),
				Height: float64(edge.LabelDimensions.Height),
				LayoutOptions: &ELKLayoutOptions{
					InlineEdgeLabels: true,
				},
			})
		}
		elkGraph.Edges = append(elkGraph.Edges, e)
		elkEdges[edge] = e
	}

	raw, err := json.Marshal(elkGraph)
	if err != nil {
		return err
	}

	loadScript := fmt.Sprintf(`var graph = %s`, raw)

	if _, err := vm.RunString(loadScript); err != nil {
		return err
	}

	val, err := vm.RunString(`elk.layout(graph)
.then(s => s)
.catch(s => s)
`)

	if err != nil {
		return err
	}

	p := val.Export()
	if err != nil {
		return err
	}

	promise := p.(*goja.Promise)

	for promise.State() == goja.PromiseStatePending {
		if err := ctx.Err(); err != nil {
			return err
		}
		continue
	}

	jsonOut := promise.Result().Export().(map[string]interface{})

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
		obj.TopLeft = geo.NewPoint(math.Round(parentX+n.X), math.Round(parentY+n.Y))
		obj.Width = n.Width
		obj.Height = n.Height

		if obj.LabelWidth != nil && obj.LabelHeight != nil {
			if len(obj.ChildrenArray) > 0 {
				obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			} else if obj.Attributes.Shape.Value == d2target.ShapeImage {
				obj.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
				obj.Height -= float64(*obj.LabelHeight) + label.PADDING
			} else if obj.Attributes.Icon != nil {
				obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			} else {
				obj.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
			}
		}
		if obj.Attributes.Icon != nil {
			obj.IconPosition = go2.Pointer(string(label.InsideMiddleCenter))
		}

		byID[obj.AbsID()] = obj
	})

	for _, edge := range g.Edges {
		e := elkEdges[edge]

		parentX := 0.0
		parentY := 0.0
		if e.Container != "root" {
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

		startIndex, endIndex := 0, len(points)-1
		srcShape := shape.NewShape(d2target.DSL_SHAPE_TO_SHAPE_TYPE[strings.ToLower(edge.Src.Attributes.Shape.Value)], edge.Src.Box)
		dstShape := shape.NewShape(d2target.DSL_SHAPE_TO_SHAPE_TYPE[strings.ToLower(edge.Dst.Attributes.Shape.Value)], edge.Dst.Box)

		// trace the edge to the specific shape's border
		points[startIndex] = shape.TraceToShapeBorder(srcShape, points[startIndex], points[startIndex+1])
		points[endIndex] = shape.TraceToShapeBorder(dstShape, points[endIndex], points[endIndex-1])

		if edge.Attributes.Label.Value != "" {
			edge.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
		}

		edge.Route = points
	}

	return nil
}
