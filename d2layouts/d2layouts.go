package d2layouts

import (
	"context"
	"math"
	"strings"

	"cdr.dev/slog"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2grid"
	"oss.terrastruct.com/d2/d2layouts/d2near"
	"oss.terrastruct.com/d2/d2layouts/d2sequence"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/log"
)

type DiagramType string

// a grid diagram at a constant near is
const (
	DefaultGraphType  DiagramType = ""
	ConstantNearGraph DiagramType = "constant-near"
	GridDiagram       DiagramType = "grid-diagram"
	SequenceDiagram   DiagramType = "sequence-diagram"
)

type GraphInfo struct {
	IsConstantNear bool
	DiagramType    DiagramType
}

func (gi GraphInfo) isDefault() bool {
	return !gi.IsConstantNear && gi.DiagramType == DefaultGraphType
}

func LayoutNested(ctx context.Context, g *d2graph.Graph, graphInfo GraphInfo, coreLayout d2graph.LayoutGraph) geo.Spacing {
	g.Root.Box = &geo.Box{}

	log.Warn(ctx, "ln info", slog.F("gi", graphInfo), slog.F("root level", g.RootLevel))
	// Before we can layout these nodes, we need to handle all nested diagrams first.
	extracted := make(map[*d2graph.Object]*d2graph.Graph)
	extractedInfo := make(map[*d2graph.Object]GraphInfo)

	var constantNears []*d2graph.Graph

	// Iterate top-down from Root so all nested diagrams can process their own contents
	queue := make([]*d2graph.Object, 0, len(g.Root.ChildrenArray))
	queue = append(queue, g.Root.ChildrenArray...)

	for _, curr := range queue {
		isGridCellContainer := graphInfo.DiagramType == GridDiagram &&
			curr.IsContainer() && curr.Parent == g.Root
		gi := NestedGraphInfo(curr)
		// if we are in a grid diagram, and our children have descendants
		// we need to run layout on them first, even if they are not special diagram types

		if isGridCellContainer {
			nestedGraph := ExtractSubgraph(curr, true)
			LayoutNested(ctx, nestedGraph, GraphInfo{}, coreLayout)
			InjectNested(g.Root, nestedGraph, false)
			dx := -curr.TopLeft.X
			dy := -curr.TopLeft.Y
			for _, o := range nestedGraph.Objects {
				o.TopLeft.X += dx
				o.TopLeft.Y += dy
			}
			for _, e := range nestedGraph.Edges {
				e.Move(dx, dy)
			}

		} else if !gi.isDefault() {
			extractedInfo[curr] = gi

			// There is a nested diagram here, so extract its contents and process in the same way
			nestedGraph := ExtractSubgraph(curr, gi.IsConstantNear)

			log.Info(ctx, "layout nested", slog.F("level", curr.Level()), slog.F("child", curr.AbsID()))
			nestedInfo := gi
			nearKey := curr.NearKey
			if gi.IsConstantNear {
				// layout nested as a non-near
				nestedInfo = GraphInfo{}
				curr.NearKey = nil
			}

			spacing := LayoutNested(ctx, nestedGraph, nestedInfo, coreLayout)

			if gi.IsConstantNear {
				curr.NearKey = nearKey
			} else {
				FitToGraph(curr, nestedGraph, spacing)
				curr.TopLeft = geo.NewPoint(0, 0)
			}

			if gi.IsConstantNear {
				// near layout will inject these nestedGraphs
				constantNears = append(constantNears, nestedGraph)
			} else {
				// We will restore the contents after running layout with child as the placeholder
				extracted[curr] = nestedGraph
			}
		} else if len(curr.Children) > 0 {
			queue = append(queue, curr.ChildrenArray...)
		}
	}
	log.Warn(ctx, "finished descendants", slog.F("rootlevel", g.RootLevel), slog.F("shapes", g.PrintString()))

	// We can now run layout with accurate sizes of nested layout containers
	// Layout according to the type of diagram
	LayoutDiagram := func(ctx context.Context, g *d2graph.Graph, graphInfo GraphInfo, coreLayout d2graph.LayoutGraph) geo.Spacing {
		spacing := geo.Spacing{}
		var err error
		switch graphInfo.DiagramType {
		case GridDiagram:
			log.Warn(ctx, "layout grid", slog.F("rootlevel", g.RootLevel), slog.F("shapes", g.PrintString()))
			if err = d2grid.Layout2(ctx, g); err != nil {
				panic(err)
			}

		case SequenceDiagram:
			log.Warn(ctx, "layout sequence", slog.F("rootlevel", g.RootLevel), slog.F("shapes", g.PrintString()))
			err = d2sequence.Layout2(ctx, g, coreLayout)
			if err != nil {
				panic(err)
			}
		default:
			log.Warn(ctx, "default layout", slog.F("rootlevel", g.RootLevel), slog.F("shapes", g.PrintString()))
			err := coreLayout(ctx, g)
			if err != nil {
				panic(err)
			}
		}
		return spacing
	}
	spacing := LayoutDiagram(ctx, g, graphInfo, coreLayout)

	if len(constantNears) > 0 {
		err := d2near.Layout(ctx, g, constantNears)
		if err != nil {
			panic(err)
		}
	}

	// With the layout set, inject all the extracted graphs
	for n, nestedGraph := range extracted {
		InjectNested(n, nestedGraph, true)
		PositionNested(n, nestedGraph)
	}

	log.Warn(ctx, "done", slog.F("rootlevel", g.RootLevel))
	return spacing
}

func NestedGraphInfo(obj *d2graph.Object) (gi GraphInfo) {
	if obj.Graph.RootLevel == 0 && obj.IsConstantNear() {
		gi.IsConstantNear = true
	}
	if obj.IsSequenceDiagram() {
		gi.DiagramType = SequenceDiagram
	} else if obj.IsGridDiagram() {
		gi.DiagramType = GridDiagram
	}
	return gi
}

func ExtractSubgraph(container *d2graph.Object, includeSelf bool) *d2graph.Graph {
	// includeSelf: when we have a constant near or a grid cell that is a container,
	// we want to include itself in the nested graph, not just its descendants,
	nestedGraph := d2graph.NewGraph()
	nestedGraph.RootLevel = int(container.Level())
	if includeSelf {
		nestedGraph.RootLevel--
	}
	nestedGraph.Root.Attributes = container.Attributes
	nestedGraph.Root.Box = &geo.Box{}

	isNestedObject := func(obj *d2graph.Object) bool {
		if includeSelf {
			return obj.IsDescendantOf(container)
		}
		return obj.Parent.IsDescendantOf(container)
	}

	// separate out nested edges
	g := container.Graph
	remainingEdges := make([]*d2graph.Edge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if isNestedObject(edge.Src) && isNestedObject(edge.Dst) {
			nestedGraph.Edges = append(nestedGraph.Edges, edge)
		} else {
			remainingEdges = append(remainingEdges, edge)
		}
	}
	g.Edges = remainingEdges

	// separate out nested objects
	remainingObjects := make([]*d2graph.Object, 0, len(g.Objects))
	for _, obj := range g.Objects {
		if isNestedObject(obj) {
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

	if includeSelf {
		// remove container parent's references
		if container.Parent != nil {
			container.Parent.RemoveChild(container)
		}

		// set root references
		nestedGraph.Root.ChildrenArray = []*d2graph.Object{container}
		container.Parent = nestedGraph.Root
		nestedGraph.Root.Children[strings.ToLower(container.ID)] = container
	} else {
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
	}

	return nestedGraph
}

func InjectNested(container *d2graph.Object, nestedGraph *d2graph.Graph, isRoot bool) {
	// TODO restore order of objects
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

	if isRoot {
		if nestedGraph.Root.LabelPosition != nil {
			container.LabelPosition = nestedGraph.Root.LabelPosition
		}
		container.Attributes = nestedGraph.Root.Attributes
	}
}

func PositionNested(container *d2graph.Object, nestedGraph *d2graph.Graph) {
	// tl, _ := boundingBox(nestedGraph)
	// Note: assumes nestedGraph's layout has contents positioned relative to 0,0
	dx := container.TopLeft.X //- tl.X
	dy := container.TopLeft.Y //- tl.Y
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
		if obj.TopLeft == nil {
			panic(obj.AbsID())
		}
		tl.X = math.Min(tl.X, obj.TopLeft.X)
		tl.Y = math.Min(tl.Y, obj.TopLeft.Y)
		br.X = math.Max(br.X, obj.TopLeft.X+obj.Width)
		br.Y = math.Max(br.Y, obj.TopLeft.Y+obj.Height)
	}

	return tl, br
}

func FitToGraph(container *d2graph.Object, nestedGraph *d2graph.Graph, padding geo.Spacing) {
	var width, height float64
	// if nestedGraph.Root.Box != nil {
	width = nestedGraph.Root.Width
	height = nestedGraph.Root.Height
	if width == 0 || height == 0 {
		tl, br := boundingBox(nestedGraph)
		width = br.X - tl.X
		height = br.Y - tl.Y
	}
	container.Width = padding.Left + width + padding.Right
	container.Height = padding.Top + height + padding.Bottom
}

// func LayoutDiagram(ctx context.Context, g *d2graph.Graph, graphInfo GraphInfo, coreLayout d2graph.LayoutGraph) geo.Spacing {
// 	spacing := geo.Spacing{}
// 	var err error
// 	// TODO

// 	// Need subgraphs?

// 	// if graphInfo.IsConstantNear
// 	// case ConstantNearGraph:
// 	// 	// constantNearGraphs := d2near.WithoutConstantNears(ctx, g)
// 	// 	constantNearGraphs := d2near.WithoutConstantNears(ctx, g)

// 	// 	err = d2near.Layout(ctx, g, constantNearGraphs)
// 	// 	if err != nil {
// 	// 		panic(err)
// 	// 	}

// 	switch graphInfo.DiagramType {
// 	case GridDiagram:
// 		layoutWithGrids := d2grid.Layout(ctx, g, coreLayout)
// 		if err = layoutWithGrids(ctx, g); err != nil {
// 			panic(err)
// 		}

// 	case SequenceDiagram:
// 		err = d2sequence.Layout(ctx, g, coreLayout)
// 		if err != nil {
// 			panic(err)
// 		}
// 	default:
// 		err := coreLayout(ctx, g)
// 		if err != nil {
// 			panic(err)
// 		}
// 	}

// 	return spacing
// }
