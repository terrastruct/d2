package d2layouts

import "oss.terrastruct.com/d2/d2graph"

func LayoutNested(g *d2graph.Graph, graphType string, coreLayout d2graph.LayoutGraph) {
	// Before we can layout these nodes, we need to handle all nested diagrams first.
	extracted := make(map[*d2graph.Object]*d2graph.Graph)

	// Iterate top-down from Root so all nested diagrams can process their own contents
	queue := make([]*d2graph.Object, 0, len(g.Root.ChildrenArray))
	queue = append(queue, g.Root.ChildrenArray...)

	for _, child := range queue {
		if graphType := NestedGraphType(child); graphType != nil {
			// There is a nested diagram here, so extract its contents and process in the same way
			nestedGraph := ExtractNested(child)

			// Layout of nestedGraph is completed
			LayoutNested(nestedGraph, *graphType, coreLayout)

			// Fit child to size of nested layout
			FitToGraph(child, nestedGraph)

			// We will restore the contents after running layout with child as the placeholder
			extracted[child] = nestedGraph
		} else if len(child.Children) > 0 {
			queue = append(queue, child.ChildrenArray...)
		}
	}

	// We can now run layout with accurate sizes of nested layout containers
	// Layout according to the type of diagram
	LayoutDiagram(g, graphType, coreLayout)

	// With the layout set, inject all the extracted graphs
	for n, nestedGraph := range extracted {
		InjectNested(n, nestedGraph)
	}
}

func NestedGraphType(container *d2graph.Object) *string {
	// TODO
	return nil
}

func ExtractNested(container *d2graph.Object) *d2graph.Graph {
	// TODO
	return nil
}

func InjectNested(container *d2graph.Object, graph *d2graph.Graph) {
	// TODO
}

func FitToGraph(container *d2graph.Object, graph *d2graph.Graph) {
	// TODO
}

func LayoutDiagram(graph *d2graph.Graph, graphType string, coreLayout d2graph.LayoutGraph) {
	// TODO
}
