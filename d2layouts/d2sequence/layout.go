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
	objectsToKeep := make(map[*d2graph.Object]struct{})
	// edges flagged to be removed (these are internal edges of the sequence diagrams)
	edgesToRemove := make(map[*d2graph.Edge]struct{})
	// store the sequence diagram related to a given node
	sequenceDiagrams := make(map[string]*sequenceDiagram)
	// keeps the reference of the children of a given node
	childrenReplacement := make(map[string][]*d2graph.Object)

	// starts in root and traverses all descendants
	queue := make([]*d2graph.Object, 1, len(g.Objects))
	queue[0] = g.Root
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]

		objectsToKeep[obj] = struct{}{}
		if obj.Attributes.Shape.Value == d2target.ShapeSequenceDiagram {
			// clean current children and keep a backup to restore them later
			obj.Children = make(map[string]*d2graph.Object)
			children := obj.ChildrenArray
			obj.ChildrenArray = nil

			// find the edges that belong to this sequence diagram
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
			obj.Width = sd.getWidth()
			obj.Height = sd.getHeight()
			sequenceDiagrams[obj.AbsID()] = sd
			childrenReplacement[obj.AbsID()] = children
		} else {
			// only move to children if the parent is not a sequence diagram
			queue = append(queue, obj.ChildrenArray...)
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
	layoutObjects := make([]*d2graph.Object, 0, len(objectsToKeep))
	for _, obj := range g.Objects {
		if _, exists := objectsToKeep[obj]; exists {
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
		// shift the sequence diagrams as they are always placed at (0, 0)
		sequenceDiagrams[obj.AbsID()].shift(obj.TopLeft)

		// restore children
		obj.Children = make(map[string]*d2graph.Object)
		for _, child := range childrenReplacement[obj.AbsID()] {
			obj.Children[child.ID] = child
		}
		obj.ChildrenArray = childrenReplacement[obj.AbsID()]

		// add lifeline edges
		g.Edges = append(g.Edges, sequenceDiagrams[obj.AbsID()].lifelines...)
		g.Edges = append(g.Edges, sequenceDiagrams[obj.AbsID()].messages...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].actors...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].spans...)
	}

	return nil
}
