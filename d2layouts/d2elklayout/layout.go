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

	"rogchap.com/v8go"

	"oss.terrastruct.com/xdefer"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/go2"
	"oss.terrastruct.com/d2/lib/label"
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
	Text   string  `json:"text"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
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
	ID            string           `json:"id"`
	LayoutOptions ELKLayoutOptions `json:"layoutOptions"`
	Children      []*ELKNode       `json:"children,omitempty"`
	Edges         []*ELKEdge       `json:"edges,omitempty"`
}

type ELKLayoutOptions struct {
	Algorithm         string  `json:"elk.algorithm,omitempty"`
	HierarchyHandling string  `json:"elk.hierarchyHandling,omitempty"`
	NodeSpacing       float64 `json:"spacing.nodeNodeBetweenLayers,omitempty"`
	Padding           string  `json:"elk.padding,omitempty"`
	EdgeNodeSpacing   float64 `json:"spacing.edgeNodeBetweenLayers,omitempty"`
	Direction         string  `json:"elk.direction"`
}

func Layout(ctx context.Context, g *d2graph.Graph) (err error) {
	defer xdefer.Errorf(&err, "failed to ELK layout")

	iso := v8go.NewIsolate()
	global := v8go.NewObjectTemplate(iso)

	// setTimeout is not defined by in v8go global
	// As far as I can tell, it's not actually useful in elk.js
	// The comment above one of the instances it's used is "why do we even need this"
	// and it's a timeout of 0.
	// If weird things happen though, look here.
	setTimeout := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		args := info.Args()
		fn, _ := args[0].AsFunction()
		receiver := v8go.NewObjectTemplate(iso)
		s, _ := receiver.NewInstance(info.Context())
		fn.Call(s)

		return s.Value
	})
	global.Set("setTimeout", setTimeout, v8go.ReadOnly)

	v8ctx := v8go.NewContext(iso, global)
	if _, err := v8ctx.RunScript(elkJS, "elk.js"); err != nil {
		return err
	}
	if _, err := v8ctx.RunScript(setupJS, "setup.js"); err != nil {
		return err
	}

	elkGraph := &ELKGraph{
		ID: "root",
		LayoutOptions: ELKLayoutOptions{
			Algorithm:         "layered",
			HierarchyHandling: "INCLUDE_CHILDREN",
			NodeSpacing:       100.0,
			EdgeNodeSpacing:   50.0,
			Direction:         "DOWN",
		},
	}
	if g.Root.Attributes.Orientation.Value == "horizontal" {
		elkGraph.LayoutOptions.Direction = "RIGHT"
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
		n := &ELKNode{
			ID:     obj.AbsID(),
			Width:  obj.Width,
			Height: obj.Height,
		}

		if len(obj.ChildrenArray) > 0 {
			n.LayoutOptions = &ELKLayoutOptions{
				Padding: "[top=75,left=75,bottom=75,right=75]",
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

	if _, err := v8ctx.RunScript(loadScript, "load.js"); err != nil {
		return err
	}

	val, err := v8ctx.RunScript(`elk.layout(graph)
.then(s => s)
.catch(s => s)
`, "layout.js")

	if err != nil {
		return err
	}

	promise, err := val.AsPromise()
	if err != nil {
		return err
	}

	for promise.State() == v8go.Pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		continue
	}

	jsonOut, err := promise.Result().MarshalJSON()
	if err != nil {
		return err
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
		obj.TopLeft = geo.NewPoint(math.Round(parentX+n.X), math.Round(parentY+n.Y))
		obj.Width = n.Width
		obj.Height = n.Height

		if obj.LabelWidth != nil && obj.LabelHeight != nil {
			if len(obj.ChildrenArray) > 0 {
				obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			} else if obj.Attributes.Shape.Value == d2target.ShapeImage {
				obj.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
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

		if edge.Attributes.Label.Value != "" {
			edge.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
		}

		edge.Route = points
	}

	return nil
}
