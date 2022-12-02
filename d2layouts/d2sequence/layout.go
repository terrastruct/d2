package d2sequence

import (
	"context"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
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
	objectsToKeep := make(map[*d2graph.Object]struct{})
	// edges flagged to be removed (these are internal edges of the sequence diagrams)
	edgesToRemove := make(map[*d2graph.Edge]struct{})
	// store the sequence diagram related to a given node
	sequenceDiagrams := make(map[string]*sequenceDiagram)
	// keeps the reference of the children of a given node
	childrenReplacement := make(map[string][]*d2graph.Object)

	// goes from root and travers all descendants
	queue := make([]*d2graph.Object, 1, len(g.Objects))
	queue[0] = g.Root
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]

		objectsToKeep[obj] = struct{}{}
		if obj.Attributes.Shape.Value == d2target.ShapeSequenceDiagram {
			// TODO: should update obj.References too?

			// clean current children and keep a backup to restore them later
			obj.Children = make(map[string]*d2graph.Object)
			children := obj.ChildrenArray
			obj.ChildrenArray = nil
			// creates a mock rectangle so that layout considers this size
			sdMock := obj.EnsureChild([]string{"sequence_diagram"})
			sdMock.Attributes.Shape.Value = d2target.ShapeRectangle
			sdMock.Attributes.Label.Value = ""
			objectsToKeep[sdMock] = struct{}{}

			// find the edges that belong to this sequence diagra
			var edges []*d2graph.Edge
			for _, edge := range g.Edges {
				// both Src and Dst must be inside the sequence diagram
				if strings.HasPrefix(edge.Src.AbsID(), obj.AbsID()) && strings.HasPrefix(edge.Dst.AbsID(), obj.AbsID()) {
					edgesToRemove[edge] = struct{}{}
					edges = append(edges, edge)
				}
			}

			sd := newSequenceDiagram(children, edges)
			sd.layout()
			sdMock.Box = geo.NewBox(nil, sd.getWidth(), sd.getHeight())
			sequenceDiagrams[sdMock.AbsID()] = sd
			childrenReplacement[sdMock.AbsID()] = children
		} else {
			// only move to children if the parent is not a sequence diagram
			queue = append(queue, obj.ChildrenArray...)
		}
	}

	if len(sequenceDiagrams) == 0 {
		return layout(ctx, g)
	}

	// removes the edges
	layoutEdges := make([]*d2graph.Edge, 0, len(g.Edges)-len(edgesToRemove))
	sequenceDiagramEdges := make([]*d2graph.Edge, 0, len(edgesToRemove))
	for _, edge := range g.Edges {
		if _, exists := edgesToRemove[edge]; !exists {
			layoutEdges = append(layoutEdges, edge)
		} else {
			sequenceDiagramEdges = append(sequenceDiagramEdges, edge)
		}
	}
	g.Edges = layoutEdges

	// done this way (by flagging objects) instead of appending to `queue`
	// because appending in that order would change the order of g.Objects which
	// could lead to layout changes (as the order of the objects might be important for the underlying engine)
	layoutObjects := make([]*d2graph.Object, 0, len(objectsToKeep))
	sequenceDiagramObjects := make([]*d2graph.Object, 0, len(g.Objects)-len(objectsToKeep))
	for _, obj := range g.Objects {
		if _, exists := objectsToKeep[obj]; exists {
			layoutObjects = append(layoutObjects, obj)
		} else {
			sequenceDiagramObjects = append(sequenceDiagramObjects, obj)
		}
	}
	g.Objects = layoutObjects

	if g.Root.Attributes.Shape.Value == d2target.ShapeSequenceDiagram {
		// don't need to run the layout engine if the root is a sequence diagram
		g.Root.ChildrenArray[0].TopLeft = geo.NewPoint(0, 0)
	} else if err := layout(ctx, g); err != nil {
		return err
	}

	g.Edges = append(g.Edges, sequenceDiagramEdges...)

	// restores objects
	// it must be this way because the objects pointer reference is lost when calling layout engines compiled in binaries
	allObjects := sequenceDiagramObjects
	for _, obj := range g.Objects {
		if _, exists := sequenceDiagrams[obj.AbsID()]; !exists {
			allObjects = append(allObjects, obj)
			continue
		}
		// we don't want `sdMock` in `g.Objects` because they shouldn't be rendered

		// just to make it easier to read
		sdMock := obj
		parent := obj.Parent

		// shift the sequence diagrams as they are always placed at (0, 0)
		sequenceDiagrams[sdMock.AbsID()].shift(sdMock.TopLeft)

		// restore children
		parent.Children = make(map[string]*d2graph.Object)
		for _, child := range childrenReplacement[sdMock.AbsID()] {
			parent.Children[child.ID] = child
		}
		parent.ChildrenArray = childrenReplacement[sdMock.AbsID()]

		// add lifeline edges
		g.Edges = append(g.Edges, sequenceDiagrams[sdMock.AbsID()].lifelines...)
	}
	g.Objects = allObjects

	return nil
}
