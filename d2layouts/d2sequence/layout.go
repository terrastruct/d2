package d2sequence

import (
	"context"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
)

// Layout runs the sequence diagram layout engine on objects of shape sequence_diagram
//
// 1. Run layout on sequence diagrams
// 2. Set the resulting dimensions to the main graph shape
func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) error {
	// used in layout code
	g.Root.Shape.Value = d2target.ShapeSequenceDiagram

	sd, err := layoutSequenceDiagram(g, g.Root)
	if err != nil {
		return err
	}
	g.Root.Box = geo.NewBox(nil, sd.getWidth()+GROUP_CONTAINER_PADDING*2, sd.getHeight()+GROUP_CONTAINER_PADDING*2)

	// the sequence diagram is the only layout engine if the whole diagram is
	// shape: sequence_diagram
	g.Root.TopLeft = geo.NewPoint(0, 0)

	obj := g.Root

	obj.LabelPosition = go2.Pointer(label.InsideTopCenter.String())

	// shift the sequence diagrams as they are always placed at (0, 0) with some padding
	sd.shift(
		geo.NewPoint(
			obj.TopLeft.X+GROUP_CONTAINER_PADDING,
			obj.TopLeft.Y+GROUP_CONTAINER_PADDING,
		),
	)

	obj.Children = make(map[string]*d2graph.Object)
	obj.ChildrenArray = make([]*d2graph.Object, 0)
	for _, child := range sd.actors {
		obj.Children[strings.ToLower(child.ID)] = child
		obj.ChildrenArray = append(obj.ChildrenArray, child)
	}
	for _, child := range sd.groups {
		if child.Parent.AbsID() == obj.AbsID() {
			obj.Children[strings.ToLower(child.ID)] = child
			obj.ChildrenArray = append(obj.ChildrenArray, child)
		}
	}

	g.Edges = append(g.Edges, sd.lifelines...)

	return nil
}

// layoutSequenceDiagram finds the edges inside the sequence diagram and performs the layout on the object descendants
func layoutSequenceDiagram(g *d2graph.Graph, obj *d2graph.Object) (*sequenceDiagram, error) {
	var edges []*d2graph.Edge
	for _, edge := range g.Edges {
		// both Src and Dst must be inside the sequence diagram
		if obj == g.Root || (strings.HasPrefix(edge.Src.AbsID(), obj.AbsID()+".") && strings.HasPrefix(edge.Dst.AbsID(), obj.AbsID()+".")) {
			edges = append(edges, edge)
		}
	}

	sd, err := newSequenceDiagram(obj.ChildrenArray, edges)
	if err != nil {
		return nil, err
	}
	err = sd.layout()
	return sd, err
}
