package d2grid

import (
	"strconv"

	"oss.terrastruct.com/d2/d2graph"
)

type gridDiagram struct {
	root    *d2graph.Object
	nodes   []*d2graph.Object
	rows    int
	columns int

	rowDominant bool

	width  float64
	height float64
}

func newGridDiagram(root *d2graph.Object) *gridDiagram {
	gd := gridDiagram{root: root, nodes: root.ChildrenArray}
	if root.Attributes.Rows != nil {
		gd.rows, _ = strconv.Atoi(root.Attributes.Rows.Value)
	}
	if root.Attributes.Columns != nil {
		gd.columns, _ = strconv.Atoi(root.Attributes.Columns.Value)
	}

	// compute exact row/column count based on values entered
	if gd.columns == 0 {
		gd.rowDominant = true
	} else if gd.rows == 0 {
		gd.rowDominant = false
	} else {
		// if keyword rows is first, rows are primary, columns secondary.
		if root.Attributes.Rows.MapKey.Range.Before(root.Attributes.Columns.MapKey.Range) {
			gd.rowDominant = true
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
		capacity := gd.rows * gd.columns
		for capacity < len(gd.nodes) {
			if gd.rowDominant {
				gd.rows++
				capacity += gd.columns
			} else {
				gd.columns++
				capacity += gd.rows
			}
		}
	}

	return &gd
}

func (gd *gridDiagram) shift(dx, dy float64) {
	for _, obj := range gd.nodes {
		obj.TopLeft.X += dx
		obj.TopLeft.Y += dy
	}
}

func (gd *gridDiagram) cleanup(obj *d2graph.Object, graph *d2graph.Graph) {
	obj.Children = make(map[string]*d2graph.Object)
	obj.ChildrenArray = make([]*d2graph.Object, 0)
	for _, child := range gd.nodes {
		obj.Children[child.ID] = child
		obj.ChildrenArray = append(obj.ChildrenArray, child)
	}
	graph.Objects = append(graph.Objects, gd.nodes...)
}
