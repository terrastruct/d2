package d2layouts

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2grid"
	"oss.terrastruct.com/d2/d2layouts/d2near"
	"oss.terrastruct.com/d2/d2layouts/d2sequence"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/util-go/go2"
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

func SaveChildrenOrder(container *d2graph.Object) (restoreOrder func()) {
	objectOrder := make(map[string]int, len(container.ChildrenArray))
	for i, obj := range container.ChildrenArray {
		objectOrder[obj.AbsID()] = i
	}
	return func() {
		sort.SliceStable(container.ChildrenArray, func(i, j int) bool {
			return objectOrder[container.ChildrenArray[i].AbsID()] < objectOrder[container.ChildrenArray[j].AbsID()]
		})
	}
}

func SaveOrder(g *d2graph.Graph) (restoreOrder func()) {
	objectOrder := make(map[string]int, len(g.Objects))
	for i, obj := range g.Objects {
		objectOrder[obj.AbsID()] = i
	}
	edgeOrder := make(map[string]int, len(g.Edges))
	for i, edge := range g.Edges {
		edgeOrder[edge.AbsID()] = i
	}
	restoreRootOrder := SaveChildrenOrder(g.Root)
	return func() {
		sort.SliceStable(g.Objects, func(i, j int) bool {
			return objectOrder[g.Objects[i].AbsID()] < objectOrder[g.Objects[j].AbsID()]
		})
		sort.SliceStable(g.Edges, func(i, j int) bool {
			iIndex, iHas := edgeOrder[g.Edges[i].AbsID()]
			jIndex, jHas := edgeOrder[g.Edges[j].AbsID()]
			if iHas && jHas {
				return iIndex < jIndex
			}
			return iHas
		})
		restoreRootOrder()
	}
}

func LayoutNested(ctx context.Context, g *d2graph.Graph, graphInfo GraphInfo, coreLayout d2graph.LayoutGraph, edgeRouter d2graph.RouteEdges) error {
	g.Root.Box = &geo.Box{}

	// Before we can layout these nodes, we need to handle all nested diagrams first.
	extracted := make(map[string]*d2graph.Graph)
	var extractedOrder []string
	var extractedEdges []*d2graph.Edge
	var extractedEdgeIDs []edgeIDs

	var constantNears []*d2graph.Graph
	restoreOrder := SaveOrder(g)
	defer restoreOrder()

	// Iterate top-down from Root so all nested diagrams can process their own contents
	queue := make([]*d2graph.Object, 0, len(g.Root.ChildrenArray))
	queue = append(queue, g.Root.ChildrenArray...)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		isGridCellContainer := graphInfo.DiagramType == GridDiagram &&
			curr.IsContainer() && curr.Parent == g.Root
		gi := NestedGraphInfo(curr)

		if isGridCellContainer && gi.isDefault() {
			// if we are in a grid diagram, and our children have descendants
			// we need to run layout on them first, even if they are not special diagram types

			// First we extract the grid cell container as a nested graph with includeSelf=true
			// resulting in externalEdges=[A, C] and nestedGraph.Edges=[B]
			// ┌grid(g.Root)───────────────────┐    ┌grid(g.Root)───────────────────┐
			// │ ┌────┐      ┌curr───────────┐ │    │ ┌────┐                        │
			// │ │    │      │  ┌──┐    ┌──┐ │ │    │ │    │                        │
			// │ │    ├──A──►│  │  ├─B─►│  │ │ │ => │ │    │                        │
			// │ │    ├──────┼C─┼──┼───►│  │ │ │    │ │    │                        │
			// │ │    │      │  └──┘    └──┘ │ │    │ │    │                        │
			// │ └────┘      └───────────────┘ │    │ └────┘                        │
			// └───────────────────────────────┘    └───────────────────────────────┘
			nestedGraph, externalEdges, externalEdgeIDs := ExtractSubgraph(curr, true)

			// Then we layout curr as a nested graph and re-inject it
			id := curr.AbsID()
			err := LayoutNested(ctx, nestedGraph, GraphInfo{}, coreLayout, edgeRouter)
			if err != nil {
				return err
			}
			InjectNested(g.Root, nestedGraph, false)
			g.Edges = append(g.Edges, externalEdges...)
			restoreOrder()

			// layout can replace Objects so we need to update the references we are holding onto (curr + externalEdges)
			idToObj := make(map[string]*d2graph.Object)
			for _, o := range g.Objects {
				idToObj[o.AbsID()] = o
			}
			lookup := func(idStr string) (*d2graph.Object, error) {
				o, exists := idToObj[idStr]
				if !exists {
					return nil, fmt.Errorf("could not find object %#v after layout", idStr)
				}
				return o, nil
			}
			curr, err = lookup(id)
			if err != nil {
				return err
			}
			for i, e := range externalEdges {
				src, err := lookup(externalEdgeIDs[i].srcID)
				if err != nil {
					return err
				}
				e.Src = src
				dst, err := lookup(externalEdgeIDs[i].dstID)
				if err != nil {
					return err
				}
				e.Dst = dst
			}

			// position nested graph (excluding curr) relative to curr
			dx := 0 - curr.TopLeft.X
			dy := 0 - curr.TopLeft.Y
			for _, o := range nestedGraph.Objects {
				if o == curr {
					continue
				}
				o.TopLeft.X += dx
				o.TopLeft.Y += dy
			}
			for _, e := range nestedGraph.Edges {
				e.Move(dx, dy)
			}

			// Then after re-injecting everything, we extract curr with includeSelf=false,
			// and externalEdges=[C], nestedGraph.Edges=[B], and graph.Edges=[A].
			// This will leave A in the graph to be routed by grid layout, and C will have cross-graph edge routing
			// Note: currently grid layout's cell-cell edge routing, and cross-graph edge routing behave the same,
			// but these are simple placeholder routings and they may be different in the future
			// ┌grid(g.Root)───────────────────┐    ┌grid(g.Root)───────────────────┐
			// │ ┌────┐      ┌curr───────────┐ │    │ ┌────┐      ┌curr───────────┐ │
			// │ │    │      │  ┌──┐    ┌──┐ │ │    │ │    │      │               │ │
			// │ │    ├──A──►│  │  ├─B─►│  │ │ │ => │ │    ├──A──►│               │ │
			// │ │    ├──────┼C─┼──┼───►│  │ │ │    │ │    │      │               │ │
			// │ │    │      │  └──┘    └──┘ │ │    │ │    │      │               │ │
			// │ └────┘      └───────────────┘ │    │ └────┘      └───────────────┘ │
			// └───────────────────────────────┘    └───────────────────────────────┘
			nestedGraph, externalEdges, externalEdgeIDs = ExtractSubgraph(curr, false)
			extractedEdges = append(extractedEdges, externalEdges...)
			extractedEdgeIDs = append(extractedEdgeIDs, externalEdgeIDs...)

			extracted[id] = nestedGraph
			extractedOrder = append(extractedOrder, id)
			continue
		}

		if !gi.isDefault() {
			// empty grid or sequence can have 0 objects..
			if !gi.IsConstantNear && len(curr.Children) == 0 {
				continue
			}

			// There is a nested diagram here, so extract its contents and process in the same way
			nestedGraph, externalEdges, externalEdgeIDs := ExtractSubgraph(curr, gi.IsConstantNear)
			extractedEdges = append(extractedEdges, externalEdges...)
			extractedEdgeIDs = append(extractedEdgeIDs, externalEdgeIDs...)

			log.Debug(ctx, "layout nested", slog.Any("level", curr.Level()), slog.Any("child", curr.AbsID()), slog.Any("gi", gi))
			nestedInfo := gi
			nearKey := curr.NearKey
			if gi.IsConstantNear {
				// layout nested as a non-near
				nestedInfo = GraphInfo{}
				curr.NearKey = nil
			}

			err := LayoutNested(ctx, nestedGraph, nestedInfo, coreLayout, edgeRouter)
			if err != nil {
				return err
			}
			// coreLayout can overwrite graph contents with newly created *Object pointers
			// so we need to update `curr` with nestedGraph's value
			if gi.IsConstantNear {
				curr = nestedGraph.Root.ChildrenArray[0]
			}

			if gi.IsConstantNear {
				curr.NearKey = nearKey
			} else {
				FitToGraph(curr, nestedGraph, geo.Spacing{})
				curr.TopLeft = geo.NewPoint(0, 0)
			}

			if gi.IsConstantNear {
				// near layout will inject these nestedGraphs
				constantNears = append(constantNears, nestedGraph)
			} else {
				// We will restore the contents after running layout with child as the placeholder
				// We need to reference using ID because there may be a new object to use after coreLayout
				id := curr.AbsID()
				extracted[id] = nestedGraph
				extractedOrder = append(extractedOrder, id)
			}
		} else if len(curr.ChildrenArray) > 0 {
			queue = append(queue, curr.ChildrenArray...)
		}
	}

	// We can now run layout with accurate sizes of nested layout containers
	// Layout according to the type of diagram
	var err error
	if len(g.Objects) > 0 {
		switch graphInfo.DiagramType {
		case GridDiagram:
			log.Debug(ctx, "layout grid", slog.Any("rootlevel", g.RootLevel), slog.Any("shapes", g.PrintString()))
			if err = d2grid.Layout(ctx, g); err != nil {
				return err
			}

		case SequenceDiagram:
			log.Debug(ctx, "layout sequence", slog.Any("rootlevel", g.RootLevel), slog.Any("shapes", g.PrintString()))
			err = d2sequence.Layout(ctx, g, coreLayout)
			if err != nil {
				return err
			}
		default:
			log.Debug(ctx, "default layout", slog.Any("rootlevel", g.RootLevel), slog.Any("shapes", g.PrintString()))
			err := coreLayout(ctx, g)
			if err != nil {
				return err
			}
		}
	}

	if len(constantNears) > 0 {
		err = d2near.Layout(ctx, g, constantNears)
		if err != nil {
			return err
		}
	}

	idToObj := make(map[string]*d2graph.Object)
	for _, o := range g.Objects {
		idToObj[o.AbsID()] = o
	}

	// With the layout set, inject all the extracted graphs
	for _, id := range extractedOrder {
		nestedGraph := extracted[id]
		// we have to find the object by ID because coreLayout can replace the Objects in graph
		obj, exists := idToObj[id]
		if !exists {
			return fmt.Errorf("could not find object %#v after layout", id)
		}
		InjectNested(obj, nestedGraph, true)
		PositionNested(obj, nestedGraph)
	}

	if len(extractedEdges) > 0 {
		// update map with injected objects
		for _, o := range g.Objects {
			idToObj[o.AbsID()] = o
		}

		// Restore cross-graph edges and route them
		g.Edges = append(g.Edges, extractedEdges...)
		for i, e := range extractedEdges {
			ids := extractedEdgeIDs[i]
			// update object references
			src, exists := idToObj[ids.srcID]
			if !exists {
				return fmt.Errorf("could not find object %#v after layout", ids.srcID)
			}
			e.Src = src
			dst, exists := idToObj[ids.dstID]
			if !exists {
				return fmt.Errorf("could not find object %#v after layout", ids.dstID)
			}
			e.Dst = dst
		}

		err = edgeRouter(ctx, g, extractedEdges)
		if err != nil {
			return err
		}
		// need to update pointers if plugin performs edge routing
		for i, e := range extractedEdges {
			ids := extractedEdgeIDs[i]
			src, exists := idToObj[ids.srcID]
			if !exists {
				return fmt.Errorf("could not find object %#v after routing", ids.srcID)
			}
			e.Src = src
			dst, exists := idToObj[ids.dstID]
			if !exists {
				return fmt.Errorf("could not find object %#v after routing", ids.dstID)
			}
			e.Dst = dst
		}
	}

	log.Debug(ctx, "done", slog.Any("rootlevel", g.RootLevel), slog.Any("shapes", g.PrintString()))
	return err
}

func DefaultRouter(ctx context.Context, graph *d2graph.Graph, edges []*d2graph.Edge) error {
	for _, e := range edges {
		// TODO replace simple straight line edge routing
		e.Route = []*geo.Point{e.Src.Center(), e.Dst.Center()}
		e.TraceToShape(e.Route, 0, 1, float64(label.PADDING))
		if e.Label.Value != "" {
			e.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}
	return nil
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

type edgeIDs struct {
	srcID, dstID string
}

func ExtractSubgraph(container *d2graph.Object, includeSelf bool) (nestedGraph *d2graph.Graph, externalEdges []*d2graph.Edge, externalEdgeIDs []edgeIDs) {
	// includeSelf: when we have a constant near or a grid cell that is a container,
	// we want to include itself in the nested graph, not just its descendants,
	nestedGraph = d2graph.NewGraph()
	nestedGraph.RootLevel = int(container.Level())
	if includeSelf {
		nestedGraph.RootLevel--
	}
	nestedGraph.Root.Attributes = container.Attributes
	nestedGraph.Root.Box = &geo.Box{}
	nestedGraph.Root.LabelPosition = container.LabelPosition
	nestedGraph.Root.IconPosition = container.IconPosition

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
		srcIsNested := isNestedObject(edge.Src)
		if d2sequence.IsLifelineEnd(edge.Dst) {
			// special handling for lifelines since their edge.Dst is a special Object
			if srcIsNested {
				nestedGraph.Edges = append(nestedGraph.Edges, edge)
			} else {
				remainingEdges = append(remainingEdges, edge)
			}
			continue
		}
		dstIsNested := isNestedObject(edge.Dst)
		if srcIsNested && dstIsNested {
			nestedGraph.Edges = append(nestedGraph.Edges, edge)
		} else if srcIsNested || dstIsNested {
			externalEdges = append(externalEdges, edge)
			// we need these AbsIDs when reconnecting since parent references may become stale
			externalEdgeIDs = append(externalEdgeIDs, edgeIDs{
				srcID: edge.Src.AbsID(),
				dstID: edge.Dst.AbsID(),
			})
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

	return nestedGraph, externalEdges, externalEdgeIDs
}

func InjectNested(container *d2graph.Object, nestedGraph *d2graph.Graph, isRoot bool) {
	g := container.Graph
	for _, obj := range nestedGraph.Root.ChildrenArray {
		obj.Parent = container
		if container.Children == nil {
			container.Children = make(map[string]*d2graph.Object)
		}
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
		if nestedGraph.Root.IconPosition != nil {
			container.IconPosition = nestedGraph.Root.IconPosition
		}
		container.Attributes = nestedGraph.Root.Attributes
	}
}

func PositionNested(container *d2graph.Object, nestedGraph *d2graph.Graph) {
	// tl, _ := boundingBox(nestedGraph)
	// Note: assumes nestedGraph's layout has contents positioned relative to 0,0
	dx := container.TopLeft.X //- tl.X
	dy := container.TopLeft.Y //- tl.Y
	if dx == 0 && dy == 0 {
		return
	}
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
