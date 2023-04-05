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
		gridDiagrams, objectOrder, err := withoutGridDiagrams(ctx, g)
		if err != nil {
			return err
		}

		if g.Root.IsGridDiagram() && len(g.Root.ChildrenArray) != 0 {
			g.Root.TopLeft = geo.NewPoint(0, 0)
		} else if err := layout(ctx, g); err != nil {
			return err
		}

		cleanup(g, gridDiagrams, objectOrder)
		return nil
	}
}

func withoutGridDiagrams(ctx context.Context, g *d2graph.Graph) (gridDiagrams map[string]*gridDiagram, objectOrder map[string]int, err error) {
	toRemove := make(map[*d2graph.Object]struct{})
	gridDiagrams = make(map[string]*gridDiagram)

	if len(g.Objects) > 0 {
		queue := make([]*d2graph.Object, 1, len(g.Objects))
		queue[0] = g.Root
		for len(queue) > 0 {
			obj := queue[0]
			queue = queue[1:]
			if len(obj.ChildrenArray) == 0 {
				continue
			}
			if !obj.IsGridDiagram() {
				queue = append(queue, obj.ChildrenArray...)
				continue
			}

			gd, err := layoutGrid(g, obj)
			if err != nil {
				return nil, nil, err
			}
			obj.Children = make(map[string]*d2graph.Object)
			obj.ChildrenArray = nil

			var dx, dy float64
			width := gd.width + 2*CONTAINER_PADDING
			labelWidth := float64(obj.LabelDimensions.Width) + 2*label.PADDING
			if labelWidth > width {
				dx = (labelWidth - width) / 2
				width = labelWidth
			}
			height := gd.height + 2*CONTAINER_PADDING
			labelHeight := float64(obj.LabelDimensions.Height) + 2*label.PADDING
			if labelHeight > CONTAINER_PADDING {
				// if the label doesn't fit within the padding, we need to add more
				grow := labelHeight - CONTAINER_PADDING
				dy = grow / 2
				height += grow
			}
			// we need to center children if we have to expand to fit the container label
			if dx != 0 || dy != 0 {
				gd.shift(dx, dy)
			}
			obj.Box = geo.NewBox(nil, width, height)

			obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			gridDiagrams[obj.AbsID()] = gd

			for _, node := range gd.nodes {
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

	return gridDiagrams, objectOrder, nil
}

func layoutGrid(g *d2graph.Graph, obj *d2graph.Object) (*gridDiagram, error) {
	gd := newGridDiagram(obj)

	if gd.rows != 0 && gd.columns != 0 {
		gd.layoutEvenly(g, obj)
	} else {
		gd.layoutDynamic(g, obj)
	}

	// position labels and icons
	for _, n := range gd.nodes {
		if n.Attributes.Icon != nil {
			n.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
			n.IconPosition = go2.Pointer(string(label.InsideMiddleCenter))
		} else {
			n.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
		}
	}

	return gd, nil
}

func (gd *gridDiagram) layoutEvenly(g *d2graph.Graph, obj *d2graph.Object) {
	// layout nodes in a grid with these 2 properties:
	// all nodes in the same row should have the same height
	// all nodes in the same column should have the same width

	getNode := func(rowIndex, columnIndex int) *d2graph.Object {
		var index int
		if gd.rowDominant {
			index = rowIndex*gd.columns + columnIndex
		} else {
			index = columnIndex*gd.rows + rowIndex
		}
		if index < len(gd.nodes) {
			return gd.nodes[index]
		}
		return nil
	}

	rowHeights := make([]float64, 0, gd.rows)
	colWidths := make([]float64, 0, gd.columns)
	for i := 0; i < gd.rows; i++ {
		rowHeight := 0.
		for j := 0; j < gd.columns; j++ {
			n := getNode(i, j)
			if n == nil {
				break
			}
			rowHeight = math.Max(rowHeight, n.Height)
		}
		rowHeights = append(rowHeights, rowHeight)
	}
	for j := 0; j < gd.columns; j++ {
		columnWidth := 0.
		for i := 0; i < gd.rows; i++ {
			n := getNode(i, j)
			if n == nil {
				break
			}
			columnWidth = math.Max(columnWidth, n.Width)
		}
		colWidths = append(colWidths, columnWidth)
	}

	cursor := geo.NewPoint(0, 0)
	if gd.rowDominant {
		for i := 0; i < gd.rows; i++ {
			for j := 0; j < gd.columns; j++ {
				n := getNode(i, j)
				if n == nil {
					break
				}
				n.Width = colWidths[j]
				n.Height = rowHeights[i]
				n.TopLeft = cursor.Copy()
				cursor.X += n.Width + HORIZONTAL_PAD
			}
			cursor.X = 0
			cursor.Y += rowHeights[i] + VERTICAL_PAD
		}
	} else {
		for j := 0; j < gd.columns; j++ {
			for i := 0; i < gd.rows; i++ {
				n := getNode(i, j)
				if n == nil {
					break
				}
				n.Width = colWidths[j]
				n.Height = rowHeights[i]
				n.TopLeft = cursor.Copy()
				cursor.Y += n.Height + VERTICAL_PAD
			}
			cursor.X += colWidths[j] + HORIZONTAL_PAD
			cursor.Y = 0
		}
	}

	var totalWidth, totalHeight float64
	for _, w := range colWidths {
		totalWidth += w + HORIZONTAL_PAD
	}
	for _, h := range rowHeights {
		totalHeight += h + VERTICAL_PAD
	}
	totalWidth -= HORIZONTAL_PAD
	totalHeight -= VERTICAL_PAD
	gd.width = totalWidth
	gd.height = totalHeight
}

func (gd *gridDiagram) layoutDynamic(g *d2graph.Graph, obj *d2graph.Object) {
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
	for _, n := range gd.nodes {
		totalWidth += n.Width
		totalHeight += n.Height
	}
	totalWidth += HORIZONTAL_PAD * float64(len(gd.nodes)-1)
	totalHeight += VERTICAL_PAD * float64(len(gd.nodes)-1)

	layout := [][]int{{}}
	if gd.rowDominant {
		targetWidth := totalWidth / float64(gd.rows)
		rowWidth := 0.
		rowIndex := 0
		addRow := func() {
			layout = append(layout, []int{})
			rowIndex++
			rowWidth = 0
		}
		addNode := func(i int, n *d2graph.Object) {
			layout[rowIndex] = append(layout[rowIndex], i)
			rowWidth += n.Width + HORIZONTAL_PAD
		}

		for i, n := range gd.nodes {
			// if the next node will be past the target, start a new row
			if rowWidth+n.Width+HORIZONTAL_PAD > targetWidth {
				// if the node is mostly past the target, put it on the next row
				if rowWidth+n.Width/2 > targetWidth {
					addRow()
					addNode(i, n)
				} else {
					addNode(i, n)
					if i < len(gd.nodes)-1 {
						addRow()
					}
				}
			} else {
				addNode(i, n)
			}
		}
	} else {
		targetHeight := totalHeight / float64(gd.columns)
		colHeight := 0.
		colIndex := 0
		addCol := func() {
			layout = append(layout, []int{})
			colIndex++
			colHeight = 0
		}
		addNode := func(i int, n *d2graph.Object) {
			layout[colIndex] = append(layout[colIndex], i)
			colHeight += n.Height + VERTICAL_PAD
		}

		for i, n := range gd.nodes {
			// if the next node will be past the target, start a new row
			if colHeight+n.Height+VERTICAL_PAD > targetHeight {
				// if the node is mostly past the target, put it on the next row
				if colHeight+n.Height/2 > targetHeight {
					addCol()
					addNode(i, n)
				} else {
					addNode(i, n)
					if i < len(gd.nodes)-1 {
						addCol()
					}
				}
			} else {
				addNode(i, n)
			}
		}
	}

	cursor := geo.NewPoint(0, 0)
	var maxY, maxX float64
	if gd.rowDominant {
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
				n := gd.nodes[nodeIndex]
				n.TopLeft = cursor.Copy()
				cursor.X += n.Width + HORIZONTAL_PAD
				rowHeight = math.Max(rowHeight, n.Height)
			}
			rowWidth := cursor.X - HORIZONTAL_PAD
			rowWidths = append(rowWidths, rowWidth)
			maxX = math.Max(maxX, rowWidth)

			// set all nodes in row to the same height
			for _, nodeIndex := range row {
				n := gd.nodes[nodeIndex]
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
				n := gd.nodes[nodeIndex]
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
						if n == gd.nodes[nodeIndex] {
							index = i
							break
						}
					}
					grow := math.Min(widest-n.Width, delta)
					n.Width += grow
					// shift following nodes
					for i := index + 1; i < len(row); i++ {
						gd.nodes[row[i]].TopLeft.X += grow
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
					n := gd.nodes[row[i]]
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
		colHeights := []float64{}
		for _, column := range layout {
			colWidth := 0.
			for _, nodeIndex := range column {
				n := gd.nodes[nodeIndex]
				n.TopLeft = cursor.Copy()
				cursor.Y += n.Height + VERTICAL_PAD
				colWidth = math.Max(colWidth, n.Width)
			}
			colHeight := cursor.Y - VERTICAL_PAD
			colHeights = append(colHeights, colHeight)
			maxY = math.Max(maxY, colHeight)
			// set all nodes in column to the same width
			for _, nodeIndex := range column {
				n := gd.nodes[nodeIndex]
				n.Width = colWidth
			}

			// new column
			cursor.Y = 0
			cursor.X += colWidth + HORIZONTAL_PAD
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
		for i, column := range layout {
			colHeight := colHeights[i]
			if colHeight == maxY {
				continue
			}
			delta := maxY - colHeight
			nodes := []*d2graph.Object{}
			var tallest float64
			for _, nodeIndex := range column {
				n := gd.nodes[nodeIndex]
				tallest = math.Max(tallest, n.Height)
				nodes = append(nodes, n)
			}
			sort.Slice(nodes, func(i, j int) bool {
				return nodes[i].Height < nodes[j].Height
			})
			// expand smaller nodes to fill remaining space
			for _, n := range nodes {
				if n.Height < tallest {
					var index int
					for i, nodeIndex := range column {
						if n == gd.nodes[nodeIndex] {
							index = i
							break
						}
					}
					grow := math.Min(tallest-n.Height, delta)
					n.Height += grow
					// shift following nodes
					for i := index + 1; i < len(column); i++ {
						gd.nodes[column[i]].TopLeft.Y += grow
					}
					delta -= grow
					if delta <= 0 {
						break
					}
				}
			}
			if delta > 0 {
				grow := delta / float64(len(column))
				for i := len(column) - 1; i >= 0; i-- {
					n := gd.nodes[column[i]]
					n.TopLeft.Y += grow * float64(i)
					n.Height += grow
					delta -= grow
				}
			}
		}
	}
	gd.width = maxX
	gd.height = maxY
}

// cleanup restores the graph after the core layout engine finishes
// - translating the grid to its position placed by the core layout engine
// - restore the children of the grid
// - sorts objects to their original graph order
func cleanup(graph *d2graph.Graph, gridDiagrams map[string]*gridDiagram, objectsOrder map[string]int) {
	defer func() {
		sort.SliceStable(graph.Objects, func(i, j int) bool {
			return objectsOrder[graph.Objects[i].AbsID()] < objectsOrder[graph.Objects[j].AbsID()]
		})
	}()

	if graph.Root.IsGridDiagram() {
		gd, exists := gridDiagrams[graph.Root.AbsID()]
		if exists {
			gd.cleanup(graph.Root, graph)
			return
		}
	}

	for _, obj := range graph.Objects {
		gd, exists := gridDiagrams[obj.AbsID()]
		if !exists {
			continue
		}
		obj.LabelPosition = go2.Pointer(string(label.InsideTopCenter))
		// shift the grid from (0, 0)
		gd.shift(
			obj.TopLeft.X+CONTAINER_PADDING,
			obj.TopLeft.Y+CONTAINER_PADDING,
		)
		gd.cleanup(obj, graph)
	}
}
