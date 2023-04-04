package d2grid

import (
	"context"
	"math"
	"sort"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

const (
	CONTAINER_PADDING = 60
	HORIZONTAL_PAD    = 40.
	VERTICAL_PAD      = 40.
)

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

		if g.Root.IsGrid() && len(g.Root.ChildrenArray) != 0 {
			g.Root.TopLeft = geo.NewPoint(0, 0)
		} else if err := layout(ctx, g); err != nil {
			return err
		}

		cleanup(g, grids, objectOrder)
		return nil
	}
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
			obj.Box = geo.NewBox(nil, grid.width+2*CONTAINER_PADDING, grid.height+2*CONTAINER_PADDING)
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

func layoutGrid(g *d2graph.Graph, obj *d2graph.Object) (*grid, error) {
	grid := newGrid(obj)
	// assume we have the following nodes to layout:
	// . ┌A──────────────┐  ┌B──┐  ┌C─────────┐  ┌D────────┐  ┌E────────────────┐
	// . └───────────────┘  │   │  │          │  │         │  │                 │
	// .                    │   │  └──────────┘  │         │  │                 │
	// .                    │   │                │         │  └─────────────────┘
	// .                    └───┘                │         │
	// .                                         └─────────┘
	// Note: if the grid is row dominant, all nodes should be the same height (same width if column dominant)
	// . ┌A─────────────┐  ┌B──┐  ┌C─────────┐  ┌D────────┐  ┌E────────────────┐
	// . ├ ─ ─ ─ ─ ─ ─ ─┤  │   │  │          │  │         │  │                 │
	// . │              │  │   │  ├ ─ ─ ─ ─ ─┤  │         │  │                 │
	// . │              │  │   │  │          │  │         │  ├ ─ ─ ─ ─ ─ ─ ─ ─ ┤
	// . │              │  ├ ─ ┤  │          │  │         │  │                 │
	// . └──────────────┘  └───┘  └──────────┘  └─────────┘  └─────────────────┘

	// we want to split up the total width across the N rows or columns as evenly as possible
	var totalWidth, totalHeight float64
	for _, n := range grid.nodes {
		totalWidth += n.Width
		totalHeight += n.Height
	}
	totalWidth += HORIZONTAL_PAD * float64(len(grid.nodes)-1)
	totalHeight += VERTICAL_PAD * float64(len(grid.nodes)-1)

	layout := [][]int{{}}
	if grid.rowDominant {
		targetWidth := totalWidth / float64(grid.rows)
		rowWidth := 0.
		rowIndex := 0
		for i, n := range grid.nodes {
			layout[rowIndex] = append(layout[rowIndex], i)
			rowWidth += n.Width + HORIZONTAL_PAD
			// add a new row if we pass the target width and there are more nodes
			if rowWidth > targetWidth && i < len(grid.nodes)-1 {
				layout = append(layout, []int{})
				rowIndex++
				rowWidth = 0
			}
		}
	} else {
		targetHeight := totalHeight / float64(grid.columns)
		columnHeight := 0.
		columnIndex := 0
		for i, n := range grid.nodes {
			layout[columnIndex] = append(layout[columnIndex], i)
			columnHeight += n.Height + VERTICAL_PAD
			if columnHeight > targetHeight && i < len(grid.nodes)-1 {
				layout = append(layout, []int{})
				columnIndex++
				columnHeight = 0
			}
		}
	}

	cursor := geo.NewPoint(0, 0)
	var maxY, maxX float64
	if grid.rowDominant {
		// if we have 2 rows, then each row's nodes should have the same height
		// . ┌A─────────────┐  ┌B──┐  ┌C─────────┐ ┬ maxHeight(A,B,C)
		// . ├ ─ ─ ─ ─ ─ ─ ─┤  │   │  │          │ │
		// . │              │  │   │  ├ ─ ─ ─ ─ ─┤ │
		// . │              │  │   │  │          │ │
		// . └──────────────┘  └───┘  └──────────┘ ┴
		// . ┌D────────┐  ┌E────────────────┐ ┬ maxHeight(D,E)
		// . │         │  │                 │ │
		// . │         │  │                 │ │
		// . │         │  ├ ─ ─ ─ ─ ─ ─ ─ ─ ┤ │
		// . │         │  │                 │ │
		// . └─────────┘  └─────────────────┘ ┴
		rowWidths := []float64{}
		for _, row := range layout {
			rowHeight := 0.
			for _, nodeIndex := range row {
				n := grid.nodes[nodeIndex]
				n.TopLeft = cursor.Copy()
				cursor.X += n.Width + HORIZONTAL_PAD
				rowHeight = math.Max(rowHeight, n.Height)
			}
			rowWidth := cursor.X - HORIZONTAL_PAD
			rowWidths = append(rowWidths, rowWidth)
			maxX = math.Max(maxX, rowWidth)

			// set all nodes in row to the same height
			for _, nodeIndex := range row {
				n := grid.nodes[nodeIndex]
				n.Height = rowHeight
			}

			// new row
			cursor.X = 0
			cursor.Y += rowHeight + VERTICAL_PAD
		}
		maxY = cursor.Y - VERTICAL_PAD

		// then expand thinnest nodes to make each row the same width
		// . ┌A─────────────┐  ┌B──┐  ┌C─────────┐ ┬ maxHeight(A,B,C)
		// . │              │  │   │  │          │ │
		// . │              │  │   │  │          │ │
		// . │              │  │   │  │          │ │
		// . └──────────────┘  └───┘  └──────────┘ ┴
		// . ┌D────────┬────┐  ┌E────────────────┐ ┬ maxHeight(D,E)
		// . │              │  │                 │ │
		// . │         │    │  │                 │ │
		// . │              │  │                 │ │
		// . │         │    │  │                 │ │
		// . └─────────┴────┘  └─────────────────┘ ┴
		for i, row := range layout {
			rowWidth := rowWidths[i]
			if rowWidth == maxX {
				continue
			}
			delta := maxX - rowWidth
			nodes := []*d2graph.Object{}
			var widest float64
			for _, nodeIndex := range row {
				n := grid.nodes[nodeIndex]
				widest = math.Max(widest, n.Width)
				nodes = append(nodes, n)
			}
			sort.Slice(nodes, func(i, j int) bool {
				return nodes[i].Width < nodes[j].Width
			})
			// expand smaller nodes to fill remaining space
			for _, n := range nodes {
				if n.Width < widest {
					var index int
					for i, nodeIndex := range row {
						if n == grid.nodes[nodeIndex] {
							index = i
							break
						}
					}
					grow := math.Min(widest-n.Width, delta)
					n.Width += grow
					// shift following nodes
					for i := index + 1; i < len(row); i++ {
						grid.nodes[row[i]].TopLeft.X += grow
					}
					delta -= grow
					if delta <= 0 {
						break
					}
				}
			}
			if delta > 0 {
				grow := delta / float64(len(row))
				for i := len(row) - 1; i >= 0; i-- {
					n := grid.nodes[row[i]]
					n.TopLeft.X += grow * float64(i)
					n.Width += grow
					delta -= grow
				}
			}
		}
	} else {
		// if we have 3 columns, then each column's nodes should have the same width
		// . ├maxWidth(A,B)─┤  ├maxW(C,D)─┤  ├maxWidth(E)──────┤
		// . ┌A─────────────┐  ┌C─────────┐  ┌E────────────────┐
		// . └──────────────┘  │          │  │                 │
		// . ┌B──┬──────────┐  └──────────┘  │                 │
		// . │              │  ┌D────────┬┐  └─────────────────┘
		// . │   │          │  │          │
		// . │              │  │         ││
		// . └───┴──────────┘  │          │
		// .                   │         ││
		// .                   └─────────┴┘
		for _, column := range layout {
			columnWidth := 0.
			for _, nodeIndex := range column {
				n := grid.nodes[nodeIndex]
				n.TopLeft = cursor.Copy()
				cursor.Y += n.Height + VERTICAL_PAD
				columnWidth = math.Max(columnWidth, n.Width)
			}
			maxY = math.Max(maxY, cursor.Y-VERTICAL_PAD)
			// set all nodes in column to the same width
			for _, nodeIndex := range column {
				n := grid.nodes[nodeIndex]
				n.Width = columnWidth
			}

			// new column
			cursor.Y = 0
			cursor.X += columnWidth + HORIZONTAL_PAD
		}
		maxX = cursor.X - HORIZONTAL_PAD
		// then expand shortest nodes to make each column the same height
		// . ├maxWidth(A,B)─┤  ├maxW(C,D)─┤  ├maxWidth(E)──────┤
		// . ┌A─────────────┐  ┌C─────────┐  ┌E────────────────┐
		// . ├ ─ ─ ─ ─ ─ ─  ┤  │          │  │                 │
		// . │              │  └──────────┘  │                 │
		// . └──────────────┘  ┌D─────────┐  ├ ─ ─ ─ ─ ─ ─ ─ ─ ┤
		// . ┌B─────────────┐  │          │  │                 │
		// . │              │  │          │  │                 │
		// . │              │  │          │  │                 │
		// . │              │  │          │  │                 │
		// . └──────────────┘  └──────────┘  └─────────────────┘
		// TODO see rows
	}
	grid.width = maxX
	grid.height = maxY

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

// cleanup restores the graph after the core layout engine finishes
// - translating the grid to its position placed by the core layout engine
// - restore the children of the grid
// - sorts objects to their original graph order
func cleanup(graph *d2graph.Graph, grids map[string]*grid, objectsOrder map[string]int) {
	defer func() {
		sort.SliceStable(graph.Objects, func(i, j int) bool {
			return objectsOrder[graph.Objects[i].AbsID()] < objectsOrder[graph.Objects[j].AbsID()]
		})
	}()

	if graph.Root.IsGrid() {
		grid, exists := grids[graph.Root.AbsID()]
		if exists {
			grid.cleanup(graph.Root, graph)
			return
		}
	}

	for _, obj := range graph.Objects {
		grid, exists := grids[obj.AbsID()]
		if !exists {
			continue
		}
		obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
		// shift the grid from (0, 0)
		grid.shift(
			obj.TopLeft.X+CONTAINER_PADDING,
			obj.TopLeft.Y+CONTAINER_PADDING,
		)
		grid.cleanup(obj, graph)
	}
}
