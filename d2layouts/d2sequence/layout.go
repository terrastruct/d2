package d2sequence

import (
	"context"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

// Layout identifies and performs layout on sequence diagrams within a graph
// first, it traverses the graph from Root and once it finds an object of shape `sequence_diagram`
// it replaces the children with a rectangle with id `sequence_diagram`, collects all edges coming to this node and
// flag the edges to be removed. Then, using the children and the edges, it lays out the sequence diagram and
// sets the dimensions of the rectangle `sequence_diagram` rectangle.
// Once all nodes were processed, it continues to run the layout engine without the sequence diagram nodes and edges.
// Then it restores all objects with their proper layout engine and sequence diagram positions
func Layout(ctx context.Context, g *d2graph.Graph, layout func(ctx context.Context, g *d2graph.Graph) error) error {
	// flag objects to keep to avoid having to flag all descendants of sequence diagram to be removed
	objectsToRemove := make(map[*d2graph.Object]struct{})
	// edges flagged to be removed (these are internal edges of the sequence diagrams)
	edgesToRemove := make(map[*d2graph.Edge]struct{})
	// store the sequence diagram related to a given node
	sequenceDiagrams := make(map[string]*sequenceDiagram)

	// starts in root and traverses all descendants
	queue := make([]*d2graph.Object, 1, len(g.Objects))
	queue[0] = g.Root
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]
		if obj.Attributes.Shape.Value != d2target.ShapeSequenceDiagram {
			// only move to children if the parent is not a sequence diagram
			queue = append(queue, obj.ChildrenArray...)
			continue
		}

		sd := layoutSequenceDiagram(g, obj)
		obj.Children = make(map[string]*d2graph.Object)
		obj.ChildrenArray = nil
		sequenceDiagrams[obj.AbsID()] = sd

		// flag objects and edges to remove
		for _, edge := range sd.messages {
			edgesToRemove[edge] = struct{}{}
		}
		for _, obj := range sd.actors {
			objectsToRemove[obj] = struct{}{}
		}
		for _, obj := range sd.spans {
			objectsToRemove[obj] = struct{}{}
		}
	}

	// removes the edges
	layoutEdges := make([]*d2graph.Edge, 0, len(g.Edges)-len(edgesToRemove))
	for _, edge := range g.Edges {
		if _, exists := edgesToRemove[edge]; !exists {
			layoutEdges = append(layoutEdges, edge)
		}
	}
	g.Edges = layoutEdges

	// done this way (by flagging objects) instead of appending while going through the `queue`
	// because appending in that order would change the order of g.Objects which
	// could lead to layout changes (as the order of the objects might be important for the underlying engine)
	layoutObjects := make([]*d2graph.Object, 0, len(objectsToRemove))
	for _, obj := range g.Objects {
		if _, exists := objectsToRemove[obj]; !exists {
			layoutObjects = append(layoutObjects, obj)
		}
	}
	g.Objects = layoutObjects

	if g.Root.Attributes.Shape.Value == d2target.ShapeSequenceDiagram {
		// don't need to run the layout engine if the root is a sequence diagram
		g.Root.ChildrenArray[0].TopLeft = geo.NewPoint(0, 0)
	} else if err := layout(ctx, g); err != nil {
		return err
	}

	// restores objects
	for _, obj := range g.Objects {
		if _, exists := sequenceDiagrams[obj.AbsID()]; !exists {
			continue
		}
		obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
		sd := sequenceDiagrams[obj.AbsID()]

		// shift the sequence diagrams as they are always placed at (0, 0)
		sd.shift(obj.TopLeft)

		// restore children
		obj.Children = make(map[string]*d2graph.Object)
		for _, child := range sd.actors {
			obj.Children[child.ID] = child
		}
		obj.ChildrenArray = sd.actors

		// add lifeline edges
		g.Edges = append(g.Edges, sequenceDiagrams[obj.AbsID()].lifelines...)
		g.Edges = append(g.Edges, sequenceDiagrams[obj.AbsID()].messages...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].actors...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].spans...)
	}

	return nil
}

func layoutSequenceDiagram(g *d2graph.Graph, obj *d2graph.Object) *sequenceDiagram {
	// find the edges that belong to this sequence diagram
	var edges []*d2graph.Edge
	for _, edge := range g.Edges {
		// both Src and Dst must be inside the sequence diagram
		if strings.HasPrefix(edge.Src.AbsID(), obj.AbsID()) && strings.HasPrefix(edge.Dst.AbsID(), obj.AbsID()) {
			edges = append(edges, edge)
		}
	}

	sd := newSequenceDiagram(obj.ChildrenArray, edges)
	sd.layout()
	obj.Width = sd.getWidth()
	obj.Height = sd.getHeight()
	return sd
}
