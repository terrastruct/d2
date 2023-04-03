package d2grid

import (
	"math"
	"strconv"

	"oss.terrastruct.com/d2/d2graph"
)

type grid struct {
	root    *d2graph.Object
	nodes   []*d2graph.Object
	rows    int
	columns int

	cellWidth  float64
	cellHeight float64
	width      float64
	height     float64
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
	if g.rows == 0 {
		// set rows based on number of columns
		g.rows = len(g.nodes) / g.columns
		if len(g.nodes)%g.columns != 0 {
			g.rows++
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

	// if we have the following nodes for a 2 row, 3 column grid
	// . ┌A──────────────────┐  ┌B─────┐  ┌C────────────┐  ┌D───────────┐  ┌E───────────────────┐
	// . │                   │  │      │  │             │  │            │  │                    │
	// . │                   │  │      │  │             │  │            │  │                    │
	// . └───────────────────┘  │      │  │             │  │            │  │                    │
	// .                        │      │  └─────────────┘  │            │  │                    │
	// .                        │      │                   │            │  └────────────────────┘
	// .                        └──────┘                   │            │
	// .                                                   └────────────┘
	// Then we must get the max width and max height to determine the grid cell size
	// .                                          maxWidth├────────────────────┤
	// .  ┌A───────────────────┐  ┌B───────────────────┐  ┌C───────────────────┐ ┬maxHeight
	// .  │                    │  │                    │  │                    │ │
	// .  │                    │  │                    │  │                    │ │
	// .  │                    │  │                    │  │                    │ │
	// .  │                    │  │                    │  │                    │ │
	// .  │                    │  │                    │  │                    │ │
	// .  │                    │  │                    │  │                    │ │
	// .  └────────────────────┘  └────────────────────┘  └────────────────────┘ ┴
	// .  ┌D───────────────────┐  ┌E───────────────────┐
	// .  │                    │  │                    │
	// .  │                    │  │                    │
	// .  │                    │  │                    │
	// .  │                    │  │                    │
	// .  │                    │  │                    │
	// .  │                    │  │                    │
	// .  └────────────────────┘  └────────────────────┘
	var maxWidth, maxHeight float64
	for _, n := range g.nodes {
		maxWidth = math.Max(maxWidth, n.Width)
		maxHeight = math.Max(maxHeight, n.Height)
	}
	g.cellWidth = maxWidth
	g.cellHeight = maxHeight
	g.width = maxWidth + (float64(g.columns)-1)*(maxWidth+HORIZONTAL_PAD)
	g.height = maxHeight + (float64(g.rows)-1)*(maxHeight+VERTICAL_PAD)

	return &g
}

func (g *grid) shift(dx, dy float64) {
	for _, obj := range g.nodes {
		obj.TopLeft.X += dx
		obj.TopLeft.Y += dy
	}
}
