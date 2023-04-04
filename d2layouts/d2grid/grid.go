package d2grid

import (
	"strconv"

	"oss.terrastruct.com/d2/d2graph"
)

type grid struct {
	root    *d2graph.Object
	nodes   []*d2graph.Object
	rows    int
	columns int

	rowDominant bool

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
	if g.columns == 0 {
		g.rowDominant = true
	} else if g.rows == 0 {
		g.rowDominant = false
	} else {
		// if keyword rows is first, rows are primary, columns secondary.
		if root.Attributes.Rows.MapKey.Range.Before(root.Attributes.Columns.MapKey.Range) {
			g.rowDominant = true
		}

		// rows and columns specified, but we want to continue naturally if user enters more nodes
		// e.g. 2 rows, 3 columns specified + g node added: │ with 3 columns, 2 rows:
		// . original  add row   add column                 │ original  add row   add column
		// . ┌───────┐ ┌───────┐ ┌─────────┐                │ ┌───────┐ ┌───────┐ ┌─────────┐
		// . │ a b c │ │ a b c │ │ a b c d │                │ │ a c e │ │ a d g │ │ a c e g │
		// . │ d e f │ │ d e f │ │ e f g   │                │ │ b d f │ │ b e   │ │ b d f   │
		// . └───────┘ │ g     │ └─────────┘                │ └───────┘ │ c f   │ └─────────┘
		// .           └───────┘ ▲                          │           └───────┘ ▲
		// .           ▲         └─existing nodes modified  │           ▲         └─existing nodes preserved
		// .           └─existing rows preserved            │           └─existing rows modified
		capacity := g.rows * g.columns
		for capacity < len(g.nodes) {
			if g.rowDominant {
				g.rows++
				capacity += g.columns
			} else {
				g.columns++
				capacity += g.rows
			}
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

func (g *grid) cleanup(obj *d2graph.Object, graph *d2graph.Graph) {
	obj.Children = make(map[string]*d2graph.Object)
	obj.ChildrenArray = make([]*d2graph.Object, 0)
	for _, child := range g.nodes {
		obj.Children[child.ID] = child
		obj.ChildrenArray = append(obj.ChildrenArray, child)
	}
	graph.Objects = append(graph.Objects, g.nodes...)
}
