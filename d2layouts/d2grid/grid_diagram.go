package d2grid

import (
	"strconv"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
)

type gridDiagram struct {
	root    *d2graph.Object
	objects []*d2graph.Object
	rows    int
	columns int

	// if true, place objects left to right along rows
	// if false, place objects top to bottom along columns
	rowDirected bool

	width  float64
	height float64

	gapRows    int
	gapColumns int
}

func newGridDiagram(root *d2graph.Object) *gridDiagram {
	gd := gridDiagram{
		root:       root,
		objects:    root.ChildrenArray,
		gapRows:    DEFAULT_GAP,
		gapColumns: DEFAULT_GAP,
	}

	if root.Attributes.GridRows != nil {
		gd.rows, _ = strconv.Atoi(root.Attributes.GridRows.Value)
	}
	if root.Attributes.GridColumns != nil {
		gd.columns, _ = strconv.Atoi(root.Attributes.GridColumns.Value)
	}

	if gd.rows != 0 && gd.columns != 0 {
		// . row-directed  column-directed
		// .  ┌───────┐    ┌───────┐
		// .  │ a b c │    │ a d g │
		// .  │ d e f │    │ b e h │
		// .  │ g h i │    │ c f i │
		// .  └───────┘    └───────┘
		// if keyword rows is first, make it row-directed, if columns is first it is column-directed
		if root.Attributes.GridRows.MapKey.Range.Before(root.Attributes.GridColumns.MapKey.Range) {
			gd.rowDirected = true
		}

		// rows and columns specified, but we want to continue naturally if user enters more objects
		// e.g. 2 rows, 3 columns specified + g added:      │ with 3 columns, 2 rows:
		// . original  add row   add column                 │ original  add row   add column
		// . ┌───────┐ ┌───────┐ ┌─────────┐                │ ┌───────┐ ┌───────┐ ┌─────────┐
		// . │ a b c │ │ a b c │ │ a b c d │                │ │ a c e │ │ a d g │ │ a c e g │
		// . │ d e f │ │ d e f │ │ e f g   │                │ │ b d f │ │ b e   │ │ b d f   │
		// . └───────┘ │ g     │ └─────────┘                │ └───────┘ │ c f   │ └─────────┘
		// .           └───────┘ ▲                          │           └───────┘ ▲
		// .           ▲         └─existing objects modified│           ▲         └─existing columns preserved
		// .           └─existing rows preserved            │           └─existing objects modified
		capacity := gd.rows * gd.columns
		for capacity < len(gd.objects) {
			if gd.rowDirected {
				gd.rows++
				capacity += gd.columns
			} else {
				gd.columns++
				capacity += gd.rows
			}
		}
	} else if gd.columns == 0 {
		gd.rowDirected = true
	}

	// grid gap sets both, but can be overridden
	if root.Attributes.GridGap != nil {
		gd.gapRows, _ = strconv.Atoi(root.Attributes.GridGap.Value)
		gd.gapColumns = gd.gapRows
	}
	if root.Attributes.GridGapRows != nil {
		gd.gapRows, _ = strconv.Atoi(root.Attributes.GridGapRows.Value)
	}
	if root.Attributes.GridGapColumns != nil {
		gd.gapColumns, _ = strconv.Atoi(root.Attributes.GridGapColumns.Value)
	}

	return &gd
}

func (gd *gridDiagram) shift(dx, dy float64) {
	for _, obj := range gd.objects {
		obj.TopLeft.X += dx
		obj.TopLeft.Y += dy
	}
}

func (gd *gridDiagram) cleanup(obj *d2graph.Object, graph *d2graph.Graph) {
	obj.Children = make(map[string]*d2graph.Object)
	obj.ChildrenArray = make([]*d2graph.Object, 0)
	for _, child := range gd.objects {
		obj.Children[strings.ToLower(child.ID)] = child
		obj.ChildrenArray = append(obj.ChildrenArray, child)
	}
	graph.Objects = append(graph.Objects, gd.objects...)
}
