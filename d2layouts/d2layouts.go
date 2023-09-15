package d2layouts

import (
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
)

type GraphType string

const (
	DefaultGraphType  GraphType = ""
	ConstantNearGraph GraphType = "constant-near"
	GridDiagram       GraphType = "grid-diagram"
	SequenceDiagram   GraphType = "sequence-diagram"
)

func LayoutNested(g *d2graph.Graph, graphType GraphType, coreLayout d2graph.LayoutGraph) geo.Spacing {
	// Before we can layout these nodes, we need to handle all nested diagrams first.
	extracted := make(map[*d2graph.Object]*d2graph.Graph)

	// Iterate top-down from Root so all nested diagrams can process their own contents
	queue := make([]*d2graph.Object, 0, len(g.Root.ChildrenArray))
	queue = append(queue, g.Root.ChildrenArray...)

	for _, child := range queue {
		if graphType := NestedGraphType(child); graphType != DefaultGraphType {
			// There is a nested diagram here, so extract its contents and process in the same way
			nestedGraph := ExtractNested(child)

			// Layout of nestedGraph is completed
			spacing := LayoutNested(nestedGraph, graphType, coreLayout)

			// Fit child to size of nested layout
			FitToGraph(child, nestedGraph, spacing)

			// We will restore the contents after running layout with child as the placeholder
			extracted[child] = nestedGraph
		} else if len(child.Children) > 0 {
			queue = append(queue, child.ChildrenArray...)
		}
	}

	// We can now run layout with accurate sizes of nested layout containers
	// Layout according to the type of diagram
	spacing := LayoutDiagram(g, graphType, coreLayout)

	// With the layout set, inject all the extracted graphs
	for n, nestedGraph := range extracted {
		InjectNested(n, nestedGraph)
	}

	return spacing
}

func NestedGraphType(obj *d2graph.Object) GraphType {
	if obj.Graph.RootLevel == 0 && obj.IsConstantNear() {
		return ConstantNearGraph
	}
	if obj.IsGridDiagram() {
		return GridDiagram
	}
	if obj.IsSequenceDiagram() {
		return SequenceDiagram
	}
	return DefaultGraphType
}

func ExtractNested(container *d2graph.Object) *d2graph.Graph {
	nestedGraph := d2graph.NewGraph()
	nestedGraph.RootLevel = int(container.Level())

	// separate out nested edges
	g := container.Graph
	remainingEdges := make([]*d2graph.Edge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if edge.Src.Parent.IsDescendantOf(container) && edge.Dst.Parent.IsDescendantOf(container) {
			nestedGraph.Edges = append(nestedGraph.Edges, edge)
		} else {
			remainingEdges = append(remainingEdges, edge)
		}
	}
	g.Edges = remainingEdges

	// separate out nested objects
	remainingObjects := make([]*d2graph.Object, 0, len(g.Objects))
	for _, obj := range g.Objects {
		if obj.IsDescendantOf(container) {
			nestedGraph.Objects = append(nestedGraph.Objects, obj)
		} else {
			remainingObjects = append(remainingObjects, obj)
		}
	}
	g.Objects = remainingObjects

	// update object and new root references
	for _, o := range nestedGraph.Objects {
		o.Graph = nestedGraph
	}
	// set root references
	nestedGraph.Root.ChildrenArray = append(nestedGraph.Root.ChildrenArray, container.ChildrenArray...)
	for _, child := range container.ChildrenArray {
		child.Parent = nestedGraph.Root
		nestedGraph.Root.Children[strings.ToLower(child.ID)] = child
	}

	// remove container's references
	for k := range container.Children {
		delete(container.Children, k)
	}
	container.ChildrenArray = nil

	// position contents relative to 0,0
	dx := -container.TopLeft.X
	dy := -container.TopLeft.Y
	for _, o := range nestedGraph.Objects {
		o.TopLeft.X += dx
		o.TopLeft.Y += dy
	}
	for _, e := range nestedGraph.Edges {
		e.Move(dx, dy)
	}
	return nestedGraph
}

func InjectNested(container *d2graph.Object, nestedGraph *d2graph.Graph) {
	g := container.Graph
	for _, obj := range nestedGraph.Root.ChildrenArray {
		obj.Parent = container
		container.Children[strings.ToLower(obj.ID)] = obj
		container.ChildrenArray = append(container.ChildrenArray, obj)
	}
	for _, obj := range nestedGraph.Objects {
		obj.Graph = g
	}
	g.Objects = append(g.Objects, nestedGraph.Objects...)
	g.Edges = append(g.Edges, nestedGraph.Edges...)

	// Note: assumes nestedGraph's layout has contents positioned relative to 0,0
	dx := container.TopLeft.X
	dy := container.TopLeft.Y
	for _, o := range nestedGraph.Objects {
		o.TopLeft.X += dx
		o.TopLeft.Y += dy
	}
	for _, e := range nestedGraph.Edges {
		e.Move(dx, dy)
	}
}

func boundingBox(g *d2graph.Graph) (tl, br *geo.Point) {
	if len(g.Objects) == 0 {
		return geo.NewPoint(0, 0), geo.NewPoint(0, 0)
	}
	tl = geo.NewPoint(math.Inf(1), math.Inf(1))
	br = geo.NewPoint(math.Inf(-1), math.Inf(-1))

	for _, obj := range g.Objects {
		tl.X = math.Min(tl.X, obj.TopLeft.X)
		tl.Y = math.Min(tl.Y, obj.TopLeft.Y)
		br.X = math.Max(br.X, obj.TopLeft.X+obj.Width)
		br.Y = math.Max(br.Y, obj.TopLeft.Y+obj.Height)
	}

	return tl, br
}

func FitToGraph(container *d2graph.Object, nestedGraph *d2graph.Graph, padding geo.Spacing) {
	tl, br := boundingBox(nestedGraph)
	container.Width = padding.Left + br.X - tl.X + padding.Right
	container.Height = padding.Top + br.Y - tl.Y + padding.Bottom
}

func LayoutDiagram(graph *d2graph.Graph, graphType GraphType, coreLayout d2graph.LayoutGraph) geo.Spacing {
	// TODO
	return geo.Spacing{}
}
