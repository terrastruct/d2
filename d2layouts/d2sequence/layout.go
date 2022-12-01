package d2sequence

import (
	"context"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
)

// Layout identifies and layouts sequence diagrams within a graph
// first, it traverses the graph from Root and once it finds an object of shape `sequence_diagram`
// it replaces the children with a rectangle with id `sequence_diagram`, collects all edges coming to this node and
// flag the edges to be removed. Then, using the children and the edges, it lays out the sequence diagram and
// sets the dimensions of the rectangle `sequence_diagram` rectangle.
// Once all nodes were processed, it continues to run the layout engine without the sequence diagram nodes and edges.
// Then it restores all objects with their proper layout engine and sequence diagram positions
func Layout(ctx context.Context, g *d2graph.Graph, layout func(ctx context.Context, g *d2graph.Graph) error) error {
	// keeps the current graph state
	oldObjects := g.Objects
	oldEdges := g.Edges

	// new graph objects
	g.Objects = make([]*d2graph.Object, 0, len(g.Objects))
	// edges flagged to be removed (these are internal edges of the sequence diagrams)
	edgesToRemove := make(map[*d2graph.Edge]struct{})
	// store the sequence diagram related to a given node
	sequenceDiagrams := make(map[*d2graph.Object]*sequenceDiagram)
	// keeps the reference of the children of a given node
	objChildrenArray := make(map[*d2graph.Object][]*d2graph.Object)

	// goes from root and travers all descendants
	queue := make([]*d2graph.Object, 1, len(oldObjects))
	queue[0] = g.Root
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]

		// root is not part of g.Objects, so we can't add it here
		if obj != g.Root {
			g.Objects = append(g.Objects, obj)
		}
		if obj.Attributes.Shape.Value == d2target.ShapeSequenceDiagram {
			// TODO: should update obj.References too?

			// clean current children and keep a backup to restore them later
			obj.Children = make(map[string]*d2graph.Object)
			objChildrenArray[obj] = obj.ChildrenArray
			obj.ChildrenArray = nil
			// creates a mock rectangle so that layout considers this size
			sdMock := obj.EnsureChild([]string{"sequence_diagram"})
			sdMock.Attributes.Shape.Value = d2target.ShapeRectangle
			sdMock.Attributes.Label.Value = ""

			// find the edges that belong to this sequence diagra
			var edges []*d2graph.Edge
			for _, edge := range g.Edges {
				// both Src and Dst must be inside the sequence diagram
				if strings.HasPrefix(edge.Src.AbsID(), obj.AbsID()) && strings.HasPrefix(edge.Dst.AbsID(), obj.AbsID()) {
					edgesToRemove[edge] = struct{}{}
					edges = append(edges, edge)
				}
			}

			sd := newSequenceDiagram(objChildrenArray[obj], edges)
			sd.layout()
			sdMock.Box = geo.NewBox(nil, sd.getWidth(), sd.getHeight())
			sequenceDiagrams[obj] = sd
		} else {
			// only move to children if the parent is not a sequence diagram
			queue = append(queue, obj.ChildrenArray...)
		}
	}

	// removes the edges
	newEdges := make([]*d2graph.Edge, 0, len(g.Edges)-len(edgesToRemove))
	for _, edge := range g.Edges {
		if _, exists := edgesToRemove[edge]; !exists {
			newEdges = append(newEdges, edge)
		}
	}

	// objToIndex := make(map[*d2graph.Object]int)
	// for i, obj := range oldObjects {
	// 	objToIndex[obj] = i
	// }

	// sort.Slice(g.Objects, func(i, j int) bool {
	// 	return objToIndex[g.Objects[i]] < objToIndex[g.Objects[j]]
	// })

	g.Edges = newEdges
	if err := layout(ctx, g); err != nil {
		return err
	}

	// restores objects & edges
	g.Edges = oldEdges
	g.Objects = oldObjects

	for obj, children := range objChildrenArray {
		// shift the sequence diagrams as they are always placed at (0, 0)
		sdMock := obj.ChildrenArray[0]
		sequenceDiagrams[obj].shift(sdMock.TopLeft)

		// restore children
		obj.Children = make(map[string]*d2graph.Object)
		for _, child := range children {
			obj.Children[child.ID] = child
		}
		obj.ChildrenArray = children

		// add lifeline edges
		g.Edges = append(g.Edges, sequenceDiagrams[obj].lifelines...)
	}

	return nil
}
