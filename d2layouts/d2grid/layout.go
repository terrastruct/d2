package d2grid

import (
	"context"
	"math"
	"sort"
	"strconv"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

const CONTAINER_PADDING = 60.
const HORIZONTAL_PAD = 40.
const VERTICAL_PAD = 40.

type grid struct {
	root    *d2graph.Object
	nodes   []*d2graph.Object
	rows    int
	columns int

	width  float64
	height float64
}

func newGrid(root *d2graph.Object) *grid {
	g := grid{root: root, nodes: root.ChildrenArray}
	if root.Attributes.Rows != nil {
		g.rows, _ = strconv.Atoi(root.Attributes.Rows.Value)
	}
	if root.Attributes.Columns != nil {
		g.columns, _ = strconv.Atoi(root.Attributes.Columns.Value)
	}

	// compute exact row/column count based on values entered
	// TODO consider making this based on node dimensions
	if g.rows == 0 {
		// set rows based on number of columns
		if g.columns == 0 {
			// 0,0: put everything in one row
			g.rows = 1
			g.columns = len(g.nodes)
		} else {
			g.rows = len(g.nodes) / g.columns
			if len(g.nodes)%g.columns != 0 {
				g.rows++
			}
		}
	} else if g.columns == 0 {
		// set columns based on number of rows
		g.columns = len(g.nodes) / g.rows
		if len(g.nodes)%g.rows != 0 {
			g.columns++
		}
	} else {
		// rows and columns specified (add more rows if needed)
		capacity := g.rows * g.columns
		for capacity < len(g.nodes) {
			g.rows++
			capacity += g.columns
		}
	}

	return &g
}

func (g *grid) shift(dx, dy float64) {
	for _, obj := range g.nodes {
		obj.TopLeft.X += dx
		obj.TopLeft.Y += dy
	}
}

// Layout runs the grid layout on containers with rows/columns
// Note: children are not allowed edges or descendants
//
// 1. Traverse graph from root, skip objects with no rows/columns
// 2. Construct a grid with the container children
// 3. Remove the children from the main graph
// 4. Run grid layout
// 5. Set the resulting dimensions to the main graph shape
// 6. Run core layouts (without grid children)
// 7. Put grid children back in correct location
func Layout(ctx context.Context, g *d2graph.Graph, layout d2graph.LayoutGraph) d2graph.LayoutGraph {
	return func(ctx context.Context, g *d2graph.Graph) error {
		grids, objectOrder, err := withoutGrids(ctx, g)
		if err != nil {
			return err
		}

		if g.Root.IsGrid() {
			g.Root.TopLeft = geo.NewPoint(0, 0)
		} else if err := layout(ctx, g); err != nil {
			return err
		}

		cleanup(g, grids, objectOrder)
		return nil
	}
}

func layoutGrid(g *d2graph.Graph, obj *d2graph.Object) (*grid, error) {
	grid := newGrid(obj)

	// position nodes
	cursor := geo.NewPoint(0, 0)
	maxWidth := 0.
	for i := 0; i < grid.rows; i++ {
		maxHeight := 0.
		for j := 0; j < grid.columns; j++ {
			n := grid.nodes[i*grid.columns+j]
			n.TopLeft = cursor.Copy()
			cursor.X += n.Width + HORIZONTAL_PAD
			maxHeight = math.Max(maxHeight, n.Height)
		}
		maxWidth = math.Max(maxWidth, cursor.X-HORIZONTAL_PAD)
		cursor.X = 0
		cursor.Y += float64(maxHeight) + VERTICAL_PAD
	}
	grid.width = maxWidth
	grid.height = cursor.Y - VERTICAL_PAD

	// position labels and icons
	for _, n := range grid.nodes {
		if n.Attributes.Icon != nil {
			n.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			n.IconPosition = go2.Pointer(string(label.InsideMiddleCenter))
		} else {
			n.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
		}
	}

	return grid, nil
}

func withoutGrids(ctx context.Context, g *d2graph.Graph) (idToGrid map[string]*grid, objectOrder map[string]int, err error) {
	toRemove := make(map[*d2graph.Object]struct{})
	grids := make(map[string]*grid)

	if len(g.Objects) > 0 {
		queue := make([]*d2graph.Object, 1, len(g.Objects))
		queue[0] = g.Root
		for len(queue) > 0 {
			obj := queue[0]
			queue = queue[1:]
			if len(obj.ChildrenArray) == 0 {
				continue
			}
			if !obj.IsGrid() {
				queue = append(queue, obj.ChildrenArray...)
				continue
			}

			grid, err := layoutGrid(g, obj)
			if err != nil {
				return nil, nil, err
			}
			obj.Children = make(map[string]*d2graph.Object)
			obj.ChildrenArray = nil
			obj.Box = geo.NewBox(nil, grid.width+CONTAINER_PADDING*2, grid.height+CONTAINER_PADDING*2)
			obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			grids[obj.AbsID()] = grid

			for _, node := range grid.nodes {
				toRemove[node] = struct{}{}
			}
		}
	}

	objectOrder = make(map[string]int)
	layoutObjects := make([]*d2graph.Object, 0, len(toRemove))
	for i, obj := range g.Objects {
		objectOrder[obj.AbsID()] = i
		if _, exists := toRemove[obj]; !exists {
			layoutObjects = append(layoutObjects, obj)
		}
	}
	g.Objects = layoutObjects

	return grids, objectOrder, nil
}

// cleanup restores the graph after the core layout engine finishes
// - translating the grid to its position placed by the core layout engine
// - restore the children of the grid
// - sorts objects to their original graph order
func cleanup(g *d2graph.Graph, grids map[string]*grid, objectsOrder map[string]int) {
	var objects []*d2graph.Object
	if g.Root.IsGrid() {
		objects = []*d2graph.Object{g.Root}
	} else {
		objects = g.Objects
	}
	for _, obj := range objects {
		if _, exists := grids[obj.AbsID()]; !exists {
			continue
		}
		obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
		sd := grids[obj.AbsID()]

		// shift the grid from (0, 0)
		sd.shift(
			obj.TopLeft.X+CONTAINER_PADDING,
			obj.TopLeft.Y+CONTAINER_PADDING,
		)

		obj.Children = make(map[string]*d2graph.Object)
		obj.ChildrenArray = make([]*d2graph.Object, 0)
		for _, child := range sd.nodes {
			obj.Children[child.ID] = child
			obj.ChildrenArray = append(obj.ChildrenArray, child)
		}

		g.Objects = append(g.Objects, grids[obj.AbsID()].nodes...)
	}

	sort.SliceStable(g.Objects, func(i, j int) bool {
		return objectsOrder[g.Objects[i].AbsID()] < objectsOrder[g.Objects[j].AbsID()]
	})
}
