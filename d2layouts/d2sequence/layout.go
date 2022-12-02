package d2sequence

import (
	"context"
	"sort"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

// Layout identifies and performs layout on sequence diagrams within a graph
// first, it traverses the graph from Root and once it finds an object of shape `sequence_diagram`
// it removes all descendants, collects all edges inside this node and flag them to be removed.
// Then, using the descendants and the edges, it lays out the sequence diagram and sets the dimensions of the node.
// Once all nodes were processed, it continues to run the layout engine without the sequence diagram nodes and edges.
// Then it restores all objects with their proper layout engine and sequence diagram positions
func Layout(ctx context.Context, g *d2graph.Graph, layout func(ctx context.Context, g *d2graph.Graph) error) error {
	objectsToRemove := make(map[*d2graph.Object]struct{})
	edgesToRemove := make(map[*d2graph.Edge]struct{})
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
		obj.Box = geo.NewBox(nil, sd.getWidth(), sd.getHeight())
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

	layoutEdges, edgeOrder := getLayoutEdges(g, edgesToRemove)
	g.Edges = layoutEdges
	layoutObjects, objectOrder := getLayoutObjects(g, objectsToRemove)
	g.Objects = layoutObjects

	if g.Root.Attributes.Shape.Value == d2target.ShapeSequenceDiagram {
		// don't need to run the layout engine if the root is a sequence diagram
		g.Root.TopLeft = geo.NewPoint(0, 0)
	} else if err := layout(ctx, g); err != nil {
		return err
	}

	cleanup(g, sequenceDiagrams, objectOrder, edgeOrder)

	return nil
}

// layoutSequenceDiagram finds the edges inside the sequence diagram and performs the layout on the object descendants
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
	return sd
}

func getLayoutEdges(g *d2graph.Graph, toRemove map[*d2graph.Edge]struct{}) ([]*d2graph.Edge, map[string]int) {
	edgeOrder := make(map[string]int)
	layoutEdges := make([]*d2graph.Edge, 0, len(g.Edges)-len(toRemove))
	for i, edge := range g.Edges {
		edgeOrder[edge.AbsID()] = i
		if _, exists := toRemove[edge]; !exists {
			layoutEdges = append(layoutEdges, edge)
		}
	}
	return layoutEdges, edgeOrder
}

func getLayoutObjects(g *d2graph.Graph, toRemove map[*d2graph.Object]struct{}) ([]*d2graph.Object, map[string]int) {
	objectOrder := make(map[string]int)
	layoutObjects := make([]*d2graph.Object, 0, len(toRemove))
	for i, obj := range g.Objects {
		objectOrder[obj.AbsID()] = i
		if _, exists := toRemove[obj]; !exists {
			layoutObjects = append(layoutObjects, obj)
		}
	}
	return layoutObjects, objectOrder
}

// cleanup restores the graph state after the layout engine finished.
// Restoring the graph state means:
// - translating the sequence to the node position placed by the layout engine
// - restore the children (`obj.ChildrenArray`) of the sequence diagram graph object
// - adds the sequence diagram edges (messages) back to the graph
// - adds the sequence diagram lifelines to the graph edges
// - adds the sequence diagram descendants back to the graph objects
// - sorts edges and objects to their original graph order
func cleanup(g *d2graph.Graph, sequenceDiagrams map[string]*sequenceDiagram, objectsOrder, edgesOrder map[string]int) {
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
		g.Edges = append(g.Edges, sequenceDiagrams[obj.AbsID()].messages...)
		g.Edges = append(g.Edges, sequenceDiagrams[obj.AbsID()].lifelines...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].actors...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].spans...)
	}

	// no new objects, so just keep the same position
	sort.SliceStable(g.Objects, func(i, j int) bool {
		return objectsOrder[g.Objects[i].AbsID()] < objectsOrder[g.Objects[j].AbsID()]
	})

	// sequence diagrams add lifelines, and they must be the last ones in this slice
	sort.SliceStable(g.Edges, func(i, j int) bool {
		iOrder, iExists := edgesOrder[g.Edges[i].AbsID()]
		jOrder, jExists := edgesOrder[g.Edges[j].AbsID()]
		if iExists && jExists {
			return iOrder < jOrder
		} else if iExists && !jExists {
			return true
		}
		// either both don't exist or i doesn't exist and j exists
		return false
	})
}
