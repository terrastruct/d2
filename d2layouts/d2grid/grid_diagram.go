package d2grid

import (
	"strconv"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
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

	verticalGap   int
	horizontalGap int
}

func newGridDiagram(root *d2graph.Object) *gridDiagram {
	gd := gridDiagram{
		root:          root,
		objects:       root.ChildrenArray,
		verticalGap:   DEFAULT_GAP,
		horizontalGap: DEFAULT_GAP,
	}

	if root.GridRows != nil {
		gd.rows, _ = strconv.Atoi(root.GridRows.Value)
	}
	if root.GridColumns != nil {
		gd.columns, _ = strconv.Atoi(root.GridColumns.Value)
	}

	if gd.rows != 0 && gd.columns != 0 {
		// . row-directed  column-directed
		// .  ┌───────┐    ┌───────┐
		// .  │ a b c │    │ a d g │
		// .  │ d e f │    │ b e h │
		// .  │ g h i │    │ c f i │
		// .  └───────┘    └───────┘
		// if keyword rows is first, make it row-directed, if columns is first it is column-directed
		if root.GridRows.MapKey.Range.Before(root.GridColumns.MapKey.Range) {
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
		// we can only make N rows with N objects
		if len(gd.objects) < gd.rows {
			gd.rows = len(gd.objects)
		}
	} else {
		if len(gd.objects) < gd.columns {
			gd.columns = len(gd.objects)
		}
	}

	// grid gap sets both, but can be overridden
	if root.GridGap != nil {
		gd.verticalGap, _ = strconv.Atoi(root.GridGap.Value)
		gd.horizontalGap = gd.verticalGap
	}
	if root.VerticalGap != nil {
		gd.verticalGap, _ = strconv.Atoi(root.VerticalGap.Value)
	}
	if root.HorizontalGap != nil {
		gd.horizontalGap, _ = strconv.Atoi(root.HorizontalGap.Value)
	}

	for _, o := range gd.objects {
		o.TopLeft = geo.NewPoint(0, 0)
	}

	return &gd
}

func (gd *gridDiagram) shift(dx, dy float64) {
	for _, obj := range gd.objects {
		obj.MoveWithDescendants(dx, dy)
	}
}

func (gd *gridDiagram) cleanup(obj *d2graph.Object, graph *d2graph.Graph) {
	obj.Children = make(map[string]*d2graph.Object)
	obj.ChildrenArray = make([]*d2graph.Object, 0)

	restore := func(parent, child *d2graph.Object) {
		parent.Children[strings.ToLower(child.ID)] = child
		parent.ChildrenArray = append(parent.ChildrenArray, child)
		graph.Objects = append(graph.Objects, child)
	}
	for _, child := range gd.objects {
		restore(obj, child)
		child.IterDescendants(restore)
	}
}
