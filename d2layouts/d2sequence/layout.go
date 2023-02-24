package d2sequence

import (
	"context"
	"sort"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
)

func WithoutSequenceDiagrams(ctx context.Context, g *d2graph.Graph) (map[string]*sequenceDiagram, map[string]int, map[string]int, error) {
	objectsToRemove := make(map[*d2graph.Object]struct{})
	edgesToRemove := make(map[*d2graph.Edge]struct{})
	sequenceDiagrams := make(map[string]*sequenceDiagram)

	if len(g.Objects) > 0 {
		queue := make([]*d2graph.Object, 1, len(g.Objects))
		queue[0] = g.Root
		for len(queue) > 0 {
			obj := queue[0]
			queue = queue[1:]
			if len(obj.ChildrenArray) == 0 {
				continue
			}
			if obj.Attributes.Shape.Value != d2target.ShapeSequenceDiagram {
				queue = append(queue, obj.ChildrenArray...)
				continue
			}

			sd, err := layoutSequenceDiagram(g, obj)
			if err != nil {
				return nil, nil, nil, err
			}
			obj.Children = make(map[string]*d2graph.Object)
			obj.ChildrenArray = nil
			obj.Box = geo.NewBox(nil, sd.getWidth()+GROUP_CONTAINER_PADDING*2, sd.getHeight()+GROUP_CONTAINER_PADDING*2)
			obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			sequenceDiagrams[obj.AbsID()] = sd

			for _, edge := range sd.messages {
				edgesToRemove[edge] = struct{}{}
			}
			for _, obj := range sd.actors {
				objectsToRemove[obj] = struct{}{}
			}
			for _, obj := range sd.notes {
				objectsToRemove[obj] = struct{}{}
			}
			for _, obj := range sd.groups {
				objectsToRemove[obj] = struct{}{}
			}
			for _, obj := range sd.spans {
				objectsToRemove[obj] = struct{}{}
			}
		}
	}

	layoutEdges, edgeOrder := getLayoutEdges(g, edgesToRemove)
	g.Edges = layoutEdges
	layoutObjects, objectOrder := getLayoutObjects(g, objectsToRemove)
	// TODO this isn't a proper deletion because the objects still appear as children of the object
	g.Objects = layoutObjects

	return sequenceDiagrams, objectOrder, edgeOrder, nil
}

// Layout runs the sequence diagram layout engine on objects of shape sequence_diagram
//
// 1. Traverse graph from root, skip objects with shape not `sequence_diagram`
// 2. Construct a sequence diagram from all descendant objects and edges
// 3. Remove those objects and edges from the main graph
// 4. Run layout on sequence diagrams
// 5. Set the resulting dimensions to the main graph shape
// 6. Run core layouts (still without sequence diagram innards)
// 7. Put back sequence diagram innards in correct location
func Layout(ctx context.Context, g *d2graph.Graph, layout func(ctx context.Context, g *d2graph.Graph) error) error {
	sequenceDiagrams, objectOrder, edgeOrder, err := WithoutSequenceDiagrams(ctx, g)
	if err != nil {
		return err
	}

	if g.Root.IsSequenceDiagram() {
		// the sequence diagram is the only layout engine if the whole diagram is
		// shape: sequence_diagram
		g.Root.TopLeft = geo.NewPoint(0, 0)
	} else if err := layout(ctx, g); err != nil {
		return err
	}

	cleanup(g, sequenceDiagrams, objectOrder, edgeOrder)
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

// cleanup restores the graph after the core layout engine finishes
// - translating the sequence diagram to its position placed by the core layout engine
// - restore the children of the sequence diagram graph object
// - adds the sequence diagram edges (messages) back to the graph
// - adds the sequence diagram lifelines to the graph edges
// - adds the sequence diagram descendants back to the graph objects
// - sorts edges and objects to their original graph order
func cleanup(g *d2graph.Graph, sequenceDiagrams map[string]*sequenceDiagram, objectsOrder, edgesOrder map[string]int) {
	var objects []*d2graph.Object
	if g.Root.IsSequenceDiagram() {
		objects = []*d2graph.Object{g.Root}
	} else {
		objects = g.Objects
	}
	for _, obj := range objects {
		if _, exists := sequenceDiagrams[obj.AbsID()]; !exists {
			continue
		}
		obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
		sd := sequenceDiagrams[obj.AbsID()]

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
			obj.Children[child.ID] = child
			obj.ChildrenArray = append(obj.ChildrenArray, child)
		}
		for _, child := range sd.groups {
			if child.Parent.AbsID() == obj.AbsID() {
				obj.Children[child.ID] = child
				obj.ChildrenArray = append(obj.ChildrenArray, child)
			}
		}

		g.Edges = append(g.Edges, sequenceDiagrams[obj.AbsID()].messages...)
		g.Edges = append(g.Edges, sequenceDiagrams[obj.AbsID()].lifelines...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].actors...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].notes...)
		g.Objects = append(g.Objects, sequenceDiagrams[obj.AbsID()].groups...)
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
